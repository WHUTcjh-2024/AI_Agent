"""PII 检测（约束 #5）。

学生名单、学号、手机号、身份证、个人成绩等内容一律标记为
PII_REVIEW + NO_RAG，禁止直接进入 WeKnora。

设计要点（避免误杀）：
  - 政策文件里出现"学号"作为字段名是正常的，不算 PII；
    只有出现"真实学号数据"（成组出现、或与姓名同处一张表）才判定 PII。
  - 通知里留一个办公电话/手机号属于公开联系方式，不算 PII；
    成列出现三个以上手机号，或手机号与姓名/学号同行，才算名册类 PII。
  - 身份证号走严格 18 位 + 校验位验证，几乎不会误判。

判定结果：
  risk = NONE / LOW / HIGH
  HIGH 或 LOW 命中"强名册信号" → 必须 PII_REVIEW + NO_RAG
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Dict, List, Optional

# ---------------------------------------------------------------------------
# 身份证：18 位 + 校验位
# ---------------------------------------------------------------------------
_ID_CARD_RE = re.compile(r"(?<!\d)([1-9]\d{5})(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])(\d{3})([\dXx])(?!\d)")
_ID_WEIGHTS = (7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2)
_ID_CHECK = "10X98765432"


def _valid_id_card(candidate: str) -> bool:
    if len(candidate) != 18:
        return False
    total = 0
    for index in range(17):
        digit = candidate[index]
        if not digit.isdigit():
            return False
        total += int(digit) * _ID_WEIGHTS[index]
    return _ID_CHECK[total % 11] == candidate[17].upper()


# ---------------------------------------------------------------------------
# 手机号
# ---------------------------------------------------------------------------
_MOBILE_RE = re.compile(r"(?<!\d)(1[3-9]\d{9})(?!\d)")

# ---------------------------------------------------------------------------
# 学号：常见 10~13 位数字，且必须以"学号"等上下文词引导，避免误伤普通数字
# ---------------------------------------------------------------------------
_STUDENT_ID_CONTEXT = ("学号", "考生号", "学籍号", "校园卡号", "一卡通号")
_STUDENT_ID_RE = re.compile(r"(?<!\d)(\d{10,13})(?!\d)")

# ---------------------------------------------------------------------------
# 名单 / 成绩 表格信号
# ---------------------------------------------------------------------------
_ROSTER_HEADER_RE = re.compile(
    r"(姓名|学号|班级|专业|学院|身份证|联系方式|手机号|成绩|绩点|学分|排名|综合测评|评语)",
)
_ROSTER_STRONG_RE = re.compile(r"(名单|花名册|成绩单|成绩表|成绩汇总|排名表|录取名单|获评名单|公示名单)")
_SCORE_ROW_RE = re.compile(r"(?<!\d)(100(?:\.0+)?|\d{1,2}(?:\.\d{1,2})?)\s*(?:分|/100)?")

_NAME_ROW_RE = re.compile(
    r"^\s*[\u4e00-\u9fa5]{2,4}\s*[|,，\t　 ]+\s*[\u4e00-\u9fa5\w]{2,20}\s*[|,，\t　 ]+\s*\d{6,}"
)


@dataclass
class PiiFinding:
    categories: List[str] = field(default_factory=list)
    match_count: int = 0
    risk: str = "NONE"
    snippet: str = ""
    detail: Dict[str, int] = field(default_factory=dict)

    @property
    def is_blocking(self) -> bool:
        return self.risk in ("HIGH", "MEDIUM")


def _snippet_around(text: str, start: int, end: int, window: int = 40) -> str:
    left = max(0, start - window)
    right = min(len(text), end + window)
    fragment = text[left:right].replace("\n", " ")
    return re.sub(r"\s+", " ", fragment).strip()


def detect_pii(
    text: str,
    *,
    title: str = "",
    tables_markdown: str = "",
) -> PiiFinding:
    """检测文本中的学生个人信息。返回分类、命中数与风险等级。"""
    if not text:
        return PiiFinding()

    detail: Dict[str, int] = {}
    categories: List[str] = []
    snippet = ""
    total = 0

    # 1) 身份证号（强信号，几乎无误判）
    id_cards: List[str] = []
    for match in _ID_CARD_RE.finditer(text):
        candidate = match.group(0)
        if _valid_id_card(candidate):
            id_cards.append(candidate)
    if id_cards:
        detail["id_card"] = len(id_cards)
        categories.append("id_card")
        total += len(id_cards)
        if not snippet:
            snippet = _snippet_around(text, text.find(id_cards[0]), text.find(id_cards[0]) + 18)

    # 2) 学号（必须与上下文词同行出现，避免误伤）
    student_ids = 0
    for keyword in _STUDENT_ID_CONTEXT:
        for match in re.finditer(re.escape(keyword) + r"\s*[:：]?\s*(\d{8,20})", text):
            student_ids += 1
    # 表格行内出现"姓名 | 学号 | ..."这类结构时，成组数字视为学号
    if tables_markdown and _ROSTER_HEADER_RE.search(tables_markdown):
        rows = [row for row in tables_markdown.split("\n") if row.strip().startswith("|")]
        for row in rows:
            digits = re.findall(r"(?<!\d)(\d{8,20})(?!\d)", row)
            if len(digits) >= 2:
                student_ids += 1
    if student_ids:
        detail["student_id"] = student_ids
        categories.append("student_id")
        total += student_ids
        if not snippet:
            match = re.search(_STUDENT_ID_CONTEXT[0], text)
            if match:
                snippet = _snippet_around(text, match.start(), match.end())

    # 3) 手机号：单个可能是公开联系方式，成组出现才是名册
    mobiles = _MOBILE_RE.findall(text)
    mobile_rows = 0
    for line in text.splitlines():
        if len(_MOBILE_RE.findall(line)) >= 1 and re.search(r"[\u4e00-\u9fa5]{2,4}", line):
            mobile_rows += 1
    if len(mobiles) >= 3 or (mobiles and student_ids) or (mobiles and _ROSTER_STRONG_RE.search(title or "")):
        detail["mobile"] = len(mobiles)
        categories.append("mobile")
        total += len(mobiles)
        if not snippet:
            match = _MOBILE_RE.search(text)
            if match:
                snippet = _snippet_around(text, match.start(), match.end())

    # 4) 名单 / 成绩单：标题或正文出现强名册信号，且表格里含姓名/成绩列
    roster_signal = bool(_ROSTER_STRONG_RE.search(title or "")) or len(_ROSTER_STRONG_RE.findall(text)) >= 2
    has_name_column = bool(tables_markdown and re.search(r"\|\s*姓名\s*\|", tables_markdown))
    has_score_column = bool(tables_markdown and re.search(r"\|\s*(成绩|绩点|学分|综合测评|排名)\s*\|", tables_markdown))

    if roster_signal and (has_name_column or has_score_column):
        rows = [row for row in tables_markdown.split("\n") if row.strip().startswith("|")]
        detail["roster_table"] = max(0, len(rows) - 2)
        categories.append("student_roster" if has_name_column else "personal_score")
        total += max(0, len(rows) - 2)
        if not snippet:
            snippet = (title or "")[:120]

    # 同名同行的"姓名 + 学号/成绩"行
    name_rows = sum(1 for line in text.splitlines() if _NAME_ROW_RE.match(line))
    if name_rows >= 3:
        detail["name_rows"] = name_rows
        categories.append("student_roster")
        total += name_rows

    if not categories:
        return PiiFinding()

    # 风险判定
    strong = {"id_card", "student_id", "student_roster", "personal_score"}
    if any(category in strong for category in categories):
        risk = "HIGH"
    elif detail.get("mobile", 0) >= 5:
        risk = "HIGH"
    else:
        risk = "MEDIUM"

    return PiiFinding(
        categories=sorted(set(categories)),
        match_count=total,
        risk=risk,
        snippet=snippet[:240],
        detail=detail,
    )


def apply_pii_policy(finding: PiiFinding) -> Dict[str, object]:
    """把检测结果转成落库字段（约束 #5：PII_REVIEW + NO_RAG）。"""
    if not finding.is_blocking:
        return {}
    return {
        "review_status": "PII_REVIEW",
        "rag_eligible": False,
        "pii_detected": True,
        "pii_categories": finding.categories,
        "rejection_reason": "PII:" + ",".join(finding.categories),
    }
