import json
import re

from asku.pii import detect_pii
from asku.question_admit import QUESTION_PATTERNS, evidence_tags, sha, validate_export


def test_question_patterns_cover_expected_phrases():
    assert re.search(QUESTION_PATTERNS["transfer_eligibility"], "一、申请条件")
    assert re.search(QUESTION_PATTERNS["cet_portal"], "报名网址 cet-bm.neea.edu.cn")
    assert re.search(QUESTION_PATTERNS["library_renew"], "每本图书允许续借一次")


def test_evidence_tags_do_not_cross_document_topics():
    questions = {
        "library_hours": {"topic": "library"},
        "card_fault": {"topic": "campus_card"},
    }
    row = {
        "title": "图书馆开放时间调整通知",
        "clean_content": "设备故障期间开放时间调整。",
        "topic": "library",
    }
    assert evidence_tags(row, questions) == ["library_hours"]


def test_policy_score_table_is_not_personal_score_data():
    policy = "本规定适用于本科生。名单由学院审核。另附项目名单。\n| 成绩 | 对应绩点 |\n| --- | --- |\n| 90 | 4.0 |"
    assert detect_pii(policy, title="学籍管理规定", tables_markdown=policy).categories == []


def test_blank_policy_form_is_not_a_student_roster():
    policy = "本办法附考试记录表。名单由学院归档。另附名单格式。\n| 姓名 | 学号 | 成绩 |\n| --- | --- | --- |\n| | | |"
    assert detect_pii(policy, title="本科课程考核管理办法", tables_markdown=policy).categories == []


def test_validate_export_checks_hashes_and_metadata(tmp_path):
    documents = tmp_path / "documents"
    documents.mkdir()
    body = b"# title\n\ncontent\n"
    (documents / "doc.md").write_bytes(body)
    row = {
        "external_id": "doc",
        "file": "documents/doc.md",
        "file_sha256": sha(body),
        "clean_content_sha256": sha("content"),
        "metadata": {
            "school_id": "whut",
            "source_url": "https://www.whut.edu.cn/example",
            "topic": "library",
            "question_tags": ["library_hours"],
            "education_level": "BOTH",
            "version_status": "latest_observed",
            "answer_scope": "DATED_SOURCE_ONLY",
        },
    }
    (tmp_path / "import-manifest.jsonl").write_text(json.dumps(row) + "\n", encoding="utf-8")
    assert validate_export(tmp_path)["status"] == "PASSED"
