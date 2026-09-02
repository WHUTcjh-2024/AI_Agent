"""HTTP 抓取层。

§33 请求速率与安全：
  - 只访问公开网页，禁止登录绕过 / 验证码绕过 / 漏洞利用 / 非公开接口
  - 默认单 host 并发 <= 3，请求间隔合理控制
  - 失败最多 3 次 retry + exponential backoff
  - 429 / 大量 5xx / 明显变慢 → 自动降速
  - 目标：绝不能影响学校网站正常运行

§34 浏览器策略：普通 HTTP 是第一选择，只有 JS 必须执行且页面确实高价值时才用浏览器。
    本实现默认纯 HTTP；browser_enabled 打开时也只作为可选路径存在。

Agent B 老 CMS 兼容：GBK / GB2312 / 无 BOM UTF-8 都要能正确解码。
"""

from __future__ import annotations

import asyncio
import random
import re
import time
from dataclasses import dataclass, field
from typing import Callable, Dict, Optional, Tuple
from urllib.parse import urljoin, urlsplit
from urllib.robotparser import RobotFileParser

import httpx

from .url_utils import host_of


@dataclass
class FetchResult:
    ok: bool
    url: str
    final_url: str = ""
    http_status: int = 0
    content: bytes = b""
    text: str = ""
    encoding: str = ""
    content_type: str = ""
    content_disposition: str = ""
    etag: str = ""
    last_modified: str = ""
    error_type: str = ""
    error_message: str = ""
    elapsed_ms: int = 0
    from_cache: bool = False


@dataclass
class HostState:
    """单 host 的限速与压力状态机。"""

    semaphore: asyncio.Semaphore
    throttle_lock: asyncio.Lock
    min_interval: float
    last_request_at: float = 0.0
    error_streak: int = 0
    success_streak: int = 0
    penalty: float = 1.0


class RobotsPolicy:
    """robots.txt 策略：缓存 + 失败保守放行。"""

    def __init__(self, enabled: bool = True, on_missing: str = "allow", user_agent: str = "*"):
        self.enabled = enabled
        self.on_missing = on_missing
        self.user_agent = user_agent
        self._parsers: Dict[str, Optional[RobotFileParser]] = {}
        self._fetched: set = set()

    def load(self, host: str, content: Optional[str]) -> None:
        if not self.enabled:
            return
        parser = RobotFileParser()
        if content:
            try:
                parser.parse(content.splitlines())
            except Exception:
                parser = None
        else:
            parser = None
        self._parsers[host] = parser

    def is_known(self, host: str) -> bool:
        return host in self._parsers

    def allows(self, url: str) -> bool:
        if not self.enabled:
            return True
        host = host_of(url)
        parser = self._parsers.get(host)
        if parser is None:
            return self.on_missing == "allow"
        try:
            return parser.can_fetch(self.user_agent, url)
        except Exception:
            return self.on_missing == "allow"

    def crawl_delay(self, url: str) -> Optional[float]:
        if not self.enabled:
            return None
        host = host_of(url)
        parser = self._parsers.get(host)
        if parser is None:
            return None
        try:
            delay = parser.crawl_delay(self.user_agent)
        except Exception:
            return None
        return float(delay) if delay else None


class Fetcher:
    """HTTP 抓取器：并发上限、每 host 限速、指数退避、压力自适应。"""

    def __init__(
        self,
        *,
        user_agent: str,
        concurrency_per_host: int = 3,
        concurrency_total: int = 8,
        min_interval_per_host: float = 0.8,
        jitter_seconds: float = 0.4,
        timeout_seconds: float = 25,
        max_retries: int = 3,
        backoff_base_seconds: float = 1.5,
        backoff_max_seconds: float = 30,
        pressure: Optional[Dict[str, float]] = None,
        robots: Optional[RobotsPolicy] = None,
        max_file_size_bytes: int = 50 * 1024 * 1024,
        redirect_validator: Optional[Callable[[str], bool]] = None,
        max_redirects: int = 5,
        transport: Optional[httpx.AsyncBaseTransport] = None,
        verify_ssl: bool = True,
    ):
        self.user_agent = user_agent
        self.concurrency_per_host = max(1, int(concurrency_per_host))
        self.min_interval_per_host = float(min_interval_per_host)
        self.jitter_seconds = float(jitter_seconds)
        self.timeout_seconds = float(timeout_seconds)
        self.max_retries = max(1, int(max_retries))
        self.backoff_base_seconds = float(backoff_base_seconds)
        self.backoff_max_seconds = float(backoff_max_seconds)
        self.max_file_size_bytes = int(max_file_size_bytes)
        self.redirect_validator = redirect_validator
        self.max_redirects = max(0, int(max_redirects))
        self._transport = transport
        self.robots = robots or RobotsPolicy(enabled=False)

        pressure = pressure or {}
        self.error_streak_threshold = int(pressure.get("error_streak_threshold", 5))
        self.slow_down_factor = float(pressure.get("slow_down_factor", 4.0))
        self.recovery_streak = int(pressure.get("recovery_streak", 10))
        self.min_crawl_delay = float(pressure.get("min_crawl_delay_seconds", 0.0))

        self._hosts: Dict[str, HostState] = {}
        self._global_semaphore = asyncio.Semaphore(max(1, int(concurrency_total)))
        self._client: Optional[httpx.AsyncClient] = None
        self._verify_ssl = verify_ssl

        # 统计
        self.stats: Dict[str, int] = {
            "requests": 0, "success": 0, "failed": 0,
            "http_429": 0, "http_5xx": 0, "bytes": 0,
            "robots_blocked": 0, "too_large": 0,
        }

    # ---- 生命周期 ----

    async def __aenter__(self) -> "Fetcher":
        self._client = httpx.AsyncClient(
            timeout=httpx.Timeout(self.timeout_seconds),
            limits=httpx.Limits(
                max_connections=64,
                max_keepalive_connections=16,
                keepalive_expiry=30,
            ),
            headers={"User-Agent": self.user_agent,
                     "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
                     "Accept-Language": "zh-CN,zh;q=0.9"},
            # Redirects are followed manually so every hop can be validated
            # against the caller's SchoolContext/UrlGate before any request.
            follow_redirects=False,
            transport=self._transport,
            verify=self._verify_ssl,
        )
        return self

    async def __aexit__(self, *exc) -> None:
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    # ---- 限速 ----

    def _host_state(self, host: str) -> HostState:
        state = self._hosts.get(host)
        if state is None:
            state = HostState(
                semaphore=asyncio.Semaphore(self.concurrency_per_host),
                throttle_lock=asyncio.Lock(),
                min_interval=self.min_interval_per_host,
            )
            self._hosts[host] = state
        return state

    def _record_pressure(self, state: HostState, *, ok: bool, status: int) -> None:
        if ok:
            state.error_streak = 0
            state.success_streak += 1
            if state.success_streak >= self.recovery_streak:
                state.penalty = 1.0
            return
        state.success_streak = 0
        if status in (429, 503) or (500 <= status < 600):
            state.error_streak += 1
            if state.error_streak >= self.error_streak_threshold:
                # 服务器压力信号：自动降速
                state.penalty = self.slow_down_factor

    async def _throttle(self, host: str) -> None:
        state = self._host_state(host)
        # The host semaphore permits bounded parallel downloads, while this
        # lock serializes request starts so min_interval is actually enforced.
        async with state.throttle_lock:
            wait = state.min_interval * state.penalty
            robots_delay = self.min_crawl_delay
            wait = max(wait, robots_delay)
            elapsed = time.monotonic() - state.last_request_at
            if elapsed < wait:
                await asyncio.sleep(wait - elapsed + random.uniform(0, self.jitter_seconds))
            state.last_request_at = time.monotonic()

    # ---- robots ----

    async def ensure_robots(self, host: str) -> None:
        """按需拉取一次 robots.txt 并缓存。取不到时按 on_missing 策略处理。"""
        if not self.robots.enabled or self.robots.is_known(host):
            return
        schemes = ("https", "http")
        content = None
        for scheme in schemes:
            url = f"{scheme}://{host}/robots.txt"
            try:
                assert self._client is not None
                response = await self._client.get(url)
                if response.status_code == 200:
                    text = response.text
                    # 学校站常见把 404 页面伪装成 200，做一次内容判断
                    if "user-agent" in text.lower():
                        content = text
                        break
                    content = None
                else:
                    content = None
            except Exception:
                content = None
        self.robots.load(host, content)

    # ---- 抓取 ----

    async def fetch(
        self,
        url: str,
        *,
        as_attachment: bool = False,
        max_bytes: Optional[int] = None,
    ) -> FetchResult:
        assert self._client is not None, "Fetcher 必须在 async with 内使用"
        limit = max_bytes or self.max_file_size_bytes
        if limit <= 0:
            raise ValueError("max_bytes must be positive")

        last_error_type = ""
        last_error_message = ""
        last_status = 0
        last_final_url = ""
        state = self._host_state(host_of(url))

        for attempt in range(1, self.max_retries + 1):
            current_url = url
            started = time.monotonic()
            try:
                for redirect_count in range(self.max_redirects + 1):
                    host = host_of(current_url)
                    state = self._host_state(host)
                    await self.ensure_robots(host)
                    if not self.robots.allows(current_url):
                        self.stats["robots_blocked"] += 1
                        return FetchResult(
                            False, url, final_url=current_url, error_type="robots_disallowed",
                            error_message="robots.txt 禁止访问",
                        )
                    robots_delay = self.robots.crawl_delay(current_url)
                    if robots_delay:
                        state.min_interval = max(state.min_interval, robots_delay)

                    async with self._global_semaphore, state.semaphore:
                        await self._throttle(host)
                        self.stats["requests"] += 1
                        async with self._client.stream("GET", current_url) as response:
                            last_status = response.status_code
                            last_final_url = str(response.url)
                            elapsed_ms = int((time.monotonic() - started) * 1000)

                            if last_status in (301, 302, 303, 307, 308):
                                location = response.headers.get("location", "").strip()
                                if not location:
                                    return self._redirect_failure(url, last_final_url, last_status, "redirect_without_location", elapsed_ms)
                                if redirect_count >= self.max_redirects:
                                    return self._redirect_failure(url, last_final_url, last_status, "too_many_redirects", elapsed_ms)
                                target = urljoin(last_final_url, location)
                                if self.redirect_validator is None or not self.redirect_validator(target):
                                    return self._redirect_failure(url, target, last_status, "redirect_disallowed", elapsed_ms)
                                current_url = target
                                continue

                            declared = response.headers.get("content-length")
                            if declared and declared.isdigit() and int(declared) > limit:
                                self.stats["too_large"] += 1
                                self._record_pressure(state, ok=False, status=last_status)
                                return FetchResult(
                                    False, url, final_url=last_final_url, http_status=last_status,
                                    error_type="file_too_large",
                                    error_message=f"内容长度 {declared} 超过上限 {limit}",
                                    elapsed_ms=elapsed_ms,
                                )

                            chunks = bytearray()
                            async for chunk in response.aiter_bytes():
                                if len(chunks) + len(chunk) > limit:
                                    self.stats["too_large"] += 1
                                    self._record_pressure(state, ok=False, status=last_status)
                                    return FetchResult(
                                        False, url, final_url=last_final_url, http_status=last_status,
                                        error_type="file_too_large",
                                        error_message=f"实际下载超过上限 {limit}",
                                        elapsed_ms=int((time.monotonic() - started) * 1000),
                                    )
                                chunks.extend(chunk)
                            content = bytes(chunks)

                            if 200 <= last_status < 300:
                                self.stats["success"] += 1
                                self.stats["bytes"] += len(content)
                                self._record_pressure(state, ok=True, status=last_status)
                                text, encoding = decode_bytes(content, response.headers.get("content-type", ""))
                                return FetchResult(
                                    True, url, final_url=last_final_url, http_status=last_status,
                                    content=content, text=text, encoding=encoding,
                                    content_type=response.headers.get("content-type", ""),
                                    content_disposition=response.headers.get("content-disposition", ""),
                                    etag=response.headers.get("etag", ""),
                                    last_modified=response.headers.get("last-modified", ""),
                                    elapsed_ms=int((time.monotonic() - started) * 1000),
                                )

                            if 400 <= last_status < 500 and last_status != 429:
                                self.stats["failed"] += 1
                                self._record_pressure(state, ok=False, status=last_status)
                                return FetchResult(
                                    False, url, final_url=last_final_url, http_status=last_status,
                                    error_type=f"http_{last_status}", error_message=f"HTTP {last_status}",
                                    elapsed_ms=int((time.monotonic() - started) * 1000),
                                )

                            if last_status == 429:
                                self.stats["http_429"] += 1
                            elif 500 <= last_status < 600:
                                self.stats["http_5xx"] += 1
                            last_error_type = f"http_{last_status}"
                            last_error_message = f"HTTP {last_status}"
                            self._record_pressure(state, ok=False, status=last_status)
                            break
            except httpx.TimeoutException as error:
                last_status = 0
                last_error_type = "timeout"
                last_error_message = str(error)[:400]
            except httpx.HTTPError as error:
                last_status = 0
                last_error_type = type(error).__name__
                last_error_message = str(error)[:400]

            if attempt < self.max_retries:
                backoff = min(self.backoff_base_seconds * (2 ** (attempt - 1)), self.backoff_max_seconds)
                await asyncio.sleep(backoff * state.penalty + random.uniform(0, self.jitter_seconds))

        self.stats["failed"] += 1
        return FetchResult(
            False, url, final_url=last_final_url, http_status=last_status,
            error_type=last_error_type or "unknown",
            error_message=last_error_message or "抓取失败",
        )

    def _redirect_failure(self, original_url: str, target_url: str, status: int, error_type: str, elapsed_ms: int) -> FetchResult:
        self.stats["failed"] += 1
        return FetchResult(
            False, original_url, final_url=target_url, http_status=status,
            error_type=error_type, error_message="重定向目标未通过官方来源校验",
            elapsed_ms=elapsed_ms,
        )

    def pressure_snapshot(self) -> Dict[str, Dict[str, float]]:
        return {
            host: {
                "penalty": round(state.penalty, 2),
                "error_streak": float(state.error_streak),
            }
            for host, state in self._hosts.items()
            if state.penalty > 1.0
        }


# ---------------------------------------------------------------------------
# 编码解码：高校老 CMS 大量使用 GBK / GB2312
# ---------------------------------------------------------------------------

_META_CHARSET_RE = re.compile(
    rb'<meta[^>]+charset\s*=\s*["\']?\s*([a-zA-Z0-9_\-]+)', re.IGNORECASE
)


def decode_bytes(content: bytes, content_type: str = "") -> Tuple[str, str]:
    """返回 (text, encoding)。优先级：BOM → meta charset → Content-Type → 探测 → 兜底。"""
    if not content:
        return "", ""

    for bom, encoding in (
        (b"\xef\xbb\xbf", "utf-8-sig"),
        (b"\xff\xfe", "utf-16-le"),
        (b"\xfe\xff", "utf-16-be"),
    ):
        if content.startswith(bom):
            try:
                return content.decode(encoding, errors="replace"), encoding
            except Exception:
                break

    match = _META_CHARSET_RE.search(content[:4096])
    if match:
        declared = match.group(1).decode("ascii", errors="ignore")
        text = _try_decode(content, declared)
        if text is not None:
            return text, _normalize_encoding_name(declared)

    if "charset=" in content_type:
        declared = content_type.split("charset=", 1)[1].split(";")[0].strip().strip('"\'')
        text = _try_decode(content, declared)
        if text is not None:
            return text, _normalize_encoding_name(declared)

    # 严格 UTF-8 优先，避免把 GBK 中文误判成其它编码
    try:
        return content.decode("utf-8"), "utf-8"
    except UnicodeDecodeError:
        pass

    for candidate in ("gb18030", "gbk", "gb2312", "big5"):
        text = _try_decode(content, candidate)
        if text is not None:
            return text, candidate

    return content.decode("utf-8", errors="replace"), "utf-8(replace)"


def _try_decode(content: bytes, encoding: str) -> Optional[str]:
    try:
        return content.decode(encoding)
    except (UnicodeDecodeError, LookupError):
        return None


def _normalize_encoding_name(name: str) -> str:
    lowered = name.lower().replace("_", "-")
    aliases = {
        "gb2312": "gb18030", "gbk": "gb18030", "gb-2312": "gb18030",
        "utf8": "utf-8", "utf-8-sig": "utf-8-sig",
    }
    return aliases.get(lowered, lowered)
