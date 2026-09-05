"""Admit reviewed question-first evidence and build an import-ready knowledge pack."""

from __future__ import annotations

import hashlib
import json
import re
import shutil
from collections import Counter, defaultdict
from collections.abc import Iterable
from datetime import UTC, datetime
from pathlib import Path

import yaml

BLOCKING_FLAGS = {
    "parse_failure",
    "no_article_body",
    "unsupported_or_image",
    "unicode_corruption",
    "roster_not_parsed",
    "pdf_ocr_required",
    "pdf_table_layout_review",
    "image_evidence_required",
    "source_scope_unknown",
    "outside_undergraduate_p0",
    "special_audience_requires_review",
    "roster_table",
}

TITLE_GUARDS = {
    "transfer_major": r"转专业",
    "course_selection": r"选课|课程预选|补退选",
    "cet": r"四[、，,]?六级|CET|大学英语.{0,4}(四|六)级",
    "exam": r"课程考试|课程考核|考试安排|成绩复核|学分绩点|考场规则|学籍管理规定",
    "makeup_retake": r"补考|缓考|补（缓）考|重修|重考|免听",
    "student_status": r"学籍管理|休学|复学|退学|转学|延长学习|提前毕业|毕业资格|学士学位|开学注册",
    "scholarship": r"奖学金|奖（助）学金|奖助学金|学生奖励|评先",
    "comprehensive_evaluation": r"综合素质测评|综合测评|综测",
    "academic_calendar": r"校历|开学|寒假|暑假|放假|调休|教学周|考试周",
    "library": r"图书馆|借阅|续借|入馆|座位|研讨室|数据库|校外访问",
    "campus_card": r"校园卡|一卡通",
}

TITLE_EXCLUDES = re.compile(
    r"招聘|拟录取|获奖名单|评选结果|录取结果|转专业结果|成绩公示|成绩汇总|"
    r"教师考核|职称|岗位|竞赛成绩|毕业设计|论文答辩|实验室安全|研究生|招生简章",
)

# A hit means the document contains answer evidence, not merely the topic word.
QUESTION_PATTERNS = {
    "transfer_time": r"(申请|报名).{0,24}(时间|日期|月|日)|时间安排",
    "transfer_eligibility": r"申请条件|报名条件|申请对象|基本条件|基本要求|申请资格",
    "transfer_exclusions": r"不得申请|不能申请|不予受理|不接受|不允许转|限制条件",
    "transfer_cross_college": r"跨学院|跨专业类|跨类转|学院之间|向接收学院|转出学院.{0,30}接收学院",
    "transfer_times": r"在校期间.{0,20}(一次|1次)|只能.{0,12}(一次|1次)|转专业次数|未转过专业|已有过转专业记录",
    "transfer_exam": r"(考核|考试|笔试|面试).{0,24}(安排|时间|地点|内容|方式)|考核办法",
    "transfer_results": r"(结果|名单).{0,16}(公布|公示)|公示.{0,16}(时间|日期)|公布时间",
    "transfer_credit": r"学分认定|课程认定|课程替代|课程免修|补修课程",
    "transfer_materials": r"申请材料|提交材料|申请表|成绩单|材料要求",
    "transfer_capacity": r"接收计划|接收人数|接收名额|计划数|专业容量|转专业计划",
    "course_time": r"选课.{0,20}(时间|阶段|安排|开始|截止)|时间安排.{0,20}选课",
    "course_system": r"教务管理系统|选课系统|选课网址|系统入口|进入.{0,10}选课",
    "course_steps": r"操作流程|操作步骤|操作手册|选课操作|登录.{0,20}选课",
    "course_preselect": r"课程预选|预选阶段|预选轮次",
    "course_add_drop": r"补退选|退补选|退选阶段|补选阶段",
    "course_pe": r"体育课|体育课程|大学体育",
    "course_general": r"通识选修|通识教育.{0,8}选修|通识课",
    "course_cross_major": r"跨专业选课|跨专业.{0,8}课程|专业任选课",
    "course_capacity": r"课程停开|停开课程|课程容量|人数不足|选课人数|额满",
    "course_conflict": r"时间冲突|课程冲突|上课冲突|选课冲突",
    "course_credit_limit": r"学分.{0,8}(上限|下限)|最多.{0,8}学分|不得少于.{0,8}学分",
    "course_arrears": r"欠费|欠学费|缴清学费|学费.{0,8}选课",
    "cet_time": r"报名时间|报名工作|报名.{0,20}(开始|截止|日期)",
    "cet_eligibility": r"报名资格|报名条件|报考资格|报考条件|报名对象",
    "cet_portal": r"cet-bm\.neea\.edu\.cn|报名网站|报名入口|全国大学英语四、六级考试报名网",
    "cet_fee": r"报名费|收费标准|缴费|支付成功",
    "cet_exam": r"考试时间|笔试时间|口试时间|考试日期",
    "cet_ticket": r"准考证.{0,20}(打印|下载)|打印准考证",
    "cet_score": r"成绩.{0,20}(查询|发布时间)|中国教育考试网",
    "cet_oral": r"口语考试|口试报名|CET-SET",
    "cet_late": r"不再补报|逾期.{0,12}(不|无法)|报名截止后|无补报名",
    "cet_score_report": r"电子成绩报告单|成绩报告单.{0,20}(下载|申请|查询)",
    "exam_schedule": r"考试安排|考试时间|考试地点|考场安排|教务管理系统.{0,20}考试",
    "exam_absence": r"无故缺考|旷考|缺考.{0,20}(处理|成绩|后果)",
    "exam_recheck": r"成绩复核|成绩复查|成绩异议|申请复核",
    "exam_gpa": r"平均学分绩点|学分绩点.{0,20}(计算|公式)|课程绩点",
    "exam_id": r"学生证|身份证|有效证件|准考证",
    "exam_late": r"迟到.{0,12}(分钟|不得入场|禁止入场)",
    "makeup_eligibility": r"补考.{0,30}(资格|可以|不得|课程|对象)|参加补考",
    "makeup_time": r"补（缓）考.{0,20}(时间|安排|日期)|补考时间",
    "makeup_score": r"补考成绩|补考.{0,20}(记载|绩点)",
    "deferred_apply": r"缓考.{0,20}(申请|审批|办理)|申请缓考",
    "deferred_materials": r"缓考.{0,24}(证明|材料)|申请缓考.{0,24}(证明|材料)",
    "deferred_timing": r"缓考.{0,24}(考试|补考|安排|时间)|补（缓）考",
    "retake_apply": r"重修.{0,20}(报名|选课|申请|办理)|重修选课",
    "retake_fee": r"重修.{0,20}(缴费|费用|收费)|重修费",
    "retake_exemption": r"重修免听|免听.{0,20}(申请|办理)|申请免听",
    "retake_score": r"重修成绩|重修.{0,20}(记载|绩点)",
    "reexam_distinction": r"重考.{0,20}(补考|重修)|补考.{0,20}重修",
    "status_suspend": r"休学.{0,30}(申请|办理|条件|期限|程序)",
    "status_resume": r"复学.{0,30}(申请|办理|期限|程序)|休学期满.{0,20}复学",
    "status_retain": r"保留学籍.{0,30}(规定|期限|申请|期间)",
    "status_withdraw": r"退学.{0,30}(情形|处理|办理|程序|申请)",
    "status_extend": r"延长学习年限|延长学制|延期毕业.{0,20}(申请|办理)",
    "status_early": r"提前毕业.{0,30}(条件|申请|要求|办理)",
    "status_transfer_school": r"转学.{0,30}(条件|申请|程序|办理)",
    "status_graduate": r"毕业资格|毕业条件|准予毕业|毕业学分|毕业要求",
    "status_degree": r"学士学位.{0,30}(授予|条件|资格)|学位授予条件",
    "status_register": r"注册.{0,30}(请假|报到|手续|办理)|开学注册|逾期不注册",
    "scholarship_national": r"国家奖学金.{0,40}(条件|标准|申请|评审|要求)",
    "scholarship_inspirational": r"国家励志奖学金.{0,40}(条件|标准|申请|评审|要求)",
    "scholarship_school": r"学校奖学金|校级奖学金|卓越奖学金.{0,30}(条件|评定|申请)",
    "scholarship_social": r"社会奖学金|专项奖学金|捐赠奖学金",
    "scholarship_time": r"奖学金.{0,30}(时间|评审|申报|截止)|评审时间",
    "scholarship_score": r"奖学金.{0,40}(成绩|综合测评|综测|排名)|成绩排名",
    "scholarship_materials": r"奖学金.{0,30}(材料|申请表)|申报材料|提交材料",
    "scholarship_publicity": r"奖学金.{0,30}(公示|异议)|公示期.{0,20}(异议|反映)",
    "scholarship_quota": r"奖学金.{0,30}(名额|指标|分配)|名额分配|奖励名额",
    "scholarship_multiple": r"奖学金.{0,30}(兼得|兼获|同时获得|不可兼得)|荣誉兼得",
    "evaluation_formula": r"综合.{0,8}测评.{0,20}(总分|计算)|测评总分.{0,8}=|分值分配",
    "evaluation_moral": r"德育分|思想道德素质分|德育.{0,20}(计算|评分)",
    "evaluation_academic": r"智育分|科学文化素质分|学业水平.{0,20}(计算|评分)",
    "evaluation_sports": r"体育分|健康素质分|体质健康.{0,20}(分|评分)",
    "evaluation_competition": r"竞赛.{0,20}(加分|奖励分)|学科竞赛",
    "evaluation_cadre": r"学生干部.{0,20}(加分|奖励分|评分)|社会工作奖励分",
    "evaluation_volunteer": r"志愿服务|社会实践.{0,20}(加分|奖励分)",
    "evaluation_research": r"科研论文|学术论文|专利.{0,20}(加分|奖励分)",
    "evaluation_penalty": r"处分.{0,20}(扣分|测评)|处罚分|违纪.{0,20}(扣分|处分)",
    "evaluation_appeal": r"测评.{0,20}(申诉|异议)|综测.{0,20}(申诉|异议)",
    "calendar_download": r"校历.{0,20}(下载|附件|查看)|学校校历",
    "calendar_start": r"开学.{0,20}(上课|报到|注册)|正式上课|上课时间",
    "calendar_winter": r"寒假.{0,20}(开始|放假|时间)|寒假前后",
    "calendar_summer": r"暑假.{0,20}(开始|放假|时间)|暑假前后",
    "calendar_weeks": r"教学周|考试周|第.{0,3}周.{0,12}考试",
    "calendar_holiday": r"国庆.{0,20}(放假|调休|安排)|国庆节",
    "library_hours": r"开放时间|开闭馆时间|开馆时间|闭馆时间",
    "library_borrow": r"借阅.{0,20}(册|本|天|期限)|借书.{0,20}(册|本|天|期限)",
    "library_renew": r"续借.{0,24}(办理|期限|一次|规则)|办理续借",
    "library_overdue": r"逾期|图书丢失|遗失.{0,12}图书|违章处理",
    "library_seat": r"座位.{0,20}(预约|预订)|座位预约",
    "library_room": r"研讨室.{0,20}(预约|预订)|空间预约",
    "library_database": r"电子数据库|数据库.{0,20}(访问|使用)|电子资源",
    "library_remote": r"校外访问|校外.{0,20}(数据库|电子资源)|远程访问",
    "library_entry": r"入馆守则|入馆.{0,80}(证件|规定|须知|规则|校园卡)|读者入馆",
    "card_apply": r"校园卡.{0,20}(申领|办理|申请)|新生卡",
    "card_loss": r"挂失|校园卡丢失|校园卡遗失",
    "card_replace": r"补办|补卡|换卡",
    "card_topup": r"充值|圈存|转账充值",
    "card_payment": r"支付方式|扫码支付|消费.{0,12}(方式|场景)",
    "card_password": r"密码.{0,20}(忘记|重置|修改|初始)",
    "card_fault": r"无法使用|故障|冻结|异常.{0,12}校园卡",
    "card_location": r"卡务中心|服务点|办公地点|联系电话",
    "card_unloss": r"解挂|解除挂失|撤销挂失",
}

COLLEGE_NAMES = (
    "材料学院", "材料科学与工程学院", "交通与物流工程学院", "船海与能源动力工程学院",
    "机电工程学院", "能源与动力工程学院", "土木工程与建筑学院", "资源与环境工程学院",
    "信息工程学院", "自动化学院", "计算机与人工智能学院", "数学与统计学院", "物理与力学学院",
    "化学化工与生命科学学院", "管理学院", "经济学院", "法学与人文社会学院", "外国语学院",
    "马克思主义学院", "艺术与设计学院", "体育学院", "国际教育学院", "创业学院",
    "艾克斯马赛学院", "材料示范学院", "航运学院", "汽车工程学院", "安全科学与应急管理学院",
)

# Cross-topic evidence confirmed by direct inspection of these immutable source
# documents. Keeping this list explicit prevents broad keywords from inflating
# coverage across unrelated topics.
SUPPLEMENTAL_TAGS_BY_URL_SUFFIX = {
    "20211012154930_9902.pdf": {
        "course_credit_limit",
        "exam_gpa",
        "makeup_eligibility",
        "makeup_score",
        "deferred_apply",
        "deferred_materials",
        "deferred_timing",
        "retake_apply",
        "retake_exemption",
        "retake_score",
        "reexam_distinction",
    },
}


def sha(data: bytes | str) -> str:
    if isinstance(data, str):
        data = data.encode("utf-8")
    return hashlib.sha256(data).hexdigest()


def load_jsonl(path: Path) -> list[dict]:
    with path.open(encoding="utf-8") as handle:
        return [json.loads(line) for line in handle if line.strip()]


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")


def write_jsonl(path: Path, rows: Iterable[dict]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows), encoding="utf-8", newline="\n")


def compact(text: str) -> str:
    return re.sub(r"\s+", "", text or "")


def evidence_tags(row: dict, questions: dict[str, dict]) -> list[str]:
    haystack = row["title"] + "\n" + row["clean_content"]
    tags = [
        qid for qid, definition in questions.items()
        if definition["topic"] == row.get("topic")
        and re.search(QUESTION_PATTERNS[qid], haystack, re.IGNORECASE | re.DOTALL)
    ]
    if "library_hours" in tags and not re.search(r"开放时间|开闭馆时间|开馆时间", row["title"]):
        tags.remove("library_hours")
    for suffix, supplemental in SUPPLEMENTAL_TAGS_BY_URL_SUFFIX.items():
        if row.get("canonical_url", "").endswith(suffix):
            tags.extend(qid for qid in supplemental if qid in questions)
    return sorted(set(tags))


def normalize_library_policy(row: dict) -> None:
    if "lib.whut.edu.cn/engine2/" not in row.get("canonical_url", ""):
        return
    title, content = row.get("title", ""), row.get("clean_content", "")
    row["topic"] = "library"
    row["topics"] = ["library"]
    if re.fullmatch(r"图管〔2024〕30号", title) and "各类型读者借阅权限" in content:
        row["title"] = "武汉理工大学图书馆图书借阅管理办法（图管〔2024〕30号）"
        row["document_type"] = "regulation"
    elif title == "第一章 前言" and "数字资源" in content:
        row["title"] = "武汉理工大学图书馆数字资源使用管理办法"
        row["document_type"] = "regulation"


def infer_college(row: dict) -> str | None:
    value = row.get("college")
    if value:
        return value
    title = row.get("title", "")
    return next((name for name in COLLEGE_NAMES if name in title), None)


def rejection_reasons(row: dict, tags: list[str]) -> list[str]:
    reasons = []
    flags = set(row.get("quality_flags", []))
    blocking = sorted(flags & BLOCKING_FLAGS)
    if blocking:
        reasons.extend("quality:" + flag for flag in blocking)
    body_len = len(compact(row.get("clean_content", "")))
    if body_len < 120:
        reasons.append("content_too_short")
    if body_len > 120_000:
        reasons.append("bulk_table_or_oversized_content")
    if row.get("document_type") in {"result", "form"}:
        reasons.append("result_or_blank_form")
    if row.get("contains_pii"):
        reasons.append("pii_detected:" + ",".join(row.get("pii_categories", [])))
    title = row.get("title", "")
    if TITLE_EXCLUDES.search(title):
        reasons.append("outside_question_scope")
    guard = TITLE_GUARDS.get(row.get("topic"))
    if not guard or not re.search(guard, title, re.IGNORECASE):
        reasons.append("title_not_topic_specific")
    if not tags:
        reasons.append("no_question_evidence")
    if row.get("authority") not in {"OFFICIAL_SCHOOL", "OFFICIAL_DEPARTMENT", "OFFICIAL_COLLEGE"}:
        reasons.append("authority_not_verified")
    published_year = int(row["published_at"][:4]) if row.get("published_at") else None
    periodic_topics = {
        "transfer_major", "course_selection", "cet", "exam", "makeup_retake",
        "scholarship", "comprehensive_evaluation", "academic_calendar",
    }
    if published_year and published_year < 2025 and row.get("topic") in periodic_topics and row.get("document_type") not in {"regulation", "guide"}:
        reasons.append("historical_periodic_notice")
    if published_year and published_year < 2025 and row.get("topic") == "library" and row.get("scope") == "college":
        reasons.append("stale_college_service_page")
    if published_year and published_year < 2026 and row.get("topic") in {
        "transfer_major", "course_selection", "cet", "exam", "makeup_retake", "academic_calendar"
    } and row.get("document_type") not in {"regulation", "guide"}:
        reasons.append("not_current_cycle")
    if row.get("topic") == "library" and row.get("published_at") and re.search(r"临时|暂停|恢复|故障|封禁", title):
        reasons.append("transient_service_notice")
    return sorted(set(reasons))


def freshness_key(row: dict) -> tuple:
    return (row.get("published_at") or "0000-00-00", row.get("academic_year") or "", row["canonical_url"])


def admission_score(row: dict) -> tuple:
    current_tags = sum(row["question_latest"].values())
    return (
        1 if row["document_type"] in {"regulation", "guide"} else 0,
        current_tags,
        len(row["question_tags"]),
        row.get("published_at") or "0000-00-00",
        len(compact(row["clean_content"])),
    )


def build(
    candidates: list[Path],
    config: Path,
    batch: Path,
    reports: Path,
    export: Path,
    evals: Path,
) -> dict:
    cfg = yaml.safe_load(config.read_text(encoding="utf-8"))
    question_defs = {
        q["id"]: {**q, "topic": topic["id"], "topic_name": topic["name"]}
        for topic in cfg["topics"] for q in topic["questions"]
    }
    missing_patterns = sorted(set(question_defs) - set(QUESTION_PATTERNS))
    if missing_patterns:
        raise ValueError("missing_question_patterns:" + ",".join(missing_patterns))

    raw_rows = [row for path in candidates for row in load_jsonl(path)]
    # Prefer the most recently crawled row for duplicate canonical URLs.
    by_url: dict[str, dict] = {}
    duplicate_rows = []
    for row in raw_rows:
        key = row["canonical_url"]
        if key in by_url:
            loser, winner = (by_url[key], row) if row.get("crawl_at", "") >= by_url[key].get("crawl_at", "") else (row, by_url[key])
            by_url[key] = winner
            duplicate_rows.append((loser, "duplicate_canonical_url"))
        else:
            by_url[key] = row

    prechecked = []
    for row in by_url.values():
        row = dict(row)
        row["college"] = infer_college(row)
        if row["college"]:
            row["scope"] = "college"
            row["authority"] = "OFFICIAL_COLLEGE"
        normalize_library_policy(row)
        tags = evidence_tags(row, question_defs)
        row["question_tags"] = tags
        row["questions"] = [question_defs[qid]["question"] for qid in tags]
        row["rejection_reasons"] = rejection_reasons(row, tags)
        prechecked.append(row)

    eligible = [row for row in prechecked if not row["rejection_reasons"]]
    # Keep one official representation of identical clean content.
    by_content: dict[str, dict] = {}
    for row in sorted(eligible, key=freshness_key, reverse=True):
        content_hash = row["content_hash"]
        if content_hash in by_content:
            row["rejection_reasons"] = ["duplicate_clean_content"]
            duplicate_rows.append((row, "duplicate_clean_content"))
        else:
            by_content[content_hash] = row
    eligible = list(by_content.values())

    latest_by_question: dict[str, str] = {}
    for qid in question_defs:
        matches = [row for row in eligible if qid in row["question_tags"]]
        if matches:
            latest_by_question[qid] = max(matches, key=freshness_key)["asku_document_id"]
    for row in eligible:
        row["question_latest"] = {qid: latest_by_question.get(qid) == row["asku_document_id"] for qid in row["question_tags"]}
        if row["document_type"] in {"regulation", "guide"} and not row.get("published_at"):
            row["version_status"] = "current_unverified_date"
        elif any(row["question_latest"].values()):
            row["version_status"] = "latest_observed"
        else:
            row["version_status"] = "historical"
        row["answer_scope"] = "COLLEGE_ONLY" if row.get("college") else (
            "TIMELESS_POLICY" if row["document_type"] in {"regulation", "guide"} else "DATED_SOURCE_ONLY"
        )

    # Greedy set cover per topic. Keep extra latest college-specific transfer rules.
    accepted: list[dict] = []
    for topic in [topic["id"] for topic in cfg["topics"]]:
        pool = [row for row in eligible if row["topic"] == topic]
        uncovered = {qid for qid, q in question_defs.items() if q["topic"] == topic}
        selected_ids = set()
        while pool:
            pool.sort(key=lambda row: (len(uncovered & set(row["question_tags"])),) + admission_score(row), reverse=True)
            row = pool.pop(0)
            new_tags = uncovered & set(row["question_tags"])
            if not new_tags:
                break
            accepted.append(row)
            selected_ids.add(row["asku_document_id"])
            uncovered -= new_tags
        extras = [
            row for row in eligible
            if row["topic"] == topic and row["asku_document_id"] not in selected_ids
            and any(row["question_latest"].values())
        ]
        if topic == "transfer_major":
            extras.extend(
                row for row in eligible
                if row["topic"] == topic and row.get("college") and row.get("published_at", "").startswith("2026")
                and row["asku_document_id"] not in selected_ids
            )
        topic_cap = 32 if topic == "transfer_major" else 12
        seen = {row["asku_document_id"] for row in accepted if row["topic"] == topic}
        for row in sorted(extras, key=admission_score, reverse=True):
            if row["asku_document_id"] in seen or len(seen) >= topic_cap:
                continue
            accepted.append(row)
            seen.add(row["asku_document_id"])

    accepted = sorted({row["asku_document_id"]: row for row in accepted}.values(), key=lambda row: (row["topic"], row["title"], row["asku_document_id"]))
    accepted_ids = {row["asku_document_id"] for row in accepted}
    for row in prechecked:
        if row["asku_document_id"] in accepted_ids:
            row["review_status"] = "ACCEPTED"
            row["rag_eligible"] = True
            row["rejection_reasons"] = []
        elif not row["rejection_reasons"]:
            row["rejection_reasons"] = ["redundant_or_historical_evidence"]
            row["review_status"] = "REVIEW"
            row["rag_eligible"] = False
        else:
            row["review_status"] = "REVIEW"
            row["rag_eligible"] = False

    batch.mkdir(parents=True, exist_ok=True)
    write_jsonl(batch / "accepted.jsonl", accepted)
    review_rows = sorted(
        ({
            "asku_document_id": row["asku_document_id"],
            "title": row.get("title"),
            "canonical_url": row.get("canonical_url"),
            "topic": row.get("topic"),
            "published_at": row.get("published_at"),
            "quality_flags": row.get("quality_flags", []),
            "reasons": row["rejection_reasons"],
        } for row in prechecked if row["asku_document_id"] not in accepted_ids),
        key=lambda row: (row["topic"], row["title"] or "", row["asku_document_id"]),
    )
    write_jsonl(batch / "review_queue.jsonl", review_rows)

    covered_docs: dict[str, list[dict]] = defaultdict(list)
    for row in accepted:
        for qid in row["question_tags"]:
            covered_docs[qid].append({
                "asku_document_id": row["asku_document_id"],
                "title": row["title"],
                "url": row["canonical_url"],
                "published_at": row.get("published_at"),
                "version_status": row["version_status"],
            })
    question_rows = []
    for qid, definition in question_defs.items():
        evidence = covered_docs.get(qid, [])
        freshness_ok = not definition.get("freshness_required") or any(item["version_status"] == "latest_observed" for item in evidence)
        status = "COVERED" if evidence and freshness_ok else "GAP_REVIEW"
        question_rows.append({
            "question_id": qid,
            "topic": definition["topic"],
            "question": definition["question"],
            "freshness_required": bool(definition.get("freshness_required")),
            "college_required": bool(definition.get("college_required")),
            "status": status,
            "evidence": evidence,
        })
    covered_count = sum(row["status"] == "COVERED" for row in question_rows)
    coverage = {
        "as_of": cfg["as_of"],
        "coverage_definition": "ACCEPTED + rag_eligible + question-specific evidence; freshness questions also require latest observed evidence",
        "questions_total": len(question_rows),
        "questions_covered": covered_count,
        "coverage_rate": round(covered_count / len(question_rows), 4),
        "target_rate": cfg["coverage_target"],
        "target_met": covered_count / len(question_rows) >= cfg["coverage_target"],
        "topics": [],
        "questions": question_rows,
    }
    for topic in cfg["topics"]:
        items = [row for row in question_rows if row["topic"] == topic["id"]]
        count = sum(row["status"] == "COVERED" for row in items)
        coverage["topics"].append({"topic": topic["id"], "name": topic["name"], "covered": count, "total": len(items), "rate": round(count / len(items), 4)})

    reports.mkdir(parents=True, exist_ok=True)
    write_json(reports / "high-frequency-coverage.json", coverage)
    lines = [
        "# 武汉理工大学高频问题覆盖报告",
        "",
        f"统计日期：{cfg['as_of']}  ",
        f"严格覆盖：{covered_count}/{len(question_rows)}（{coverage['coverage_rate']:.1%}）；目标：{cfg['coverage_target']:.0%}  ",
        "口径：仅统计已准入、可进入 RAG、且正文存在问题级证据的文档；时效问题还必须命中本次采集所见最新版。",
        "",
        "| 主题 | 已覆盖 | 问题数 | 覆盖率 |",
        "| --- | ---: | ---: | ---: |",
    ]
    lines.extend(f"| {item['name']} | {item['covered']} | {item['total']} | {item['rate']:.1%} |" for item in coverage["topics"])
    lines.extend(["", "## 待补缺口", ""])
    lines.extend(f"- `{row['question_id']}` {row['question']}" for row in question_rows if row["status"] != "COVERED")
    (reports / "high-frequency-coverage.md").write_text("\n".join(lines) + "\n", encoding="utf-8", newline="\n")
    write_json(reports / "coverage-gap-queue.json", [row for row in question_rows if row["status"] != "COVERED"])

    cleaning = {
        "as_of": cfg["as_of"],
        "input_candidates": len(raw_rows),
        "unique_canonical_urls": len(by_url),
        "accepted_documents": len(accepted),
        "review_documents": len(review_rows),
        "accepted_by_topic": dict(sorted(Counter(row["topic"] for row in accepted).items())),
        "review_reason_counts": dict(sorted(Counter(reason for row in review_rows for reason in row["reasons"]).items())),
        "pii_documents_released": sum(bool(row.get("contains_pii")) for row in accepted),
        "blocking_quality_flags_released": sum(bool(set(row.get("quality_flags", [])) & BLOCKING_FLAGS) for row in accepted),
        "duplicate_content_released": len(accepted) - len({row["content_hash"] for row in accepted}),
        "rules": [
            "official WHUT sources only",
            "no roster, result, detected PII, OCR-dependent, image-dependent, or unverified table-layout evidence",
            "at least one explicit high-frequency question evidence pattern",
            "canonical URL and clean-content SHA-256 deduplication",
            "historical and redundant evidence retained in review queue, excluded from import pack",
        ],
    }
    write_json(reports / "data-cleaning-report.json", cleaning)
    cleaning_md = [
        "# 数据清洗报告", "",
        f"- 输入候选：{cleaning['input_candidates']}",
        f"- 唯一规范 URL：{cleaning['unique_canonical_urls']}",
        f"- 准入文档：{cleaning['accepted_documents']}",
        f"- 复核队列：{cleaning['review_documents']}",
        f"- 准入 PII 文档：{cleaning['pii_documents_released']}",
        f"- 准入阻断质量标记文档：{cleaning['blocking_quality_flags_released']}",
        f"- 准入重复正文：{cleaning['duplicate_content_released']}",
        "", "## 准入主题分布", "",
    ]
    cleaning_md.extend(f"- {topic}: {count}" for topic, count in cleaning["accepted_by_topic"].items())
    (reports / "data-cleaning-report.md").write_text("\n".join(cleaning_md) + "\n", encoding="utf-8", newline="\n")

    if export.exists():
        shutil.rmtree(export)
    documents_dir = export / "documents"
    documents_dir.mkdir(parents=True)
    manifest = []
    for row in accepted:
        content = f"# {row['title']}\n\n{row['clean_content'].strip()}\n\n---\n来源：{row['canonical_url']}\n"
        destination = documents_dir / f"{row['asku_document_id']}.md"
        destination.write_text(content, encoding="utf-8", newline="\n")
        metadata = {
            "title": row["title"],
            "school_id": "whut",
            "source_url": row["canonical_url"],
            "published_at": row.get("published_at"),
            "topic": row["topic"],
            "question_tags": row["question_tags"],
            "department": row.get("department"),
            "college": row.get("college"),
            "education_level": row["education_level"],
            "document_type": row["document_type"],
            "version_status": row["version_status"],
            "answer_scope": row["answer_scope"],
            "knowledge_bundle_id": row["knowledge_bundle_id"],
            "review_status": "ACCEPTED",
            "rag_eligible": True,
            "contains_pii": False,
            "raw_sha256": row["raw_sha256"],
        }
        manifest.append({
            "external_id": row["asku_document_id"],
            "file": "documents/" + destination.name,
            "file_sha256": sha(destination.read_bytes()),
            "clean_content_sha256": row["content_hash"],
            "metadata": metadata,
        })
    write_jsonl(export / "import-manifest.jsonl", manifest)
    write_jsonl(export / "metadata.jsonl", ({"external_id": row["external_id"], **row["metadata"]} for row in manifest))
    export_summary = {
        "status": "WEKNORA_IMPORT_READY",
        "school_id": "whut",
        "documents": len(manifest),
        "coverage_rate": coverage["coverage_rate"],
        "manifest_sha256": sha((export / "import-manifest.jsonl").read_bytes()),
        "pii_documents": 0,
        "duplicate_content": 0,
        "generated_at": datetime.now(UTC).isoformat(),
        "imported": False,
    }
    write_json(export / "export.json", export_summary)
    canary_rows = []
    for topic in [item["id"] for item in cfg["topics"]]:
        candidate = next((row for row in manifest if row["metadata"]["topic"] == topic), None)
        if candidate:
            canary_rows.append(candidate)
    write_jsonl(export / "canary-manifest.jsonl", canary_rows)
    canary_cases = []
    for topic in [item["id"] for item in cfg["topics"]]:
        candidate = next((row for row in question_rows if row["topic"] == topic and row["status"] == "COVERED"), None)
        if candidate:
            canary_cases.append({
                "question_id": candidate["question_id"],
                "question": candidate["question"],
                "topic": topic,
                "expected_source_ids": [item["asku_document_id"] for item in candidate["evidence"]],
            })
    write_json(reports / "canary-plan.json", {
        "status": "DATA_READY",
        "retrieval_run": False,
        "documents": len(canary_rows),
        "cases": canary_cases,
        "pass_condition": "top results include an expected_source_id and the answer cites the official source URL",
    })
    readme = (
        "# WeKnora 导入包\n\n"
        "`documents/` 中的 Markdown 可直接批量上传到一个武汉理工大学官方知识库。"
        "`import-manifest.jsonl` 提供幂等 ID、文件哈希和检索过滤元数据。\n\n"
        "导入后应保留 `school_id=whut`、`topic`、`question_tags`、`published_at`、"
        "`version_status` 和 `answer_scope` 元数据。\n"
    )
    (export / "README.md").write_text(readme, encoding="utf-8", newline="\n")
    sums = [f"{row['file_sha256']}  {row['file']}" for row in manifest]
    sums.append(f"{export_summary['manifest_sha256']}  import-manifest.jsonl")
    sums.append(f"{sha((export / 'canary-manifest.jsonl').read_bytes())}  canary-manifest.jsonl")
    (export / "SHA256SUMS").write_text("\n".join(sums) + "\n", encoding="ascii", newline="\n")

    evals.parent.mkdir(parents=True, exist_ok=True)
    eval_payload = {
        "version": 1,
        "school_id": "whut",
        "as_of": cfg["as_of"],
        "cases": [{
            "id": row["question_id"],
            "topic": row["topic"],
            "question": row["question"],
            "expected_status": row["status"],
            "expected_source_ids": [item["asku_document_id"] for item in row["evidence"]],
            "requires_citation": True,
        } for row in question_rows],
    }
    evals.write_text(yaml.safe_dump(eval_payload, allow_unicode=True, sort_keys=False), encoding="utf-8", newline="\n")

    validation = validate_export(export)
    write_json(reports / "import-validation.json", validation)
    write_json(batch / "validation.json", validation)
    return {"cleaning": cleaning, "coverage": coverage, "export": export_summary, "validation": validation}


def validate_export(export: Path) -> dict:
    manifest = load_jsonl(export / "import-manifest.jsonl")
    failures = []
    ids, content_hashes = set(), set()
    for row in manifest:
        path = export / row["file"]
        if row["external_id"] in ids:
            failures.append("duplicate_external_id:" + row["external_id"])
        ids.add(row["external_id"])
        if row["clean_content_sha256"] in content_hashes:
            failures.append("duplicate_clean_content:" + row["external_id"])
        content_hashes.add(row["clean_content_sha256"])
        if not path.is_file():
            failures.append("missing_file:" + row["file"])
        elif sha(path.read_bytes()) != row["file_sha256"]:
            failures.append("file_hash_mismatch:" + row["file"])
        required = {"school_id", "source_url", "topic", "question_tags", "education_level", "version_status", "answer_scope"}
        if required - set(row.get("metadata", {})):
            failures.append("missing_metadata:" + row["external_id"])
        if not row.get("metadata", {}).get("question_tags"):
            failures.append("missing_question_tags:" + row["external_id"])
    return {
        "status": "PASSED" if not failures else "FAILED",
        "documents_checked": len(manifest),
        "unique_external_ids": len(ids),
        "unique_clean_content_hashes": len(content_hashes),
        "files_hash_verified": len(manifest) - sum(reason.startswith(("missing_file:", "file_hash_mismatch:")) for reason in failures),
        "failures": failures,
    }
