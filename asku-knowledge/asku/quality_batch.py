"""Immutable-input enrichment and evidence-reviewed, scope-limited delivery."""

from __future__ import annotations

import hashlib
import json
import re
import sqlite3
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path

import yaml

from .admission import canonical_text, evaluate, text_hash
from .pii import detect_pii
from .semantic_quality import (
    RULE_VERSION,
    normalize_artifact,
    suggest,
    title_from_evidence,
)


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def write_json(path: Path, value) -> None:
    temp = path.with_suffix(path.suffix + ".tmp")
    temp.write_text(
        json.dumps(value, ensure_ascii=False, indent=2), encoding="utf-8", newline="\n"
    )
    temp.replace(path)


def read_db(path: Path) -> sqlite3.Connection:
    conn = sqlite3.connect(path.resolve().as_uri() + "?mode=ro", uri=True)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA query_only=ON")
    return conn


def file_text(d: dict, root: Path) -> str:
    if not d.get("normalized_path"):
        return ""
    path = Path(d["normalized_path"]).resolve()
    if not path.is_relative_to(root.resolve() / "normalized") or not path.is_file():
        raise ValueError("normalized_path_invalid:" + d["id"])
    data = path.read_bytes()
    if sha(data) != d["normalized_sha256"]:
        raise ValueError("normalized_bytes_changed:" + d["id"])
    text = data.decode("utf-8", errors="strict")
    if (
        text_hash(text) != d["content_hash"]
        or len(canonical_text(text)) != d["content_chars"]
    ):
        raise ValueError("normalized_content_changed:" + d["id"])
    return text


def build(
    source: Path, output: Path, school_path: Path, taxonomy_path: Path, raw_root: Path
) -> dict:
    source, output, raw_root = source.resolve(), output.resolve(), raw_root.resolve()
    if output.exists() or output == source or output.is_relative_to(source):
        raise ValueError("output_must_be_new_sibling_batch")
    receipt = json.loads((source / "validation.json").read_text(encoding="utf-8"))
    if receipt.get("status") != "PASSED":
        raise ValueError("input_batch_not_validated")
    taxonomy = yaml.safe_load(taxonomy_path.read_text(encoding="utf-8"))
    school = yaml.safe_load(school_path.read_text(encoding="utf-8"))
    output.mkdir(parents=True)
    (output / "normalized").mkdir()
    fingerprint = sha((source / "catalog.sqlite").read_bytes())
    manifest = {
        "status": "BUILDING",
        "school_id": school["school_id"],
        "source_batch": str(source),
        "input_sha256": fingerprint,
        "rule_version": RULE_VERSION,
        "created_at": datetime.now(timezone.utc).isoformat(),
        "online_freshness_verified": False,
    }
    write_json(output / "batch.json", manifest)
    src = read_db(source / "catalog.sqlite")
    conn = sqlite3.connect(output / "catalog.sqlite")
    src.backup(conn)
    src.close()
    conn.row_factory = sqlite3.Row
    columns = {
        "source_content_hash": "TEXT NOT NULL DEFAULT ''",
        "source_normalized_sha256": "TEXT NOT NULL DEFAULT ''",
        "semantic_status": "TEXT NOT NULL DEFAULT 'UNREVIEWED'",
        "semantic_reasons": "TEXT NOT NULL DEFAULT '[]'",
        "content_role": "TEXT NOT NULL DEFAULT 'UNKNOWN'",
        "title_evidence": "TEXT NOT NULL DEFAULT ''",
        "version_labels": "TEXT NOT NULL DEFAULT '[]'",
        "answer_scope": "TEXT NOT NULL DEFAULT ''",
        "review_evidence": "TEXT NOT NULL DEFAULT '[]'",
        "reviewer_type": "TEXT NOT NULL DEFAULT ''",
    }
    try:
        existing = {r[1] for r in conn.execute("PRAGMA table_info(documents)")}
        for field, sql in columns.items():
            if field not in existing:
                conn.execute(f"ALTER TABLE documents ADD COLUMN {field} {sql}")
        conn.execute(
            "CREATE TABLE semantic_reviews (document_id TEXT PRIMARY KEY, suggestion_json TEXT NOT NULL, review_json TEXT)"
        )
        docs = [dict(d) for d in conn.execute("SELECT * FROM documents ORDER BY id")]
        stats = Counter()
        pii_version = sha(Path(__file__).with_name("pii.py").read_bytes())
        for index, d in enumerate(docs, 1):
            if d["school_id"] != school["school_id"]:
                raise ValueError("mixed_school_batch")
            text = file_text(d, source)
            raw = None
            if not d["is_attachment"]:
                path = Path(re.sub(r"\\+", "/", d["raw_path"] or "")).resolve()
                if path.is_relative_to(raw_root) and path.is_file():
                    raw = path.read_bytes()
                    if sha(raw) != d["raw_sha256"]:
                        raise ValueError("raw_bytes_changed:" + d["id"])
            title, evidence = title_from_evidence(d, text, raw)
            enriched = suggest(d, text, title, taxonomy)
            cleaned = (
                normalize_artifact(text, title, bool(d["is_attachment"]))
                if text
                else ""
            )
            path = output / "normalized" / (d["id"] + ".md")
            if cleaned:
                path.write_bytes(cleaned.encode("utf-8"))
            finding = (
                detect_pii(cleaned, title=title, tables_markdown=cleaned)
                if cleaned and len(cleaned) <= 100_000
                else None
            )
            scan = (
                "HIT"
                if finding and finding.is_blocking
                else "CLEAR"
                if finding
                else "NOT_SCANNED"
            )
            changes = {
                "source_content_hash": d["content_hash"],
                "source_normalized_sha256": d["normalized_sha256"],
                "title": title,
                "title_evidence": evidence,
                "normalized_path": str(path) if cleaned else "",
                "local_file_path": str(path) if cleaned else "",
                "normalized_sha256": sha(cleaned.encode()) if cleaned else "",
                "content_hash": text_hash(cleaned) if cleaned else "",
                "content_chars": len(canonical_text(cleaned)),
                "text_length": len(canonical_text(cleaned)),
                "secondary_topic": enriched["topic"],
                "primary_module": enriched["primary_module"],
                "audience": enriched["audience"],
                "education_level": enriched["education_level"],
                "content_role": enriched["content_role"],
                "semantic_reasons": json.dumps(enriched["risk_reasons"]),
                "version_labels": json.dumps(enriched["version_labels"]),
                "version_family": enriched["version_family"],
                "is_current": None,
                "current_confidence": None,
                "semantic_status": "UNREVIEWED",
                "review_status": "PII_REVIEW"
                if d["pii_detected"] or scan == "HIT"
                else "REVIEW",
                "pii_detected": int(bool(d["pii_detected"]) or scan == "HIT"),
                "pii_scan_status": scan,
                "pii_content_hash": text_hash(cleaned) if scan != "NOT_SCANNED" else "",
                "pii_rule_version": pii_version,
                "pii_categories": json.dumps(finding.categories if finding else []),
                "rag_eligible": 0,
                "admission_status": "BLOCKED",
                "admission_version": RULE_VERSION,
                "admission_reasons": json.dumps(["evidence_review_required"]),
                "updated_at": manifest["created_at"],
            }
            if enriched["content_role"] in taxonomy["document_types"]:
                changes["document_type"] = enriched["content_role"]
            conn.execute(
                "UPDATE documents SET "
                + ",".join(f"{k}=?" for k in changes)
                + " WHERE id=?",
                [*changes.values(), d["id"]],
            )
            conn.execute(
                "INSERT INTO semantic_reviews VALUES (?,?,NULL)",
                (d["id"], json.dumps(enriched, ensure_ascii=False)),
            )
            conn.execute(
                "INSERT OR REPLACE INTO clean_pii_scans VALUES (?,?,?,?,?,?)",
                (
                    d["id"],
                    changes["pii_content_hash"],
                    pii_version,
                    scan,
                    changes["pii_categories"],
                    manifest["created_at"],
                ),
            )
            stats["title_changed"] += title != d["title"]
            stats["topic_changed"] += enriched["topic"] != d["secondary_topic"]
            stats["audience_changed"] += enriched["audience"] != d["audience"]
            stats["text_changed"] += cleaned != text
            stats["non_student_or_news"] += enriched["content_role"] in {
                "NON_STUDENT",
                "NEWS",
                "RESULT_ANNOUNCEMENT",
            }
            stats["version_label_extracted"] += bool(enriched["version_labels"])
            if index % 500 == 0:
                print(f"Enriched {index}/{len(docs)}", flush=True)
        conn.execute(
            "UPDATE attachments SET rag_eligible=0,admission_status='BLOCKED',review_status='REVIEW'"
        )
        conn.execute("UPDATE weknora_mappings SET import_status='PENDING_REVALIDATION'")
        conn.commit()
        manifest["status"] = "ENRICHED"
        manifest["enrichment_counts"] = dict(stats)
        manifest["documents"] = len(docs)
        write_json(output / "batch.json", manifest)
        return manifest
    except Exception:
        manifest["status"] = "FAILED"
        write_json(output / "batch.json", manifest)
        raise
    finally:
        conn.close()


def review_reasons(d: dict, review: dict, text: str, taxonomy: dict) -> list[str]:
    reasons = []
    if review.get("source_content_hash") != d["source_content_hash"]:
        reasons.append("review_source_hash_mismatch")
    if review.get("source_url") != d["source_url"]:
        reasons.append("review_source_url_mismatch")
    if review.get("reviewer_type") != "AI_ASSISTED_EVIDENCE_REVIEW":
        reasons.append("reviewer_type_missing")
    if review.get("answer_scope") not in {
        "DATED_SOURCE_ONLY",
        "VERSION_SPECIFIC_ONLY",
        "FORM_ONLY",
    }:
        reasons.append("explicit_answer_scope_required")
    if not review.get("scope_note"):
        reasons.append("scope_note_required")
    if review.get("version_label") and (
        not review.get("version_family")
        or re.sub(r"\s+", "", str(review["version_label"]))
        not in re.sub(r"\s+", "", text)
    ):
        reasons.append("version_label_not_in_source")
    proofs = review.get("evidence", [])
    if not proofs or any(
        len(p) < 12 or canonical_text(p) not in canonical_text(text) for p in proofs
    ):
        reasons.append("review_evidence_missing_or_changed")
    if (
        review.get("secondary_topic") not in taxonomy["secondary_topics"]
        or review.get("secondary_topic") == "other"
    ):
        reasons.append("review_topic_required")
    if review.get("education_level") not in set(taxonomy["education_levels"]) - {
        "UNKNOWN"
    }:
        reasons.append("review_education_required")
    if review.get("audience") not in set(taxonomy["audiences"]) - {
        "UNKNOWN",
        "TEACHER",
    }:
        reasons.append("review_student_audience_required")
    if review.get("document_type") not in taxonomy["document_types"] or review.get(
        "document_type"
    ) in {
        "OTHER",
        "INDEX_PAGE",
        "SOURCE_DISCOVERY",
        "NEWS_RELEVANT",
        "RESULT_ANNOUNCEMENT",
    }:
        reasons.append("review_document_role_required")
    if d["content_role"] in {"NON_STUDENT", "NEWS", "RESULT_ANNOUNCEMENT"}:
        reasons.append("excluded_content_role")
    resolution = review.get("pii_resolution", {})
    resolved = (
        d["pii_scan_status"] == "CLEAR"
        and resolution.get("decision") == "HISTORICAL_FALSE_POSITIVE"
        and resolution.get("reviewed_entire_document") is True
        and len(resolution.get("rationale", "")) >= 30
        and len(resolution.get("evidence", "")) >= 20
        and canonical_text(resolution["evidence"]) in canonical_text(text)
    )
    if (d["pii_detected"] and not resolved) or d["pii_scan_status"] != "CLEAR":
        reasons.append("pii_not_cleared")
    if d["parse_status"] != "PARSED":
        reasons.append("parse_not_verified")
    if not d["publish_date"] and not review.get("version_label"):
        reasons.append("date_or_explicit_version_required")
    return reasons


def release(
    batch: Path,
    ledger_path: Path,
    school_path: Path,
    taxonomy_path: Path,
    raw_root: Path,
) -> dict:
    batch = batch.resolve()
    manifest = json.loads((batch / "batch.json").read_text(encoding="utf-8"))
    if manifest["status"] not in {"ENRICHED", "COMPLETE", "REVIEW_FAILED"}:
        raise ValueError("batch_not_enriched")
    taxonomy = yaml.safe_load(taxonomy_path.read_text(encoding="utf-8"))
    school = yaml.safe_load(school_path.read_text(encoding="utf-8"))
    ledger = json.loads(ledger_path.read_text(encoding="utf-8"))
    if ledger["input_sha256"] != manifest["input_sha256"]:
        raise ValueError("ledger_input_mismatch")
    reviews = ledger["reviews"]
    if len({r["document_id"] for r in reviews}) != len(reviews):
        raise ValueError("duplicate_reviews")
    conn = sqlite3.connect(batch / "catalog.sqlite")
    conn.row_factory = sqlite3.Row
    write_json(batch / "validation.json", {"status": "VALIDATING"})
    write_json(batch / "delivery-contract.json", {"status": "REQUIRES_REVALIDATION"})
    try:
        docs = {d["id"]: dict(d) for d in conn.execute("SELECT * FROM documents")}
        source_active = {
            s["id"]: bool(s["active"]) for s in conn.execute("SELECT * FROM sources")
        }
        reviewed_ids = {r["document_id"] for r in reviews}
        approved = []
        hashes = set()
        for review in reviews:
            d = docs[review["document_id"]]
            text = file_text(d, batch)
            raw = Path(re.sub(r"\\+", "/", d["raw_path"] or "")).resolve()
            if (
                not raw.is_relative_to(raw_root.resolve())
                or not raw.is_file()
                or sha(raw.read_bytes()) != d["raw_sha256"]
            ):
                raise ValueError("reviewed_raw_source_changed:" + d["id"])
            reasons = review_reasons(d, review, text, taxonomy)
            if d["is_attachment"]:
                parents = {
                    r[0]
                    for r in conn.execute(
                        "SELECT parent_id FROM clean_relations WHERE child_id=?",
                        (d["id"],),
                    )
                }
                if not parents & reviewed_ids:
                    reasons.append("reviewed_parent_required")
                if review.get("parent_document_id") not in parents & reviewed_ids:
                    reasons.append("reviewed_parent_selection_required")
            if d["content_hash"] in hashes:
                reasons.append("duplicate_release_content")
            hashes.add(d["content_hash"])
            if reasons:
                raise ValueError(d["id"] + ":" + ",".join(reasons))
            for field in [
                "secondary_topic",
                "education_level",
                "audience",
                "document_type",
                "answer_scope",
                "reviewer_type",
            ]:
                d[field] = review[field]
            d["primary_module"] = taxonomy["secondary_topics"][d["secondary_topic"]][
                "primary_module"
            ]
            d["review_status"] = "ACCEPTED"
            d["rejection_reason"] = ""
            d["canonical_document_id"] = None
            d["semantic_status"] = "EVIDENCE_REVIEWED"
            d["semantic_reasons"] = "[]"
            d["review_evidence"] = json.dumps(review, ensure_ascii=False)
            if d["pii_detected"] and review.get("pii_resolution"):
                d["pii_detected"] = 0
                d["pii_categories"] = "[]"
            if review.get("title"):
                d["title"] = review["title"]
            for field in [
                "valid_from",
                "valid_to",
                "deadline",
                "effective_date",
                "version_family",
            ]:
                if field in review:
                    d[field] = review[field]
            # Never derive current validity from latest upload/publication date.
            d["is_current"] = None
            d["current_confidence"] = None
            if review.get("version_label"):
                d["version_labels"] = json.dumps([review["version_label"]])
            if d["is_attachment"]:
                parent = docs[review["parent_document_id"]]
                d["parent_page_url"] = parent["source_url"]
                d["relation_status"] = "RESOLVED"
            gate = evaluate(
                d,
                taxonomy,
                school,
                source_active=source_active.get(d["source_id"], False),
            )
            if not gate.eligible:
                raise ValueError(d["id"] + ":" + ",".join(gate.reasons))
            d["rag_eligible"] = 1
            d["admission_status"] = "READY"
            d["admission_reasons"] = "[]"
            approved.append(d)
        # Export only the parent relationship explicitly selected in the review.
        ready_relations = [
            {
                "child_id": d["id"],
                "parent_id": json.loads(d["review_evidence"])["parent_document_id"],
                "evidence_url": d["parent_page_url"],
            }
            for d in approved
            if d["is_attachment"]
        ]
        by_parent = defaultdict(list)
        for edge in ready_relations:
            by_parent[edge["parent_id"]].append(edge["child_id"])
        for d in approved:
            d["attachment_count"] = len(by_parent[d["id"]])
            if d["is_attachment"]:
                parent_id = json.loads(d["review_evidence"])["parent_document_id"]
                d["knowledge_bundle_id"] = "reviewed_" + parent_id
            else:
                d["knowledge_bundle_id"] = (
                    "reviewed_" + d["id"] if by_parent[d["id"]] else None
                )
        contexts = ledger.get("context_decisions", [])
        for context in contexts:
            target = docs[context["document_id"]]
            evidence_doc = docs[context["evidence_document_id"]]
            if target["id"] in reviewed_ids:
                raise ValueError("context_hold_cannot_release_same_document")
            if (
                context["target_content_hash"] != target["source_content_hash"]
                or context["evidence_content_hash"]
                != evidence_doc["source_content_hash"]
                or canonical_text(context["evidence"])
                not in canonical_text(file_text(evidence_doc, batch))
            ):
                raise ValueError("context_evidence_changed")
            if context.get("version_label"):
                if evidence_doc["id"] not in reviewed_ids:
                    raise ValueError("version_rule_source_requires_review")
                if context["version_label"] not in json.loads(target["version_labels"]):
                    raise ValueError("version_label_not_in_document")
                target["version_family"] = context["version_family"]
                target["title"] = context["title"]
                target["secondary_topic"] = evidence_doc["secondary_topic"]
                target["primary_module"] = evidence_doc["primary_module"]
            target["semantic_reasons"] = json.dumps([context["hold_reason"]])
            target["semantic_status"] = "CONTEXT_REVIEWED_BODY_HELD"
        # Validate all normalized files, including quarantine, before publishing.
        for d in docs.values():
            file_text(d, batch)
        with conn:
            conn.execute(
                "CREATE TABLE IF NOT EXISTS quality_pii_resolutions (document_id TEXT PRIMARY KEY, source_content_hash TEXT NOT NULL, resolution_json TEXT NOT NULL)"
            )
            conn.execute(
                "UPDATE documents SET rag_eligible=0,admission_status='BLOCKED',semantic_status='UNREVIEWED',admission_reasons='[\"evidence_review_required\"]',review_status=CASE WHEN pii_detected=1 THEN 'PII_REVIEW' ELSE 'REVIEW' END"
            )
            conn.execute("UPDATE semantic_reviews SET review_json=NULL")
            for context in contexts:
                target = docs[context["document_id"]]
                fields = [
                    "title",
                    "version_family",
                    "secondary_topic",
                    "primary_module",
                    "semantic_reasons",
                    "semantic_status",
                ]
                conn.execute(
                    "UPDATE documents SET "
                    + ",".join(k + "=?" for k in fields)
                    + " WHERE id=?",
                    [target[k] for k in fields] + [target["id"]],
                )
                conn.execute(
                    "UPDATE semantic_reviews SET review_json=? WHERE document_id=?",
                    (json.dumps(context, ensure_ascii=False), target["id"]),
                )
            for d in approved:
                names = [k for k in d if k != "id"]
                conn.execute(
                    "UPDATE documents SET "
                    + ",".join(k + "=?" for k in names)
                    + " WHERE id=?",
                    [d[k] for k in names] + [d["id"]],
                )
                conn.execute(
                    "UPDATE semantic_reviews SET review_json=? WHERE document_id=?",
                    (d["review_evidence"], d["id"]),
                )
                review = json.loads(d["review_evidence"])
                if review.get("pii_resolution"):
                    conn.execute(
                        "INSERT OR REPLACE INTO quality_pii_resolutions VALUES (?,?,?)",
                        (
                            d["id"],
                            d["source_content_hash"],
                            json.dumps(review["pii_resolution"], ensure_ascii=False),
                        ),
                    )
            conn.execute(
                "UPDATE attachments SET rag_eligible=0,admission_status='BLOCKED',review_status='REVIEW'"
            )
            for d in approved:
                if d["is_attachment"]:
                    review = json.loads(d["review_evidence"])
                    conn.execute(
                        "UPDATE attachments SET rag_eligible=1,admission_status='READY',review_status='ACCEPTED',parent_document_id=?,parent_page_url=?,local_file_path=?,file_path=?,content_hash=?,pii_content_hash=?,knowledge_bundle_id=?,document_type=?,pii_scan_status='CLEAR',pii_detected=0 WHERE document_id=?",
                        (
                            review["parent_document_id"],
                            d["parent_page_url"],
                            d["normalized_path"],
                            d["normalized_path"],
                            d["content_hash"],
                            d["pii_content_hash"],
                            d["knowledge_bundle_id"],
                            d["document_type"],
                            d["id"],
                        ),
                    )
        exports = {
            "context_decisions.jsonl": contexts,
            "ready_documents.jsonl": approved,
            "ready_attachments.jsonl": [
                dict(r)
                for r in conn.execute(
                    "SELECT * FROM attachments WHERE admission_status='READY'"
                )
            ],
            "ready_relations.jsonl": ready_relations,
            "review_queue.jsonl": [
                {
                    "id": d["id"],
                    "title": d["title"],
                    "content_role": d["content_role"],
                    "suggested_topic": d["secondary_topic"],
                    "reasons": json.loads(d["semantic_reasons"])
                    + ["evidence_review_required"],
                }
                for d in docs.values()
                if d["id"] not in reviewed_ids
            ],
        }
        # Build bundles from the permitted edges only; never mirror blocked members.
        exports["ready_bundles.jsonl"] = [
            {
                "id": "reviewed_" + pid,
                "primary_document_id": pid,
                "member_ids": [pid] + sorted(set(children)),
            }
            for pid, children in sorted(by_parent.items())
            if children
        ]
        for name, rows in exports.items():
            (batch / name).write_text(
                "".join(json.dumps(d, ensure_ascii=False) + "\n" for d in rows),
                encoding="utf-8",
                newline="\n",
            )
        write_json(batch / "review-ledger.json", ledger)
        report = {
            "status": "PASSED",
            "delivery_scope": "OFFLINE_EVIDENCE_REVIEWED_CANARY",
            "documents_checked": len(docs),
            "ready_documents": len(approved),
            "ready_attachments": len(exports["ready_attachments.jsonl"]),
            "review_queue": len(exports["review_queue.jsonl"]),
            "ready_topics": dict(Counter(d["secondary_topic"] for d in approved)),
            "blocked_parent_exports": 0,
            "duplicate_ready_content": 0,
            "current_policy_claims": 0,
            "export_sha256": {
                name: sha((batch / name).read_bytes()) for name in exports
            },
            "review_ledger_sha256": sha((batch / "review-ledger.json").read_bytes()),
            "online_freshness_verified": False,
            "production_rag_evaluated": False,
        }
        write_json(batch / "validation.json", report)
        manifest["status"] = "COMPLETE"
        manifest["release"] = report
        write_json(batch / "batch.json", manifest)
        return report
    except Exception as exc:
        write_json(batch / "validation.json", {"status": "FAILED", "error": str(exc)})
        manifest["status"] = "REVIEW_FAILED"
        write_json(batch / "batch.json", manifest)
        raise
    finally:
        conn.close()
