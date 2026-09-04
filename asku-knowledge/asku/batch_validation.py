"""Verify file-level admission receipts and finalize staging-only mirror exports."""

from __future__ import annotations

import hashlib
import json
import sqlite3
from collections import Counter
from pathlib import Path

import yaml

from .admission import canonical_text, evaluate, text_hash


def finalize_and_verify(batch: Path, school_path: Path, taxonomy_path: Path) -> dict:
    batch = batch.resolve()
    receipt = json.loads((batch / "batch.json").read_text(encoding="utf-8"))
    if receipt.get("status") != "COMPLETE":
        raise ValueError("batch_not_complete")
    school = yaml.safe_load(school_path.read_text(encoding="utf-8"))
    taxonomy = yaml.safe_load(taxonomy_path.read_text(encoding="utf-8"))
    conn = sqlite3.connect(batch / "catalog.sqlite")
    conn.row_factory = sqlite3.Row
    try:
        rows = [dict(row) for row in conn.execute("SELECT * FROM documents")]
        sources = {r["id"]: dict(r) for r in conn.execute("SELECT * FROM sources")}
        failures = Counter()
        ready = []
        for d in rows:
            if d["school_id"] != school["school_id"]:
                continue
            text = ""
            if d["normalized_path"]:
                path = Path(d["normalized_path"]).resolve()
                if not path.is_relative_to(batch / "normalized") or not path.is_file():
                    failures["text_path_invalid"] += 1
                    continue
                text = path.read_text(encoding="utf-8", errors="strict")
                if hashlib.sha256(text.encode()).hexdigest() != d["normalized_sha256"]:
                    failures["text_changed_after_cleaning"] += 1
            if len(canonical_text(text)) != d["content_chars"]:
                failures["content_char_mismatch"] += 1
            if (text_hash(text) if text else "") != d["content_hash"]:
                failures["content_hash_mismatch"] += 1
            decision = evaluate(
                d,
                taxonomy,
                school,
                source_active=bool(sources.get(d["source_id"], {}).get("active")),
            )
            if decision.eligible != bool(d["rag_eligible"]):
                failures["admission_mismatch"] += 1
            if decision.eligible:
                ready.append(d)
        failures["orphan_relations"] = conn.execute(
            "SELECT count(*) FROM clean_relations r LEFT JOIN documents c ON c.id=r.child_id LEFT JOIN documents p ON p.id=r.parent_id WHERE c.id IS NULL OR p.id IS NULL"
        ).fetchone()[0]
        failures["orphan_bundle_parents"] = conn.execute(
            "SELECT count(*) FROM knowledge_bundles b LEFT JOIN documents p ON p.id=b.primary_document_id WHERE p.id IS NULL"
        ).fetchone()[0]
        failures["broken_resolved_parents"] = conn.execute(
            "SELECT count(*) FROM documents d WHERE d.is_attachment=1 AND d.relation_status='RESOLVED' AND NOT EXISTS (SELECT 1 FROM clean_relations r JOIN documents p ON p.id=r.parent_id WHERE r.child_id=d.id AND p.source_url=d.parent_page_url)"
        ).fetchone()[0]
        if sum(failures.values()):
            raise ValueError("batch_validation_failed:" + json.dumps(failures))
        # Mirror receipt columns are part of the backend attachment admission contract.
        fields = {
            "parse_status": "PENDING",
            "pii_scan_status": "PENDING",
            "pii_content_hash": "",
            "content_hash": "",
            "admission_status": "BLOCKED",
            "relation_status": "UNRESOLVED",
        }
        existing = {r[1] for r in conn.execute("PRAGMA table_info(attachments)")}
        with conn:
            for name, default in fields.items():
                if name not in existing:
                    conn.execute(
                        f"ALTER TABLE attachments ADD COLUMN {name} TEXT NOT NULL DEFAULT '{default}'"
                    )
            for d in rows:
                if d["is_attachment"] and d["school_id"] == school["school_id"]:
                    conn.execute(
                        "UPDATE attachments SET "
                        + ",".join(k + "=?" for k in fields)
                        + " WHERE document_id=?",
                        [d[k] for k in fields] + [d["id"]],
                    )
        exports = {
            "ready_documents.jsonl": ready,
            "ready_attachments.jsonl": [
                dict(r)
                for r in conn.execute(
                    "SELECT a.* FROM attachments a JOIN documents d ON d.id=a.document_id WHERE d.school_id=? AND d.rag_eligible=1 AND a.rag_eligible=1 AND a.admission_status='READY'",
                    (school["school_id"],),
                )
            ],
            "bundles.jsonl": [
                dict(r)
                for r in conn.execute(
                    "SELECT * FROM knowledge_bundles WHERE school_id=?",
                    (school["school_id"],),
                )
            ],
            "relations.jsonl": [
                dict(r)
                for r in conn.execute(
                    "SELECT r.* FROM clean_relations r JOIN documents d ON d.id=r.child_id WHERE d.school_id=?",
                    (school["school_id"],),
                )
            ],
        }
        for name, values in exports.items():
            with (batch / name).open("w", encoding="utf-8") as stream:
                for row in values:
                    stream.write(json.dumps(row, ensure_ascii=False) + "\n")
        report = {
            "status": "PASSED",
            "documents_checked": len(rows),
            "ready_documents": len(ready),
            "ready_attachments": len(exports["ready_attachments.jsonl"]),
            "violations": dict(failures),
            "export_sha256": {
                name: hashlib.sha256((batch / name).read_bytes()).hexdigest()
                for name in exports
            },
        }
        (batch / "validation.json").write_text(
            json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8"
        )
        return report
    finally:
        conn.close()
