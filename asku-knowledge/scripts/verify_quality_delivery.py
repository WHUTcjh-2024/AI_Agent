"""Independent delivery checks, evidence acceptance, and scope-negative cases."""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import Counter
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from asku.admission import canonical_text, text_hash
from asku.quality_batch import read_db, sha, write_json
from asku.scoped_evidence import allowed_for_scope


def verify(batch: Path, cases_path: Path, output: Path) -> dict:
    output.mkdir(parents=True, exist_ok=True)
    write_json(output / "acceptance.json", {"status": "VALIDATING"})
    write_json(batch / "delivery-contract.json", {"status": "VALIDATING"})
    try:
        report = _verify(batch, cases_path, output)
    except Exception as exc:
        failed = {"status": "FAILED", "error": str(exc)}
        write_json(output / "acceptance.json", failed)
        write_json(batch / "delivery-contract.json", failed)
        raise
    if report["status"] != "PASSED":
        write_json(batch / "delivery-contract.json", {"status": "FAILED"})
    return report


def _verify(batch: Path, cases_path: Path, output: Path) -> dict:
    receipt = json.loads((batch / "validation.json").read_text(encoding="utf-8"))
    manifest = json.loads((batch / "batch.json").read_text(encoding="utf-8"))
    failures = []

    def check(condition, label):
        if not condition:
            failures.append(label)

    check(receipt["status"] == "PASSED", "batch_not_passed")
    check(manifest["status"] == "COMPLETE", "batch_incomplete")
    check(
        sha((Path(manifest["source_batch"]) / "catalog.sqlite").read_bytes())
        == manifest["input_sha256"],
        "input_modified",
    )
    exports = {}
    for filename, digest in receipt["export_sha256"].items():
        check(
            sha((batch / filename).read_bytes()) == digest, "export_changed:" + filename
        )
        exports[filename] = [
            json.loads(line)
            for line in (batch / filename).read_text(encoding="utf-8").splitlines()
        ]
    with read_db(batch / "catalog.sqlite") as conn:
        docs = {d["id"]: dict(d) for d in conn.execute("SELECT * FROM documents")}
        check(
            conn.execute("PRAGMA integrity_check").fetchone()[0] == "ok",
            "sqlite_corrupt",
        )
        check(
            not conn.execute("PRAGMA foreign_key_check").fetchall(),
            "foreign_key_violation",
        )
    ready = exports["ready_documents.jsonl"]
    ids = {d["id"] for d in ready}
    check(len(ids) == len(ready), "duplicate_ids")
    check(
        ids == {d["id"] for d in docs.values() if d["admission_status"] == "READY"},
        "ready_set_mismatch",
    )
    check(all(d == docs[d["id"]] for d in ready), "ready_row_mismatch")
    check(len({d["content_hash"] for d in ready}) == len(ready), "duplicate_content")
    texts = {}
    for d in docs.values():
        text = ""
        if d["normalized_path"]:
            path = Path(d["normalized_path"]).resolve()
            check(
                path.is_relative_to(batch.resolve() / "normalized"),
                "path_outside:" + d["id"],
            )
            raw = path.read_bytes()
            text = raw.decode("utf-8")
            check(sha(raw) == d["normalized_sha256"], "byte_hash:" + d["id"])
        check(
            (text_hash(text) if text else "") == d["content_hash"],
            "content_hash:" + d["id"],
        )
        check(
            len(canonical_text(text)) == d["content_chars"], "content_length:" + d["id"]
        )
        if d["id"] in ids:
            texts[d["id"]] = text
            check(
                d["pii_detected"] == 0 and d["pii_scan_status"] == "CLEAR",
                "pii_ready:" + d["id"],
            )
            check(d["pii_content_hash"] == d["content_hash"], "pii_stale:" + d["id"])
            check(
                d["semantic_status"] == "EVIDENCE_REVIEWED",
                "semantic_unreviewed:" + d["id"],
            )
            check(
                d["education_level"] != "UNKNOWN"
                and d["audience"] not in {"UNKNOWN", "TEACHER"},
                "unknown_scope:" + d["id"],
            )
            check(d["is_current"] is None, "unverified_current_claim:" + d["id"])
    for edge in exports["ready_relations.jsonl"]:
        check(
            edge["parent_id"] in ids and edge["child_id"] in ids,
            "blocked_relation_member",
        )
    for bundle in exports["ready_bundles.jsonl"]:
        check(set(bundle["member_ids"]) <= ids, "blocked_bundle_member")
        check(
            all(
                docs[i]["knowledge_bundle_id"] == bundle["id"]
                for i in bundle["member_ids"]
            ),
            "bundle_reference_mismatch",
        )
        check(
            docs[bundle["primary_document_id"]]["attachment_count"]
            == len(bundle["member_ids"]) - 1,
            "attachment_count_mismatch",
        )
    for att in exports["ready_attachments.jsonl"]:
        check(
            att["document_id"] in ids and att["parent_document_id"] in ids,
            "blocked_attachment_parent",
        )
        check(
            att["knowledge_bundle_id"]
            == docs[att["document_id"]]["knowledge_bundle_id"],
            "attachment_bundle_mismatch",
        )
    for context in exports.get("context_decisions.jsonl", []):
        check(context["document_id"] not in ids, "held_context_released")
        target = docs[context["document_id"]]
        check(
            target["semantic_status"] == "CONTEXT_REVIEWED_BODY_HELD",
            "context_hold_not_persisted",
        )
        check(
            context["target_content_hash"] == target["source_content_hash"],
            "context_target_changed",
        )
        if context.get("version_label"):
            check(
                target["version_family"] == context["version_family"],
                "version_family_not_persisted",
            )
            check(
                context["evidence_document_id"] in ids,
                "version_rule_source_not_released",
            )
    cases = json.loads(cases_path.read_text(encoding="utf-8"))["cases"]
    case_results = []
    for case in cases:
        d = docs[case["document_id"]]
        before = len(failures)
        check(d["id"] in ids, "case_document_not_released:" + case["id"])
        quote = case["source_quote"]
        check(
            canonical_text(quote) in canonical_text(texts.get(d["id"], "")),
            "case_quote_changed:" + case["id"],
        )

        def fold(value):
            return re.sub(r"\s+", "", value)

        check(
            fold(case["required_source_token"]) in fold(quote),
            "case_fact_not_in_source:" + case["id"],
        )
        check(
            case["source_content_hash"] == d["source_content_hash"],
            "case_source_changed:" + case["id"],
        )
        scope = {
            "school_id": d["school_id"],
            "topic": case["expected_topic"],
            "applicable_period": case["applicable_period"],
        }
        check(allowed_for_scope(d, **scope), "correct_scope_rejected:" + case["id"])
        check(
            not allowed_for_scope(d, **scope, needs_current_policy=True),
            "current_scope_bypass:" + case["id"],
        )
        check(
            not allowed_for_scope(
                d, **{**scope, "applicable_period": "UNVERIFIED-OTHER-COHORT"}
            ),
            "cohort_scope_bypass:" + case["id"],
        )
        check(
            not allowed_for_scope(d, **{**scope, "school_id": "other-school"}),
            "school_scope_bypass:" + case["id"],
        )
        case_results.append({"id": case["id"], "passed": len(failures) == before})
    # Negative cases are concrete known-bad source examples, not random exclusions.
    negatives = {
        "teacher_as_undergraduate": "whd_84884853bd1ed7398a324d318c0b8bb3",
        "medical_payment_year_conflict": "whd_397b04ccd59be41f565c49b8d06abb33",
        "thesis_exact_twenty_percent_ambiguity": "whd_bb243915c6f106afd6289c95c0ed25bc",
        "school_leader_greeting_not_service": "whd_a8738920d5e0fd07824f47e490fcc8f4",
        "teacher_nomination_not_student_recommendation": "whd_a6c297d26a3c0d36aca505c5198b33ac",
    }
    for name, ident in negatives.items():
        check(ident not in ids, "negative_released:" + name)
    report = {
        "status": "PASSED" if not failures else "FAILED",
        "input_unchanged": "input_modified" not in failures,
        "documents_checked": len(docs),
        "ready_documents": len(ready),
        "ready_attachments": len(exports["ready_attachments.jsonl"]),
        "evidence_cases": len(cases),
        "evidence_cases_passed": sum(c["passed"] for c in case_results),
        "scope_negative_checks": len(cases) * 3,
        "known_bad_examples_blocked": sum(i not in ids for i in negatives.values()),
        "topic_counts": dict(Counter(d["secondary_topic"] for d in ready)),
        "failures": failures,
        "case_results": case_results,
        "evaluation_boundary": "Evidence/source/scope acceptance only; not vector retrieval, live official freshness, or end-to-end answer accuracy.",
    }
    output.mkdir(parents=True, exist_ok=True)
    write_json(output / "acceptance.json", report)
    # Text handed to an importer contains the scope next to each reviewed evidence span.
    units = []
    for case in cases:
        d = docs[case["document_id"]]
        units.append(
            {
                "id": case["id"],
                "document_id": d["id"],
                "school_id": d["school_id"],
                "topic": d["secondary_topic"],
                "applicable_period": case["applicable_period"],
                "answer_scope": d["answer_scope"],
                "scope_note": case["scope_note"],
                "content": case["source_quote"],
                "source_url": d["source_url"],
                "content_hash": d["content_hash"],
            }
        )
    if not failures:
        (batch / "reviewed_evidence.jsonl").write_text(
            "".join(json.dumps(u, ensure_ascii=False) + "\n" for u in units),
            encoding="utf-8",
            newline="\n",
        )
        write_json(
            batch / "delivery-contract.json",
            {
                "status": "CANARY_READY",
                "source_units": len(units),
                "evidence_sha256": sha(
                    (batch / "reviewed_evidence.jsonl").read_bytes()
                ),
                "must_enforce": [
                    "school",
                    "topic",
                    "applicable_period",
                    "answer_scope",
                    "source_content_hash",
                ],
                "default_scope": "DENY",
                "current_policy_queries": "DENY_UNTIL_LIVE_VERIFIED",
                "production_rag_evaluated": False,
                "human_approval_claimed": False,
                "catalog_sha256": sha((batch / "catalog.sqlite").read_bytes()),
                "acceptance_sha256": sha((output / "acceptance.json").read_bytes()),
                "evidence_cases_sha256": sha(cases_path.read_bytes()),
            },
        )
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ["batch", "cases", "output"]:
        parser.add_argument("--" + name, type=Path, required=True)
    args = parser.parse_args()
    result = verify(args.batch, args.cases, args.output)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if result["status"] != "PASSED":
        sys.exit(1)
