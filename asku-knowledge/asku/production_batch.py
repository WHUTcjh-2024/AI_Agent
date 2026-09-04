"""Create a compact production candidate by physically removing high-risk noise."""

from __future__ import annotations

import json
import shutil
import sqlite3
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path

from .quality_batch import file_text, read_db, sha, write_json

NOISE_ROLES = {"NEWS", "RESULT_ANNOUNCEMENT", "NON_STUDENT"}
NOISE_FORMATS = {"archive", "rar", "zip", "unknown", "ole"}
AUDIT_HOLD_STATUS = "CONTEXT_REVIEWED_BODY_HELD"


def noise_reason(document: dict) -> str | None:
    """Return the first deterministic deletion reason; reviewed evidence is retained."""
    chars = document.get("content_chars")
    if (
        document.get("parse_status") != "PARSED"
        or document.get("pii_scan_status") != "CLEAR"
        or not isinstance(chars, int)
        or not 100 <= chars <= 100_000
        or not document.get("normalized_path")
    ):
        return "INVALID_PARSE_PII_OR_TEXT"
    if (
        document.get("admission_status") == "READY"
        or document.get("semantic_status") == AUDIT_HOLD_STATUS
    ):
        return None
    if document.get("content_role") in NOISE_ROLES:
        return "NEWS_RESULT_OR_NON_STUDENT"
    if (
        document.get("content_role") == "FORM_TEMPLATE"
        or document.get("parse_format") in NOISE_FORMATS
    ):
        return "FORM_OR_UNSUPPORTED_ARCHIVE"
    if (
        document.get("secondary_topic") == "other"
        or document.get("content_role") == "UNKNOWN"
        or document.get("audience") == "UNKNOWN"
        or document.get("education_level") == "UNKNOWN"
    ):
        return "UNRESOLVED_SEMANTIC_SCOPE"
    if document.get("content_role") == "ANNUAL_NOTICE" and (
        not document.get("publish_date") or document["publish_date"] < "2023-01-01"
    ):
        return "STALE_ANNUAL_NOTICE"
    return None


def _write_jsonl(path: Path, rows) -> None:
    path.write_text(
        "".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows),
        encoding="utf-8",
        newline="\n",
    )


def prune(source: Path, output: Path) -> dict:
    source, output = source.resolve(), output.resolve()
    if output.exists() or output == source or output.is_relative_to(source):
        raise ValueError("output_must_be_new_sibling_batch")
    validation = json.loads((source / "validation.json").read_text(encoding="utf-8"))
    contract = json.loads(
        (source / "delivery-contract.json").read_text(encoding="utf-8")
    )
    if validation.get("status") != "PASSED" or contract.get("status") != "CANARY_READY":
        raise ValueError("source_delivery_not_accepted")

    output.mkdir(parents=True)
    normalized = output / "normalized"
    normalized.mkdir()
    input_sha = sha((source / "catalog.sqlite").read_bytes())
    manifest = {
        "status": "PRUNING",
        "school_id": "whut",
        "source_batch": str(source),
        "input_sha256": input_sha,
        "rule_version": "production-prune-v1",
        "created_at": datetime.now(timezone.utc).isoformat(),
        "online_freshness_verified": False,
    }
    write_json(output / "batch.json", manifest)
    src = read_db(source / "catalog.sqlite")
    conn = sqlite3.connect(output / "catalog.sqlite")
    src.backup(conn)
    src.close()
    conn.row_factory = sqlite3.Row
    try:
        docs = {
            row["id"]: dict(row)
            for row in conn.execute("SELECT * FROM documents ORDER BY id")
        }
        removed = {}
        for ident, document in docs.items():
            reason = noise_reason(document)
            if reason:
                removed[ident] = reason

        # Keep one source record per identical body, preferring reviewed and newer records.
        by_hash = defaultdict(list)
        for document in docs.values():
            if document["id"] not in removed:
                by_hash[document["content_hash"]].append(document)
        for group in by_hash.values():
            if len(group) < 2:
                continue
            group.sort(
                key=lambda d: (
                    d["admission_status"] == "READY",
                    d["semantic_status"] == AUDIT_HOLD_STATUS,
                    bool(d["publish_date"]),
                    d["publish_date"] or "",
                    not bool(d["is_attachment"]),
                    d["id"],
                ),
                reverse=True,
            )
            for duplicate in group[1:]:
                removed[duplicate["id"]] = "DUPLICATE_CONTENT"

        # Attachments without a retained parent cannot be trusted as a bundle.
        parents = defaultdict(set)
        for row in conn.execute("SELECT child_id,parent_id FROM clean_relations"):
            parents[row["child_id"]].add(row["parent_id"])
        changed = True
        while changed:
            changed = False
            for document in docs.values():
                ident = document["id"]
                if ident in removed or not document["is_attachment"]:
                    continue
                if not (parents[ident] - removed.keys()):
                    removed[ident] = "ORPHAN_ATTACHMENT"
                    changed = True

        kept = [document for ident, document in docs.items() if ident not in removed]
        for document in kept:
            text = file_text(document, source)
            if text:
                destination = normalized / (document["id"] + ".md")
                shutil.copyfile(document["normalized_path"], destination)
                if sha(destination.read_bytes()) != document["normalized_sha256"]:
                    raise ValueError("copied_artifact_changed:" + document["id"])
                document["normalized_path"] = str(destination)
                document["local_file_path"] = str(destination)

        conn.execute("PRAGMA foreign_keys=OFF")
        conn.execute("CREATE TEMP TABLE removed_ids(id TEXT PRIMARY KEY)")
        conn.executemany(
            "INSERT INTO removed_ids VALUES(?)", ((ident,) for ident in removed)
        )
        statements = [
            "DELETE FROM pii_reviews WHERE document_id IN removed_ids OR attachment_id IN (SELECT id FROM attachments WHERE document_id IN removed_ids)",
            "DELETE FROM quality_pii_resolutions WHERE document_id IN removed_ids",
            "DELETE FROM semantic_reviews WHERE document_id IN removed_ids",
            "DELETE FROM clean_pii_scans WHERE document_id IN removed_ids",
            "DELETE FROM clean_relations WHERE child_id IN removed_ids OR parent_id IN removed_ids",
            "DELETE FROM weknora_mappings WHERE asku_document_id IN removed_ids",
            "DELETE FROM attachments WHERE document_id IN removed_ids OR parent_document_id IN removed_ids",
            "DELETE FROM documents WHERE id IN removed_ids",
            "DELETE FROM knowledge_bundles WHERE primary_document_id IN removed_ids",
            "DELETE FROM cck_knowledge_bundles WHERE primary_document_id IN removed_ids",
        ]
        for statement in statements:
            conn.execute(statement)
        for document in kept:
            conn.execute(
                "UPDATE documents SET normalized_path=?,local_file_path=? WHERE id=?",
                (
                    document["normalized_path"],
                    document["local_file_path"],
                    document["id"],
                ),
            )
            if document["is_attachment"]:
                conn.execute(
                    "UPDATE attachments SET local_file_path=?,file_path=? WHERE document_id=?",
                    (
                        document["normalized_path"],
                        document["normalized_path"],
                        document["id"],
                    ),
                )
        conn.commit()
        if conn.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
            raise ValueError("pruned_catalog_corrupt")

        _write_jsonl(
            output / "deletion-manifest.jsonl",
            (
                {
                    "id": ident,
                    "reason": removed[ident],
                    "source_content_hash": docs[ident]["source_content_hash"],
                    "source_url": docs[ident]["source_url"],
                }
                for ident in sorted(removed)
            ),
        )
        ready = [
            document for document in kept if document["admission_status"] == "READY"
        ]
        ready_ids = {document["id"] for document in ready}
        ready_attachments = [
            dict(row)
            for row in conn.execute(
                "SELECT * FROM attachments WHERE admission_status='READY' ORDER BY document_id"
            )
        ]
        relations = [
            dict(row)
            for row in conn.execute(
                "SELECT * FROM clean_relations ORDER BY parent_id,child_id"
            )
            if row["parent_id"] in ready_ids and row["child_id"] in ready_ids
        ]
        by_parent = defaultdict(list)
        for edge in relations:
            by_parent[edge["parent_id"]].append(edge["child_id"])
        bundles = [
            {
                "id": "reviewed_" + parent,
                "primary_document_id": parent,
                "member_ids": [parent, *children],
            }
            for parent, children in sorted(by_parent.items())
        ]
        contexts = [
            json.loads(line)
            for line in (source / "context_decisions.jsonl")
            .read_text(encoding="utf-8")
            .splitlines()
            if json.loads(line)["document_id"] not in removed
        ]
        queue = [
            {
                "id": d["id"],
                "title": d["title"],
                "content_role": d["content_role"],
                "suggested_topic": d["secondary_topic"],
                "reasons": json.loads(d["semantic_reasons"])
                + ["evidence_review_required"],
            }
            for d in kept
            if d["id"] not in ready_ids and d["semantic_status"] != AUDIT_HOLD_STATUS
        ]
        exports = {
            "context_decisions.jsonl": contexts,
            "ready_documents.jsonl": ready,
            "ready_attachments.jsonl": ready_attachments,
            "ready_relations.jsonl": relations,
            "ready_bundles.jsonl": bundles,
            "review_queue.jsonl": queue,
        }
        for name, rows in exports.items():
            _write_jsonl(output / name, rows)
        shutil.copyfile(source / "review-ledger.json", output / "review-ledger.json")
        shutil.copyfile(
            source / "reviewed_evidence.jsonl", output / "reviewed_evidence.jsonl"
        )
        stats = Counter(removed.values())
        receipt = {
            "status": "PASSED",
            "delivery_scope": "PRUNED_PRODUCTION_CANDIDATE",
            "source_documents": len(docs),
            "documents_checked": len(kept),
            "documents_deleted": len(removed),
            "deletion_reasons": dict(sorted(stats.items())),
            "ready_documents": len(ready),
            "ready_attachments": len(ready_attachments),
            "review_queue": len(queue),
            "blocked_parent_exports": 0,
            "duplicate_ready_content": 0,
            "current_policy_claims": 0,
            "export_sha256": {
                name: sha((output / name).read_bytes()) for name in exports
            },
            "review_ledger_sha256": sha((output / "review-ledger.json").read_bytes()),
            "deletion_manifest_sha256": sha(
                (output / "deletion-manifest.jsonl").read_bytes()
            ),
            "online_freshness_verified": False,
            "production_rag_evaluated": False,
        }
        write_json(output / "validation.json", receipt)
        manifest.update(
            {
                "status": "COMPLETE",
                "documents": len(kept),
                "documents_deleted": len(removed),
                "release": receipt,
            }
        )
        write_json(output / "batch.json", manifest)
        write_json(
            output / "delivery-contract.json", {"status": "REQUIRES_REVALIDATION"}
        )
        return receipt
    except Exception:
        manifest["status"] = "FAILED"
        write_json(output / "batch.json", manifest)
        raise
    finally:
        conn.close()
