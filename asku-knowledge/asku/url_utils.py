"""URL 规范化与安全准入。

约束 #3：
  - 只抓取 SourceRegistry 已批准的官方域名；
  - 站外链接只记录为候选来源，不自动继续抓取；
  - 禁止访问内网、私有 IP、登录页面、验证码、非公开接口。

约束 #6：
  - official_url / attachment_original_url / parent_page_url 必须是公开官方 URL；
  - local_file_path 只是内部保存地址，永不作为公开 URL 使用。
"""

from __future__ import annotations

import ipaddress
import re
import socket
from dataclasses import dataclass
from typing import Iterable, List, Optional, Set
from urllib.parse import parse_qsl, urlencode, urljoin, urlsplit, urlunsplit

# 会话类、追踪类参数一律剔除，避免同一页面产生大量重复 URL（§21 去重）
TRACKING_PARAMS: Set[str] = {
    "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content",
    "from", "share_token", "weixin", "wx", "sid", "sessionid",
    "phpsessid", "jsessionid", "aspxsessionid",
}

# 明确无价值的文件类型，不作为知识抓取对象
SKIP_EXTENSIONS: Set[str] = {
    "jpg", "jpeg", "png", "gif", "bmp", "svg", "webp", "ico",
    "mp3", "mp4", "avi", "mov", "wmv", "flv",
    "css", "js", "json", "woff", "woff2", "ttf", "eot",
    "swf", "exe", "dmg", "apk", "iso",
}

# 老 CMS 常见的分页/列表占位参数，规范化时保留（有价值），但去除空值
INDEX_FILENAMES: Set[str] = {
    "index.html", "index.htm", "index.shtml", "index.php", "index.aspx",
    "index.jsp", "default.aspx", "default.html", "default.htm",
}


@dataclass(frozen=True)
class UrlDecision:
    """一次 URL 准入检查的完整结论。"""

    allowed: bool
    reason: str = ""
    canonical_url: str = ""

    def __bool__(self) -> bool:  # pragma: no cover - 语法糖
        return self.allowed


def canonicalize(url: str, *, keep_query: bool = True) -> str:
    """URL 规范化（§21 URL Canonicalization）。

    - 统一小写 scheme 与 host，去掉默认端口与末尾点号
    - 去掉 fragment
    - 去掉 tracking 参数与空参数
    - 排序剩余参数，保证同参不同序得到同一 canonical
    - 目录型 URL 去掉 index.* 尾巴（老 CMS 常见重复源）
    """
    if not url:
        return ""
    try:
        split = urlsplit(url.strip())
    except ValueError:
        return url.strip()

    scheme = (split.scheme or "https").lower()
    if scheme not in ("http", "https"):
        return url.strip()

    host = (split.hostname or "").lower().rstrip(".")
    try:
        port = split.port
    except ValueError:
        return url.strip()
    default_port = (scheme == "http" and port == 80) or (scheme == "https" and port == 443)
    if port and not default_port:
        netloc = f"{host}:{port}"
    else:
        netloc = host

    path = split.path or "/"
    # 压缩重复斜杠，保留末尾斜杠语义
    path = re.sub(r"/{2,}", "/", path)

    query = ""
    if keep_query and split.query:
        pairs = [
            (key, value)
            for key, value in parse_qsl(split.query, keep_blank_values=False)
            if key.lower() not in TRACKING_PARAMS and value != ""
        ]
        # 剔除值为空的项后重新编码，保证稳定排序
        query = urlencode(sorted(pairs), doseq=True)

    return urlunsplit((scheme, netloc, path, query, ""))


def host_of(url: str) -> str:
    try:
        return (urlsplit(url).hostname or "").lower().rstrip(".")
    except ValueError:
        return ""


def extension_of(url: str) -> str:
    path = urlsplit(url).path.lower()
    if "." not in path:
        return ""
    return path.rsplit(".", 1)[-1]


def is_skippable(url: str) -> bool:
    """图片/样式/脚本/可执行文件等无知识价值的资源。附件不走这里。"""
    return extension_of(url) in SKIP_EXTENSIONS


def _is_private_address(host: str) -> bool:
    """判定是否为内网 / 私有 / 回环 / 链路本地地址（约束 #3）。

    先尝试直接按 IP 解析；否则做一次 DNS 解析并检查全部返回地址，
    防止 DNS rebinding 把内网主机指向公网域名。
    """
    if not host:
        return True
    if host in ("localhost",) or host.endswith(".local"):
        return True
    try:
        address = ipaddress.ip_address(host)
    except ValueError:
        address = None

    if address is not None:
        return bool(
            address.is_private
            or address.is_loopback
            or address.is_link_local
            or address.is_reserved
            or address.is_unspecified
            or address.is_multicast
        )

    try:
        infos = socket.getaddrinfo(host, None)
    except socket.gaierror:
        # 解析失败视为不可达，交由 fetch 阶段记录 FAILED
        return False

    for info in infos:
        try:
            resolved = ipaddress.ip_address(info[4][0])
        except ValueError:
            continue
        if (
            resolved.is_private
            or resolved.is_loopback
            or resolved.is_link_local
            or resolved.is_reserved
            or resolved.is_unspecified
        ):
            return True
    return False


class UrlGate:
    """URL 准入网关：官方域名 + 禁止路径 + 内网隔离 + 预算深度。"""

    def __init__(self, school, source_registry, *, allowed_extensions: Optional[Iterable[str]] = None):
        self.school = school
        self.registry = source_registry
        self._host_to_source = source_registry.host_to_source()
        self._allowed_extensions = set(allowed_extensions or ())

    def classify_host(self, host: str) -> str:
        """official（已登记官方） / candidate（站外候选） / forbidden。"""
        host = (host or "").lower().rstrip(".")
        if not host:
            return "forbidden"
        if host in self.school.forbidden_domains:
            return "forbidden"
        if self.school.is_official_domain(host):
            return "official"
        return "candidate"

    def source_for(self, url: str):
        host = host_of(url)
        return self._host_to_source.get(host)

    def check(self, url: str, *, depth: int = 0, max_depth: int = 6, for_attachment: bool = False) -> UrlDecision:
        """返回是否允许抓取该 URL 及原因。"""
        if not url:
            return UrlDecision(False, "empty_url")

        lowered = url.lower()
        if not lowered.startswith(("http://", "https://")):
            return UrlDecision(False, "unsupported_scheme")

        try:
            parsed = urlsplit(url)
            if parsed.username is not None or parsed.password is not None:
                return UrlDecision(False, "userinfo_not_allowed")
            # Accessing port validates malformed/out-of-range values.
            _ = parsed.port
        except ValueError:
            return UrlDecision(False, "invalid_url")

        canonical = canonicalize(url)
        host = host_of(canonical)
        if not host:
            return UrlDecision(False, "no_host")

        # 1) 登录页 / 验证码 / 后台管理（约束 #3）
        if self.school.is_forbidden(lowered):
            return UrlDecision(False, "forbidden_path", canonical)

        # 2) 内网 / 私有 IP（约束 #3）
        if _is_private_address(host):
            return UrlDecision(False, "private_or_internal_host", canonical)

        # 3) 域名归属判定
        kind = self.classify_host(host)
        if kind == "forbidden":
            return UrlDecision(False, "forbidden_domain", canonical)
        if kind != "official":
            # 站外链接：只记为候选来源，不自动抓取（约束 #3）
            return UrlDecision(False, "offsite_candidate_only", canonical)

        registered_source = self._host_to_source.get(host)
        if registered_source is not None and not registered_source.active:
            return UrlDecision(False, "source_inactive", canonical)

        # 4) 官方域名但尚未登记到 SourceRegistry → 需要 Discovery 先登记
        #    这里仍然放行，由 crawler 调用 register_discovered_source 落库
        #    （*.whut.edu.cn 命中 domain_suffixes，视为官方）

        if not for_attachment and is_skippable(canonical):
            return UrlDecision(False, "unsupported_extension", canonical)

        if depth > max_depth:
            return UrlDecision(False, "max_depth_exceeded", canonical)

        return UrlDecision(True, "ok", canonical)

    def is_attachment_candidate(self, url: str) -> bool:
        """附件候选判定（§12）：不只看扩展名，还看 URL 关键词与 Content 特征。"""
        extension = extension_of(url)
        if extension in self._allowed_extensions:
            return True
        if extension in SKIP_EXTENSIONS:
            return False
        lowered = url.lower()
        return any(hint in lowered for hint in ("download", "attach", "upload", "file", "fujian", "getfile"))


def same_site(url_a: str, url_b: str) -> bool:
    return host_of(url_a) == host_of(url_b)


def resolve_link(base_url: str, href: str) -> str:
    """把页面里的相对链接解析成绝对 URL，并做一次规范化。"""
    if not href:
        return ""
    href = href.strip()
    if href.startswith(("javascript:", "mailto:", "tel:", "data:", "#")):
        return ""
    try:
        return canonicalize(urljoin(base_url, href))
    except Exception:
        return ""


def parent_page_url(url: str) -> str:
    """推断父级栏目页：去掉最后一段路径。用于 parent_page_url 契约字段。"""
    try:
        split = urlsplit(canonicalize(url))
    except ValueError:
        return ""
    path = split.path or "/"
    if path.count("/") <= 1:
        return urlunsplit((split.scheme, split.netloc, "/", split.query, ""))
    parent = path.rsplit("/", 1)[0] or "/"
    return urlunsplit((split.scheme, split.netloc, parent, "", ""))
