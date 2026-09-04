"""Export immutable Markdown inputs and mapping metadata for a WeKnora canary."""

from __future__ import annotations

import argparse
import json
import shutil
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from asku.quality_batch import sha, write_json


def load_jsonl(path: Path) -> list[dict]:
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()]


def export(batch: Path, report: Path, output: Path) -> dict:
    batch, report, output = batch.resolve(), report.resolve(), output.resolve()
    if output.exists():
        raise ValueError("output_must_not_exist")
    readiness = json.loads(
        (report / "weknora-readiness.json").read_text(encoding="utf-8")
    )
    if readiness.get("status") != "PASSED":
        raise ValueError("weknora_data_not_ready")
    output.mkdir(parents=True)
    documents_dir = output / "documents"
    documents_dir.mkdir()
    attachments = {
        row["document_id"]: row for row in load_jsonl(batch / "ready_attachments.jsonl")
    }
    rows = []
    for document in load_jsonl(batch / "ready_documents.jsonl"):
        source = Path(document["normalized_path"])
        destination = documents_dir / (document["id"] + ".md")
        shutil.copyfile(source, destination)
        if sha(destination.read_bytes()) != document["normalized_sha256"]:
            raise ValueError("exported_document_changed:" + document["id"])
        review = json.loads(document["review_evidence"])
        attachment = attachments.get(document["id"])
        rows.append(
            {
                "external_id": document["id"],
                "file": "documents/" + destination.name,
                "file_sha256": document["normalized_sha256"],
                "content_hash": document["content_hash"],
                "asku_document_id": document["id"],
                "attachment_id": attachment["id"] if attachment else None,
                "metadata": {
                    "school_id": document["school_id"],
                    "source_url": document["source_url"],
                    "published_at": document["publish_date"],
                    "topic": document["secondary_topic"],
                    "education_level": document["education_level"],
                    "applicable_period": review["applicable_period"],
                    "answer_scope": document["answer_scope"],
                    "knowledge_bundle_id": document["knowledge_bundle_id"],
                },
            }
        )
    mapping = output / "import-manifest.jsonl"
    mapping.write_text(
        "".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows),
        encoding="utf-8",
        newline="\n",
    )
    manifest = {
        "status": "WEKNORA_CANARY_IMPORT_READY",
        "documents": len(rows),
        "import_manifest_sha256": sha(mapping.read_bytes()),
        "production_enable_allowed": False,
        "required_after_upload": [
            "record returned WeKnora knowledge IDs in knowledge.weknora_mappings",
            "run retrieval and citation canary",
            "enforce school, topic and applicable-period scope before enabling provider",
        ],
    }
    write_json(output / "export.json", manifest)
    return manifest


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("batch", "report", "output"):
        parser.add_argument("--" + name, type=Path, required=True)
    args = parser.parse_args()
    print(
        json.dumps(
            export(args.batch, args.report, args.output), ensure_ascii=False, indent=2
        )
    )
