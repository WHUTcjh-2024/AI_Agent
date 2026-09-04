import json
import runpy
import tempfile
import unittest
from pathlib import Path

import yaml
from asku.admission import text_hash
from asku.normalizer import normalize_html
from asku.production_batch import noise_reason
from asku.quality_batch import file_text, review_reasons, sha
from asku.scoped_evidence import allowed_for_scope
from asku.semantic_quality import normalize_artifact, suggest, title_from_evidence

ROOT = Path(__file__).resolve().parents[2]
TAXONOMY = yaml.safe_load(
    (ROOT / "asku-knowledge/config/taxonomy.yaml").read_text(encoding="utf-8")
)


class SemanticQualityTests(unittest.TestCase):
    def suggest(self, title, text=""):
        return suggest({"school_id": "whut"}, text, title, TAXONOMY)

    def test_recommendation_not_inferred_from_staff_nomination(self):
        result = self.suggest("2026年度优秀研究生指导教师推荐名单公示")
        self.assertNotEqual(result["topic"], "recommendation")
        self.assertEqual(result["content_role"], "RESULT_ANNOUNCEMENT")

    def test_teacher_research_is_not_student_innovation(self):
        result = self.suggest("关于开展2026年省级教研项目认定工作的通知")
        self.assertEqual(result["content_role"], "NON_STUDENT")
        self.assertEqual(result["audience"], "TEACHER")
        self.assertNotEqual(result["topic"], "innovation_project")

    def test_production_prune_is_fail_closed_but_preserves_reviewed_evidence(self):
        base = {
            "admission_status": "BLOCKED",
            "semantic_status": "UNREVIEWED",
            "parse_status": "PARSED",
            "pii_scan_status": "CLEAR",
            "content_chars": 500,
            "normalized_path": "d.md",
            "content_role": "PROCEDURE_GUIDE",
            "parse_format": "html",
            "secondary_topic": "course_selection",
            "audience": "UNDERGRADUATE",
            "education_level": "UNDERGRADUATE",
            "publish_date": "2026-01-01",
        }
        self.assertIsNone(noise_reason(base))
        self.assertEqual(
            noise_reason({**base, "pii_scan_status": "HIT"}),
            "INVALID_PARSE_PII_OR_TEXT",
        )
        self.assertEqual(
            noise_reason({**base, "content_role": "NEWS"}), "NEWS_RESULT_OR_NON_STUDENT"
        )
        self.assertEqual(
            noise_reason({**base, "audience": "UNKNOWN"}), "UNRESOLVED_SEMANTIC_SCOPE"
        )
        self.assertEqual(
            noise_reason(
                {**base, "pii_scan_status": "HIT", "admission_status": "READY"}
            ),
            "INVALID_PARSE_PII_OR_TEXT",
        )

    def test_recommendation_beats_incidental_degree_word(self):
        result = self.suggest(
            "武汉理工大学推免研究生用科学研究论文分类认定目录", "（2022版）"
        )
        self.assertEqual(result["topic"], "recommendation")
        self.assertEqual(result["education_level"], "UNDERGRADUATE")
        self.assertEqual(result["version_labels"], ["2022"])

    def test_versions_extracted_without_current_claim(self):
        result = self.suggest("论文分类认定目录", "2026版目录，附件仍有2022版。")
        self.assertEqual(result["version_labels"], ["2022", "2026"])
        self.assertNotIn("is_current", result)

    def test_blank_forms_not_misrepresented_as_policy(self):
        self.assertEqual(
            self.suggest("学生申请推荐免试研究生承诺书")["content_role"],
            "FORM_TEMPLATE",
        )

    def test_generic_html_title_replaced_with_article_heading(self):
        raw = "<title>欢迎访问武汉理工大学研究生教育信息网</title><h2>关于研究生公共课考试安排的通知</h2>".encode()
        title, _ = title_from_evidence(
            {"title": "欢迎访问", "is_attachment": 0}, "", raw
        )
        self.assertEqual(title, "关于研究生公共课考试安排的通知")

    def test_whitespace_cleanup_preserves_table_and_policy_numbers(self):
        body = "# old\n\n\n2026版\n\n| 类型 | 标准 |\n| --- | --- |\n| A | 10%-20% |\n"
        result = normalize_artifact(body, "new", False)
        self.assertIn("| A | 10%-20% |", result)
        self.assertIn("2026版", result)
        self.assertNotIn("\n\n\n", result)

    def test_event_date_is_not_publish_date(self):
        page = normalize_html(
            "<main>会议时间：2026年10月22日。" + ("办理说明。" * 50) + "</main>"
        )
        self.assertIsNone(page.publish_date)

    def test_navigation_does_not_inflate_body_characters(self):
        page = normalize_html(
            '<div class="nav">'
            + "菜单" * 200
            + '</div><div class="TRS_Editor">短正文</div>'
        )
        self.assertEqual(page.text, "短正文")
        self.assertEqual(page.content_chars, 3)


class EvidenceGateTests(unittest.TestCase):
    def setUp(self):
        self.text = "申请人须为本校全日制本科生，按通知要求提交申请材料。"
        self.d = {
            "source_content_hash": "expected",
            "source_url": "https://jwc.whut.edu.cn/a",
            "content_role": "ANNUAL_NOTICE",
            "pii_detected": 0,
            "pii_scan_status": "CLEAR",
            "parse_status": "PARSED",
            "publish_date": "2026-09-01",
        }
        self.r = {
            "source_content_hash": "expected",
            "source_url": self.d["source_url"],
            "reviewer_type": "AI_ASSISTED_EVIDENCE_REVIEW",
            "answer_scope": "DATED_SOURCE_ONLY",
            "scope_note": "仅用于该通知所载学年，不推断现行政策。",
            "evidence": [self.text],
            "secondary_topic": "recommendation",
            "education_level": "UNDERGRADUATE",
            "audience": "UNDERGRADUATE",
            "document_type": "ANNUAL_NOTICE",
        }

    def reasons(self, **changes):
        return review_reasons(self.d, {**self.r, **changes}, self.text, TAXONOMY)

    def test_complete_evidence_can_pass(self):
        self.assertEqual(self.reasons(), [])

    def test_changed_source_cannot_reuse_old_review(self):
        self.assertIn(
            "review_source_hash_mismatch", self.reasons(source_content_hash="changed")
        )

    def test_invented_quote_cannot_pass(self):
        self.assertIn(
            "review_evidence_missing_or_changed",
            self.reasons(evidence=["该校全体学生无需申请直接获得名额。"]),
        )

    def test_unsupported_current_claim_cannot_pass(self):
        self.assertIn(
            "explicit_answer_scope_required",
            self.reasons(answer_scope="CURRENT_POLICY"),
        )

    def test_unknown_audience_cannot_pass(self):
        self.assertIn(
            "review_student_audience_required", self.reasons(audience="UNKNOWN")
        )

    def test_old_pii_hold_cannot_be_cleared_by_review_flag(self):
        self.d["pii_detected"] = 1
        self.assertIn("pii_not_cleared", self.reasons())

    def test_evidenced_historical_resolution_requires_clear_rescan(self):
        self.d["pii_detected"] = 1
        resolution = {
            "decision": "HISTORICAL_FALSE_POSITIVE",
            "reviewed_entire_document": True,
            "rationale": "全文为公开政策和虚构例子；没有真实学生名册，保留原始风险标记和单条解除记录。",
            "evidence": self.text,
        }
        self.assertNotIn("pii_not_cleared", self.reasons(pii_resolution=resolution))
        self.d["pii_scan_status"] = "HIT"
        self.assertIn("pii_not_cleared", self.reasons(pii_resolution=resolution))

    def test_scoped_evidence_refuses_current_or_wrong_year(self):
        d = {
            **self.d,
            "school_id": "whut",
            "secondary_topic": "recommendation",
            "content_hash": "a" * 64,
            "pii_content_hash": "a" * 64,
            "source_content_hash": "b" * 64,
            "rag_eligible": 1,
            "admission_status": "READY",
            "semantic_status": "EVIDENCE_REVIEWED",
            "review_status": "ACCEPTED",
            "answer_scope": "DATED_SOURCE_ONLY",
            "review_evidence": json.dumps(
                {"applicable_period": "2027届", "source_content_hash": "b" * 64}
            ),
        }
        scope = {
            "school_id": "whut",
            "topic": "recommendation",
            "applicable_period": "2027届",
        }
        self.assertTrue(allowed_for_scope(d, **scope))
        self.assertFalse(allowed_for_scope(d, **scope, needs_current_policy=True))
        self.assertFalse(
            allowed_for_scope(d, **{**scope, "applicable_period": "2028届"})
        )
        self.assertFalse(allowed_for_scope(d, **{**scope, "applicable_period": ""}))
        for key in ("content_hash", "pii_content_hash", "source_content_hash"):
            with self.subTest(missing_hash=key):
                self.assertFalse(allowed_for_scope({**d, key: None}, **scope))
        self.assertFalse(allowed_for_scope({**d, "rag_eligible": 0}, **scope))
        self.assertFalse(allowed_for_scope({**d, "review_evidence": "[]"}, **scope))

    def test_actual_artifact_tampering_is_detected(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            (root / "normalized").mkdir()
            path = root / "normalized" / "d.md"
            data = self.text.encode()
            path.write_bytes(data)
            d = {
                "id": "d",
                "normalized_path": str(path),
                "normalized_sha256": sha(data),
                "content_hash": text_hash(self.text),
                "content_chars": len(self.text),
            }
            self.assertEqual(file_text(d, root), self.text)
            path.write_text("changed", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "normalized_bytes_changed"):
                file_text(d, root)

    def test_failed_acceptance_invalidates_previous_delivery(self):
        verify = runpy.run_path(
            str(ROOT / "asku-knowledge/scripts/verify_quality_delivery.py")
        )["verify"]
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            contract = root / "delivery-contract.json"
            contract.write_text('{"status":"CANARY_READY"}', encoding="utf-8")
            with self.assertRaises(FileNotFoundError):
                verify(root, root / "missing-cases.json", root / "report")
            self.assertEqual(json.loads(contract.read_text())["status"], "FAILED")
            self.assertEqual(
                json.loads((root / "report/acceptance.json").read_text())["status"],
                "FAILED",
            )


if __name__ == "__main__":
    unittest.main()
