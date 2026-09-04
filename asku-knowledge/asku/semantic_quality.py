"""Evidence-bearing suggestions; automatic classification never approves a record."""

from __future__ import annotations

import hashlib
import re
from typing import Any

from bs4 import BeautifulSoup

from .admission import canonical_text
from .normalizer import _extract_title

RULE_VERSION = "semantic-quality-v1"
EXCLUDED = re.compile(
    r"采购|招标|中标|成交公告|教职工|教师招聘|人才招聘|成员招聘|先进工作者|慰问信|教研项目|教学成果奖|开放课题|软件正版化检查|楼宇网络建设规范"
)
NEWS = re.compile(
    r"召开|顺利举行|顺利举办|慰问|风采|事迹|述职|调研|座谈会|主题教育|新闻"
)
RESULT = re.compile(r"名单|结果公示|结果公布|评选结果|审核结果|拟录取")
TEMPLATE = re.compile(r"申请表|审批表|承诺书|推荐书|登记表|汇总表|空白表|模板")
TOPIC_RULES = {
    "recommendation": r"推免|保研|推荐免试|免试攻读|免试推荐",
    "transfer_major": r"转专业|专业分流|大类分流|专业分配",
    "scholarship": r"奖学金|奖（助）学金|奖\(助\)学金",
    "comprehensive_evaluation": r"综合素质测评|综合测评|综测",
    "cet": r"四[、，,]?六级|四、六级|CET(?:[46]|考试)?|大学英语.{0,4}级考试",
    "makeup_retake": r"补考|重修|缓考",
    "thesis_review": r"论文.{0,6}(送审|评阅|盲审|复制比|查重)",
    "thesis_defense": r"论文.{0,4}答辩|毕业答辩|学位答辩",
    "graduate_degree": r"研究生.{0,5}学位|硕士.{0,5}学位|博士.{0,5}学位|学位授予",
    "graduate_training": r"研究生.{0,8}(培养|课程|转导)|硕士研究生培养",
    "course_selection": r"选课|课程预选|补退选|课程补选|学分认定|修读|预排课表",
    "student_status": r"学籍管理|休学|复学|退学|保留学籍|学籍异动",
    "program_plan": r"培养方案|培养计划|教学计划",
    "graduation": r"毕业要求|毕业资格|毕业审核|毕业申请",
    "degree": r"学士学位|本科.{0,6}学位",
    "exam": r"考试安排|考试纪律|考试违规|考场规则",
    "library": r"图书馆|借阅|自习教室|阅览室",
    "campus_network": r"校园网|宽带|网络认证|无线网|VPN|ISP",
    "campus_card": r"校园卡|一卡通",
    "medical": r"医保|就医|校医院|疫苗|医疗费|体检|住院报销",
    "dorm": r"宿舍|住宿|寝室",
    "financial_aid": r"助学金|助学贷款|勤工助学|学费减免|学生资助|无息借款",
    "competition": r"竞赛|机器人大赛|创新大赛|数学建模|挑战杯",
    "innovation_project": r"大创|大学生创新创业|创新创业训练",
    "international": r"留学|公派|国际合作培养|交换生|国际交流|CSC|国际中文教育志愿者",
    "career_procedure": r"三方协议|就业协议|就业手续|毕业生档案|派遣|报到证",
    "academic_calendar": r"校历|学期安排|开学安排",
    "holiday": r"放假安排|假期安排|寒暑假",
    "freshman": r"新生.{0,5}(报到|入学|注册)|迎新",
    "discipline": r"违纪|处分|作弊|学术不端",
    "party_youthleague": r"入党|入团|团组织关系|党组织关系",
}


def normalize_artifact(text: str, title: str, attachment: bool) -> str:
    """Only whitespace/header changes; do not rewrite policy wording or numbers."""
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    if not attachment and text.startswith("# ") and title:
        text = "# " + title + "\n" + text.partition("\n")[2]
    return re.sub(r"\n{3,}", "\n\n", text).strip()


def title_from_evidence(
    document: dict, text: str, raw_html: bytes | None = None
) -> tuple[str, str]:
    if raw_html:
        title = _extract_title(BeautifulSoup(raw_html, "lxml"))
        if title and not re.search(
            r"欢迎|^武汉理工大学.{0,16}(学院|医院|信息网)$", title
        ):
            return title, "raw_html_heading"
    original = document.get("title") or ""
    if document.get("is_attachment"):
        lines = [
            re.sub(r"^#+\s*", "", line.strip())
            for line in text.splitlines()
            if line.strip()
        ]
        candidates = []
        for index, line in enumerate(lines[:8]):
            if line.startswith(("|", ">", "附件", "序号")):
                continue
            joined = line
            if (
                index
                and len(lines[index - 1]) < 12
                and not re.search(r"\d", lines[index - 1])
            ):
                joined = lines[index - 1] + line
            if 8 <= len(joined) <= 140 and re.search(
                r"办法|规定|目录|申请|承诺|推荐|通知|细则|指南|方案|计划|证明|表|说明|学年|校历",
                joined,
            ):
                candidates.append(joined)
        if candidates:
            return candidates[0], "attachment_first_heading"
    return re.sub(r"[-—]武汉理工大学[^\n]*$", "", original).strip(), "legacy_title"


def suggest(document: dict, text: str, title: str, taxonomy: dict) -> dict[str, Any]:
    compact = canonical_text(text)
    # Joining whitespace is for matching only; the source body stays intact.
    heading = re.sub(r"\s+", "", title)
    intro = re.sub(r"\s+", "", compact[:1800])
    kind = "UNKNOWN"
    reasons = []
    if EXCLUDED.search(heading):
        kind = "NON_STUDENT"
        reasons.append("non_student_subject")
    elif RESULT.search(heading):
        kind = "RESULT_ANNOUNCEMENT"
        reasons.append("result_not_policy")
    elif TEMPLATE.search(heading):
        kind = "FORM_TEMPLATE"
    elif NEWS.search(heading) and not re.search(
        r"关于.{0,30}(报名|申请|考试|选课|推免|转专业)", heading
    ):
        kind = "NEWS"
        reasons.append("news_not_policy")
    elif re.search(r"办法|细则|规定|条例|章程|规范|工作方案|实施方案", heading):
        kind = "POLICY_REGULATION"
    elif re.search(r"指南|办理|查询|操作说明|服务", heading):
        kind = "PROCEDURE_GUIDE"
    elif re.search(r"通知|安排|公告|招生简章", heading):
        kind = "ANNUAL_NOTICE"
    elif re.search(r"培养方案|培养计划", heading):
        kind = "PROGRAM_PLAN"
    elif "目录" in heading:
        kind = "APPLICATION_MATERIAL"
    hits = [
        (topic, m.group())
        for topic, pattern in TOPIC_RULES.items()
        if topic in taxonomy["secondary_topics"]
        and (m := re.search(pattern, heading, re.I))
    ]
    origin = "title"
    if not hits and kind not in {"NEWS", "NON_STUDENT", "RESULT_ANNOUNCEMENT"}:
        hits = [
            (topic, m.group())
            for topic, pattern in TOPIC_RULES.items()
            if topic in taxonomy["secondary_topics"]
            and (m := re.search(pattern, intro, re.I))
        ]
        origin = "body_intro"
    # Explicit task beats incidental terms (e.g. CET as a recommendation criterion).
    selected = hits[0][0] if hits else "other"
    if selected == "other":
        reasons.append("semantic_topic_unresolved")
    if len(hits) > 1:
        reasons.append("multiple_topic_signals")
    if kind == "UNKNOWN":
        reasons.append("content_role_unresolved")
    if kind == "NON_STUDENT":
        audience, education = "TEACHER", "UNKNOWN"
    elif selected in {"recommendation", "transfer_major", "degree"} and not re.search(
        r"接收|招生简章", heading
    ):
        audience, education = "UNDERGRADUATE", "UNDERGRADUATE"
    else:
        under = bool(
            re.search(r"本科生|本科学生|本科毕业生|本专科", heading + intro[:400])
        )
        grad = bool(re.search(r"研究生|硕士生|博士生", heading + intro[:400]))
        audience, education = (
            ("ALL", "BOTH")
            if under and grad
            else ("UNDERGRADUATE", "UNDERGRADUATE")
            if under
            else ("GRADUATE", "GRADUATE")
            if grad
            else ("UNKNOWN", "UNKNOWN")
        )
    versions = sorted(
        set(re.findall(r"[（(]?((?:19|20)\d{2})\s*(?:年)?\s*版", compact[:2000]))
    )
    family = re.sub(r"[（(]?\d{4}\s*(?:年)?\s*版[）)]?", "", heading)
    family = re.sub(r"20\d{2}(?:[-—]20\d{2})?(?:学年|年度|年|级|届)?", "{year}", family)
    family_id = (
        hashlib.sha256((document["school_id"] + "\0" + family).encode()).hexdigest()[
            :24
        ]
        if family
        else ""
    )
    date_mentions = []
    for match in re.finditer(
        r"(20\d{2})\s*年\s*(\d{1,2})\s*月\s*(\d{1,2})\s*日", compact
    ):
        date_mentions.append(
            {
                "text": match.group(),
                "context": compact[max(0, match.start() - 45) : match.end() + 65],
            }
        )
    return {
        "title": title,
        "topic": selected,
        "primary_module": taxonomy["secondary_topics"][selected]["primary_module"],
        "audience": audience,
        "education_level": education,
        "content_role": kind,
        "evidence": [
            {"field": "topic", "origin": origin, "match": match, "topic": topic}
            for topic, match in hits
        ],
        "version_labels": versions,
        "version_family": family_id,
        "date_mentions": date_mentions[:40],
        "risk_reasons": reasons,
        "rule_version": RULE_VERSION,
    }
