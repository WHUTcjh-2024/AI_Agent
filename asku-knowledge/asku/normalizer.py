"""HTML → Normalized Markdown。

§15 清洗规则：
  删除：页面导航、Footer、菜单、友情链接、CSS、JS、推荐新闻、重复栏目、无意义模板
  保留：标题、正文、章节标题、表格、发布时间、发布单位、联系人、联系电话、邮箱、
        时间节点、申请条件、操作流程、附件、重要链接

  尽可能保留 Heading 层级 —— 这是后续 WeKnora Chunk / Citation 的重要上下文。

§14 原始 HTML 永久保留在 raw/webpages，本模块只产出 normalized/webpages/*.md。
"""

from __future__ import annotations

import re
from datetime import date
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Tuple

from bs4 import BeautifulSoup, NavigableString, Tag

# ---------------------------------------------------------------------------
# 噪声选择器：命中即整块丢弃
# ---------------------------------------------------------------------------
NOISE_SELECTORS = [
    "script", "style", "noscript", "iframe", "svg", "canvas",
    "form", "input", "select", "textarea", "button",
    "[class*=nav]", "[class*=menu]", "[class*=footer]", "[class*=header]",
    "[class*=friend]", "[class*=link-list]", "[class*=bread]",
    "[id*=nav]", "[id*=menu]", "[id*=footer]", "[id*=friend]",
    "[class*=slide]", "[class*=banner]", "[class*=carousel]",
    "[class*=share]", "[class*=qrcode]", "[class*=weixin]",
    "[class*=comment]", "[class*=stat]", "[class*=counter]",
]

# 中文站点常见噪声文案
NOISE_TEXT_PATTERNS = [
    re.compile(r"^\s*(上一条|下一条|上一篇|下一篇|返回顶部|打印本页|关闭窗口)\s*$"),
    re.compile(r"^\s*(Copyright|版权所有|技术支持|浏览人次|您是第.*位访客)"),
    re.compile(r"^\s*(首页\s*[-–>]{1,2}\s*)"),
]

DATE_PATTERNS = [
    re.compile(r"(20\d{2})\s*[-年/.]\s*(\d{1,2})\s*[-月/.]\s*(\d{1,2})"),
    re.compile(r"(20\d{2})\s*年\s*(\d{1,2})\s*月"),
]

PUBLISH_HINTS = ["发布时间", "发布日期", "发表时间", "发布：", "日期：", "时间："]
DEPARTMENT_HINTS = ["发布部门", "发布单位", "信息来源", "来源：", "作者：", "编辑：", "撰稿"]


@dataclass
class NormalizedPage:
    title: str
    markdown: str
    text: str
    publish_date: Optional[str]
    department: str
    contacts: List[str] = field(default_factory=list)
    phones: List[str] = field(default_factory=list)
    emails: List[str] = field(default_factory=list)
    attachment_links: List[Dict[str, str]] = field(default_factory=list)
    important_links: List[Dict[str, str]] = field(default_factory=list)
    heading_outline: List[str] = field(default_factory=list)
    table_count: int = 0
    content_chars: int = 0


def _clean_text(value: str) -> str:
    value = value.replace("\u3000", " ").replace("\xa0", " ")
    value = re.sub(r"[ \t]+", " ", value)
    value = re.sub(r"\n{3,}", "\n\n", value)
    return value.strip()


def _is_noise_text(text: str) -> bool:
    for pattern in NOISE_TEXT_PATTERNS:
        if pattern.match(text):
            return True
    return False


def _extract_title(soup: BeautifulSoup, fallback: str = "") -> str:
    for selector in ("h1", "title", "[class*=title]", "[id*=title]"):
        node = soup.select_one(selector)
        if node:
            text = _clean_text(node.get_text(" ", strip=True))
            if text:
                return text[:300]
    return fallback


def _extract_meta(soup: BeautifulSoup, names: List[str]) -> str:
    for name in names:
        node = soup.find("meta", attrs={"name": name})
        if node and node.get("content"):
            return _clean_text(node["content"])
    return ""


def _extract_date(soup: BeautifulSoup, text: str) -> Optional[str]:
    """§43：只在有可靠证据时给出日期，绝不从 URL 里的年份猜测。"""
    # 1) meta 里的发布时间
    for name in ("publishdate", "pubdate", "publish_date", "article:published_time", "date"):
        node = soup.find("meta", attrs={"name": name}) or soup.find("meta", attrs={"property": name})
        if node and node.get("content"):
            content = node["content"].strip()
            match = DATE_PATTERNS[0].search(content)
            if match:
                return _format_date(*match.groups())

    # 2) 正文里带"发布时间"提示的片段
    for hint in PUBLISH_HINTS:
        index = text.find(hint)
        if index >= 0:
            match = DATE_PATTERNS[0].search(text, index, index + 80)
            if match:
                return _format_date(*match.groups())

    # 3) 独立出现在一个小容器里的日期（常见于通知头部/尾部）
    for node in soup.select("[class*=time], [class*=date], [id*=time], [id*=date]"):
        node_text = _clean_text(node.get_text(" ", strip=True))
        if node_text and len(node_text) <= 40:
            match = DATE_PATTERNS[0].search(node_text)
            if match:
                return _format_date(*match.groups())
    return None


def _format_date(year: str, month: str, day: str = "1") -> str:
    try:
        parsed = date(int(year), int(month), int(day))
        return parsed.isoformat()
    except ValueError:
        return ""


def _extract_department(soup: BeautifulSoup, text: str) -> str:
    for hint in DEPARTMENT_HINTS:
        index = text.find(hint)
        if index >= 0:
            segment = text[index + len(hint): index + len(hint) + 60]
            segment = segment.split("\n")[0].strip(" ：:|")
            if segment:
                return segment[:60]
    return _extract_meta(soup, ["author", "source"])


def _table_to_markdown(table: Tag) -> str:
    rows: List[List[str]] = []
    for tr in table.find_all("tr"):
        cells = []
        for cell in tr.find_all(["td", "th"]):
            cells.append(_clean_text(cell.get_text(" ", strip=True)).replace("|", "\\|"))
        if cells:
            rows.append(cells)
    if not rows:
        return ""
    width = max(len(row) for row in rows)
    rows = [row + [""] * (width - len(row)) for row in rows]
    header = rows[0]
    lines = ["| " + " | ".join(header) + " |",
             "| " + " | ".join(["---"] * width) + " |"]
    for row in rows[1:]:
        lines.append("| " + " | ".join(row) + " |")
    return "\n".join(lines)


def _node_to_markdown(node: Tag, depth: int = 0) -> Tuple[str, int]:
    """把正文节点转成 Markdown，返回 (markdown, 表格数)。"""
    parts: List[str] = []
    tables = 0

    for child in node.children:
        if isinstance(child, NavigableString):
            text = _clean_text(str(child))
            if text and not _is_noise_text(text):
                parts.append(text)
            continue
        if not isinstance(child, Tag):
            continue

        name = child.name.lower()
        if name in ("table",):
            markdown_table = _table_to_markdown(child)
            if markdown_table:
                parts.append("\n" + markdown_table + "\n")
                tables += 1
            continue
        if name in ("h1", "h2", "h3", "h4", "h5", "h6"):
            level = int(name[1])
            text = _clean_text(child.get_text(" ", strip=True))
            if text and not _is_noise_text(text):
                parts.append(f"\n{'#' * min(level + 1, 6)} {text}\n")
            continue
        if name in ("p", "div", "section", "article", "blockquote"):
            inner, inner_tables = _node_to_markdown(child, depth + 1)
            tables += inner_tables
            if inner:
                parts.append("\n" + inner + "\n")
            continue
        if name in ("br",):
            parts.append("\n")
            continue
        if name in ("ul", "ol"):
            items = []
            for li in child.find_all("li", recursive=False):
                text = _clean_text(li.get_text(" ", strip=True))
                if text and not _is_noise_text(text):
                    items.append(f"- {text}")
            if items:
                parts.append("\n" + "\n".join(items) + "\n")
            continue
        if name in ("img",):
            continue
        # 其它标签递归取文本
        inner, inner_tables = _node_to_markdown(child, depth + 1)
        tables += inner_tables
        if inner:
            parts.append(inner)

    return _clean_text("\n".join(parts)), tables


def _pick_main_container(soup: BeautifulSoup) -> Tag:
    """选择正文主容器：优先常见内容类名，其次取文本最密集的块。"""
    preferred = [
        "[class*=content]", "[class*=article]", "[class*=detail]",
        "[id*=content]", "[id*=article]", "[id*=detail]",
        "article", "main",
    ]
    for selector in preferred:
        node = soup.select_one(selector)
        if node:
            text = node.get_text(" ", strip=True)
            if len(text) > 200:
                return node

    # 兜底：找文本最长的 div
    best, best_len = soup.body or soup, 0
    for node in soup.find_all(["div", "td", "section"]):
        text = node.get_text(" ", strip=True)
        if len(text) > best_len:
            best, best_len = node, len(text)
    return best


def normalize_html(
    html: str,
    *,
    base_url: str = "",
    allowed_attachment_extensions: Optional[List[str]] = None,
    attachment_url_hints: Optional[List[str]] = None,
) -> NormalizedPage:
    """把原始 HTML 转成 Normalized Markdown 并抽取结构化元数据。"""
    from urllib.parse import urljoin

    allowed_extensions = set(allowed_attachment_extensions or [])
    hints = attachment_url_hints or []
    soup = BeautifulSoup(html, "lxml")

    title = _extract_title(soup)

    # 删除噪声节点
    for selector in NOISE_SELECTORS:
        for node in soup.select(selector):
            node.decompose()

    # 收集附件与重要链接（必须在删除 body 噪声前做）
    attachments: List[Dict[str, str]] = []
    important_links: List[Dict[str, str]] = []
    if base_url:
        for anchor in soup.find_all("a", href=True):
            href = urljoin(base_url, anchor["href"])
            text = _clean_text(anchor.get_text(" ", strip=True))
            extension = href.rsplit(".", 1)[-1].lower() if "." in href.rsplit("/", 1)[-1] else ""
            lowered = href.lower()
            matches_hint = any(_matches_url_hint(lowered, hint) for hint in hints)
            if extension in allowed_extensions or matches_hint:
                attachments.append({"url": href, "text": text or extension.upper()})
            elif text and len(text) >= 6:
                important_links.append({"url": href, "text": text})

    container = _pick_main_container(soup)
    markdown, tables = _node_to_markdown(container)
    plain = soup.get_text("\n", strip=True)

    publish_date = _extract_date(soup, plain)
    department = _extract_department(soup, plain)

    phones = sorted(set(re.findall(r"(?:\d{3,4}-)?\d{7,8}(?:-\d{1,4})?", plain)))[:10]
    phones = [p for p in phones if len(re.sub(r"\D", "", p)) >= 7][:5]
    emails = sorted(set(re.findall(r"[\w.\-]+@[\w.\-]+\.\w+", plain)))[:5]
    contacts = sorted(set(re.findall(r"[\u4e00-\u9fa5]{2,4}(?:老师|主任|科长|老师)", plain)))[:5]

    outline = [
        _clean_text(node.get_text(" ", strip=True))
        for node in container.find_all(["h1", "h2", "h3", "h4"])
        if _clean_text(node.get_text(" ", strip=True))
    ][:30]

    markdown_body = _clean_text(markdown)
    if title:
        markdown_body = f"# {title}\n\n{markdown_body}"
    if publish_date:
        markdown_body += f"\n\n> 发布时间：{publish_date}"
    if department:
        markdown_body += f"\n> 发布单位：{department}"
    if attachments:
        lines = ["", "## 附件"]
        for item in attachments[:30]:
            lines.append(f"- [{item['text']}]({item['url']})")
        markdown_body += "\n" + "\n".join(lines)

    return NormalizedPage(
        title=title,
        markdown=_clean_text(markdown_body),
        text=_clean_text(plain),
        publish_date=publish_date,
        department=department,
        contacts=contacts,
        phones=phones,
        emails=emails,
        attachment_links=attachments,
        important_links=important_links[:30],
        heading_outline=outline,
        table_count=tables,
        content_chars=len(_clean_text(plain)),
    )


def _matches_url_hint(url: str, hint: str) -> bool:
    """Match configured URL hints as regexes, falling back to literals."""
    try:
        return re.search(hint, url, flags=re.IGNORECASE) is not None
    except re.error:
        return hint.lower() in url.lower()
