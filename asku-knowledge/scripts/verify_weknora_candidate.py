"""Verify a pruned batch against the configured WeKnora data canary contract."""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import Counter
from pathlib import Path

import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from asku.quality_batch import sha, write_json
from asku.scoped_evidence import allowed_for_scope


def load_jsonl(path: Path) -> list[dict]:
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()]


def verify(batch: Path, pipeline: Path, output: Path) -> dict:
    batch, output = batch.resolve(), output.resolve()
    config = yaml.safe_load(pipeline.read_text(encoding="utf-8"))["weknora"]
    requirements = config["canary_requirements"]
    validation = json.loads((batch / "validation.json").read_text(encoding="utf-8"))
    acceptance = json.loads((output / "acceptance.json").read_text(encoding="utf-8"))
    contract = json.loads(
        (batch / "delivery-contract.json").read_text(encoding="utf-8")
    )
    docs = load_jsonl(batch / "ready_documents.jsonl")
    bundles = load_jsonl(batch / "ready_bundles.jsonl")
    failures = []

    def check(value: bool, label: str) -> None:
        if not value:
            failures.append(label)

    check(validation.get("status") == "PASSED", "prune_validation_failed")
    check(acceptance.get("status") == "PASSED", "evidence_acceptance_failed")
    check(contract.get("status") == "CANARY_READY", "delivery_contract_not_ready")
    check(validation.get("documents_deleted", 0) > 0, "noise_not_physically_removed")
    check(
        sha((batch / "deletion-manifest.jsonl").read_bytes())
        == validation.get("deletion_manifest_sha256"),
        "deletion_manifest_changed",
    )
    formats = Counter(d["parse_format"] for d in docs)
    observed = {
        "html_or_markdown": formats["html"] + formats["text"] + formats["markdown"],
        "pdf": formats["pdf"],
        "docx": formats["docx"],
        "excel": formats["xlsx"] + formats["xls"],
        "knowledge_bundles": len(bundles),
        "topic_recommendation": sum(
            d["secondary_topic"] == "recommendation" for d in docs
        ),
        "topic_transfer_major": sum(
            d["secondary_topic"] == "transfer_major" for d in docs
        ),
        "topic_graduate": sum(d["education_level"] == "GRADUATE" for d in docs),
        "version_history_series": len(
            {
                json.loads(d["review_evidence"]).get("version_family")
                for d in docs
                if json.loads(d["review_evidence"]).get("version_label")
                and json.loads(d["review_evidence"]).get("version_family")
            }
        ),
    }
    for name, minimum in requirements.items():
        check(observed.get(name, 0) >= minimum, f"canary_minimum:{name}")

    unsafe_sheet = re.compile(r"学号|身份证号|主考教师|监考教师|考生姓名|工号")
    scope_checks = 0
    for document in docs:
        review = json.loads(document["review_evidence"])
        scope = {
            "school_id": document["school_id"],
            "topic": document["secondary_topic"],
            "applicable_period": review["applicable_period"],
        }
        check(
            allowed_for_scope(document, **scope),
            "correct_scope_rejected:" + document["id"],
        )
        check(
            not allowed_for_scope(document, **scope, needs_current_policy=True),
            "current_policy_scope_bypass:" + document["id"],
        )
        scope_checks += 2
        if document["parse_format"] in {"xlsx", "xls"}:
            text = Path(document["normalized_path"]).read_text(encoding="utf-8")
            check(
                not unsafe_sheet.search(text),
                "personnel_sheet_released:" + document["id"],
            )

    report = {
        "status": "PASSED" if not failures else "FAILED",
        "data_standard": "WEKNORA_CANARY_IMPORT_READY" if not failures else "NOT_READY",
        "production_ready": False,
        "ready_documents": len(docs),
        "documents_deleted": validation.get("documents_deleted"),
        "observed": observed,
        "required": requirements,
        "scope_checks": scope_checks,
        "personnel_spreadsheets_released": sum(
            bool(
                unsafe_sheet.search(
                    Path(d["normalized_path"]).read_text(encoding="utf-8")
                )
            )
            for d in docs
            if d["parse_format"] in {"xlsx", "xls"}
        ),
        "online_freshness_verified": False,
        "production_rag_evaluated": False,
        "failures": failures,
    }
    write_json(output / "weknora-readiness.json", report)
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--batch", type=Path, required=True)
    parser.add_argument("--pipeline", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    result = verify(args.batch, args.pipeline, args.output)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if result["status"] != "PASSED":
        raise SystemExit(1)
