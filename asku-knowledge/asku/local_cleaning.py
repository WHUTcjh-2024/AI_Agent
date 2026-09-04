"""Rebuild an offline staging batch; PostgreSQL remains the production catalog.

Raw files and the source SQLite snapshot are immutable inputs. Each output batch
has a separate catalog, versioned extraction cache, evidence relations and exports.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import sqlite3
from collections import Counter, defaultdict
from concurrent.futures import ProcessPoolExecutor, as_completed
from contextlib import closing
from datetime import date, datetime, timezone
from pathlib import Path
from urllib.parse import unquote, urljoin, urlsplit, urlunsplit

import yaml
from lxml import html as lxml_html

from .admission import RULE_VERSION, canonical_text, evaluate, official_url, text_hash
from .copy_dump import read_copy
from .document_parser import PARSER_VERSION, Parsed, finish, parse_file
from .normalizer import normalize_html
from .pii import detect_pii


def json_write(path: Path, value) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(
        json.dumps(value, ensure_ascii=False, indent=2), encoding="utf-8"
    )
    os.replace(temporary, path)


def safe_raw_path(value: str, raw_root: Path) -> Path:
    # Legacy COPY importers retained doubled backslashes. Windows normalized
    # them implicitly; normalize once and still enforce the configured boundary.
    path = Path(re.sub(r"\\+", "/", value or ""))
    if not path.is_absolute():
        path = raw_root / path
    path = path.resolve()
    if not path.is_relative_to(raw_root.resolve()):
        raise ValueError("raw_path_outside_root")
    return path


def url_key(value: str) -> str:
    p = urlsplit(value)
    # HTTP and HTTPS mirrors use the same school resource. Preserve the query:
    # download?id=1 and download?id=2 must never be conflated.
    return urlunsplit(("https", (p.netloc or "").lower(), unquote(p.path), p.query, ""))


def decode_html(data: bytes) -> str:
    """Prefer strict known encodings; never run statistical detection on MBs."""
    for encoding in ("utf-8-sig", "gb18030"):
        try:
            return data.decode(encoding, errors="strict")
        except UnicodeError:
            pass
    return data.decode("utf-8", errors="replace")


def extract_job(job: dict) -> dict:
    cache = Path(job["cache"])
    if cache.is_file():
        cached = json.loads(cache.read_text(encoding="utf-8"))
        if (
            cached.get("raw_sha256") == job["raw_sha256"]
            and cached.get("profile") == job["profile"]
        ):
            return cached
    path = Path(job["path"])
    metadata = {}
    if job["attachment"]:
        result = parse_file(
            path,
            tesseract=job["tesseract"],
            legacy_dir=Path(job["legacy_dir"]),
            archive_dir=Path(job["archive_dir"]),
        )
    else:
        try:
            data = path.read_bytes()
            html = decode_html(data)
            page = normalize_html(html, base_url=job["url"])
            result = finish(Parsed("PARSED", "html", page.markdown))
            # Metadata/heading text cannot make an empty body appear complete.
            if len(canonical_text(page.text)) < 100:
                result.status = "REVIEW"
                result.errors.append("short_body")
            metadata = {
                "title": page.title,
                "publish_date": page.publish_date,
                "table_count": page.table_count,
            }
        except Exception as exc:
            result = Parsed("FAILED", "html", errors=[type(exc).__name__])
    output = {
        **result.as_dict(),
        "metadata": metadata,
        "raw_sha256": job["raw_sha256"],
        "profile": job["profile"],
        "document_id": job["id"],
    }
    json_write(cache, output)
    return output


def rebuild_links(documents: list[dict], crawler: dict, school: dict, raw_root: Path):
    """Use exact resolved anchor URLs, never bare filenames or fabricated IDs."""
    by_id = {d["id"]: d for d in documents}
    webpages = {
        url_key(d["source_url"]): d for d in documents if not d["is_attachment"]
    }
    targets = defaultdict(set)
    for d in documents:
        if d["is_attachment"] and official_url(d["source_url"], school):
            targets[url_key(d["source_url"])].add(d["id"])
    pages = {}
    for row in crawler.get("cck.raw_documents", []):
        if row.get("is_attachment") not in ("f", False, 0) or not official_url(
            row.get("url", ""), school
        ):
            continue
        pages.setdefault(url_key(row["url"]), row)
    for key, doc in webpages.items():
        pages[key] = {
            "url": doc["source_url"],
            "raw_path": doc["raw_path"],
            "title": doc["title"],
            "source_id": doc["source_id"],
        }
    relations = set()
    errors = Counter()
    new_parents = []
    for index, (key, row) in enumerate(pages.items(), 1):
        if index % 1000 == 0:
            print(f"Linked source pages {index}/{len(pages)}", flush=True)
        try:
            path = safe_raw_path(row["raw_path"], raw_root)
            data = path.read_bytes()
            tree = lxml_html.fromstring(decode_html(data))
            matches = set()
            base = row["url"]
            base_tags = tree.xpath("//base/@href")
            if base_tags and official_url(urljoin(base, base_tags[0]), school):
                base = urljoin(base, base_tags[0])
            for href in tree.xpath("//a/@href"):
                target = urljoin(base, href.strip())
                if official_url(target, school):
                    matches.update(targets.get(url_key(target), ()))
            if not matches:
                continue
            parent = webpages.get(key)
            if parent is None:
                # Recover only source pages actually needed by a known attachment.
                ident = "whp_" + hashlib.sha256(key.encode()).hexdigest()[:32]
                parent = {k: None for k in documents[0]}
                parent.update(
                    {
                        "id": ident,
                        "school_id": school["school_id"],
                        "source_id": row["source_id"],
                        "title": row["title"] or "附件来源通知",
                        "source_url": row["url"],
                        "canonical_url": row["url"],
                        "raw_path": str(path),
                        "is_attachment": 0,
                        "review_status": "REVIEW",
                        "rag_eligible": 0,
                        "pii_detected": 0,
                        "secondary_topic": "other",
                        "primary_module": "OTHER",
                        "education_level": "UNKNOWN",
                        "audience": "UNKNOWN",
                        "document_type": "OTHER",
                        "rejection_reason": "recovered_parent_requires_review",
                    }
                )
                webpages[key] = parent
                by_id[ident] = parent
                new_parents.append(parent)
            relations.update((child, parent["id"]) for child in matches)
        except (OSError, ValueError, TypeError) as exc:
            errors[type(exc).__name__] += 1
    documents.extend(new_parents)
    return sorted(relations), {
        "pages_examined": len(pages),
        "recovered_parent_documents": len(new_parents),
        "source_page_errors": dict(errors),
    }


EXTRA_COLUMNS = {
    "parse_status": "TEXT NOT NULL DEFAULT 'PENDING'",
    "parser_version": "TEXT NOT NULL DEFAULT ''",
    "parse_errors": "TEXT NOT NULL DEFAULT '[]'",
    "pii_scan_status": "TEXT NOT NULL DEFAULT 'PENDING'",
    "pii_content_hash": "TEXT NOT NULL DEFAULT ''",
    "pii_rule_version": "TEXT NOT NULL DEFAULT ''",
    "relation_status": "TEXT NOT NULL DEFAULT 'UNRESOLVED'",
    "admission_status": "TEXT NOT NULL DEFAULT 'BLOCKED'",
    "admission_reasons": "TEXT NOT NULL DEFAULT '[]'",
    "admission_version": "TEXT NOT NULL DEFAULT ''",
    "raw_sha256": "TEXT NOT NULL DEFAULT ''",
    "normalized_sha256": "TEXT NOT NULL DEFAULT ''",
    "parse_format": "TEXT NOT NULL DEFAULT ''",
    "publish_date_evidence": "TEXT NOT NULL DEFAULT ''",
}


def run_batch(
    *,
    source_db: Path,
    crawler_dump: Path,
    school_path: Path,
    taxonomy_path: Path,
    raw_root: Path,
    output: Path,
    legacy_dir: Path,
    tesseract: str | None,
    workers: int = 4,
    archive_dir: Path | None = None,
):
    output.mkdir(parents=True, exist_ok=False)
    archive_dir = archive_dir or legacy_dir / "archives"
    school = yaml.safe_load(school_path.read_text(encoding="utf-8"))
    taxonomy = yaml.safe_load(taxonomy_path.read_text(encoding="utf-8"))
    now = datetime.now(timezone.utc).isoformat()
    with closing(
        sqlite3.connect(source_db.resolve().as_uri() + "?mode=ro", uri=True)
    ) as source:
        source.row_factory = sqlite3.Row
        docs = [
            dict(r)
            for r in source.execute(
                "SELECT * FROM documents WHERE school_id=?", (school["school_id"],)
            )
        ]
        sources = {
            r["id"]: dict(r)
            for r in source.execute(
                "SELECT * FROM sources WHERE school_id=?", (school["school_id"],)
            )
        }
        if not docs:
            raise ValueError("school_has_no_documents")
        if any(not re.fullmatch(r"[A-Za-z0-9_-]+", d["id"]) for d in docs):
            raise ValueError("unsafe_document_identifier")
        conn = sqlite3.connect(output / "catalog.sqlite")
        source.backup(conn)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA foreign_keys=ON")
    existing = {r[1] for r in conn.execute("PRAGMA table_info(documents)")}
    for name, definition in EXTRA_COLUMNS.items():
        if name not in existing:
            conn.execute(f"ALTER TABLE documents ADD COLUMN {name} {definition}")
    conn.execute(
        "CREATE TABLE IF NOT EXISTS clean_relations (child_id TEXT NOT NULL REFERENCES documents(id), parent_id TEXT NOT NULL REFERENCES documents(id), evidence_url TEXT NOT NULL, PRIMARY KEY(child_id,parent_id))"
    )
    conn.execute(
        "CREATE TABLE IF NOT EXISTS clean_pii_scans (document_id TEXT PRIMARY KEY REFERENCES documents(id), content_hash TEXT NOT NULL, rule_version TEXT NOT NULL, status TEXT NOT NULL, categories TEXT NOT NULL, scanned_at TEXT NOT NULL)"
    )
    crawler = read_copy(crawler_dump)
    original_count = len(docs)
    print("Rebuilding exact URL relationships...", flush=True)
    relations, link_stats = rebuild_links(docs, crawler, school, raw_root)
    print(json.dumps(link_stats), flush=True)
    profile = hashlib.sha256(
        json.dumps(
            {
                "parser": PARSER_VERSION,
                "ocr": bool(tesseract),
                "normalizer": hashlib.sha256(
                    Path(__file__).with_name("normalizer.py").read_bytes()
                ).hexdigest(),
                "parser_code": hashlib.sha256(
                    Path(__file__).with_name("document_parser.py").read_bytes()
                ).hexdigest(),
                "archive_files": sorted(p.stem for p in archive_dir.glob("*.zip")),
                "legacy_files": sorted(p.stem for p in legacy_dir.glob("*.txt")),
            },
            sort_keys=True,
        ).encode()
    ).hexdigest()
    cache_root = output.parent / "extraction-cache" / profile
    jobs, results = [], {}
    by_id = {d["id"]: d for d in docs}
    for d in docs:
        try:
            path = safe_raw_path(d["raw_path"], raw_root)
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            cache_key = hashlib.sha256(
                (digest + d["source_url"] + d["id"]).encode()
            ).hexdigest()
            jobs.append(
                {
                    "id": d["id"],
                    "path": str(path),
                    "url": d["source_url"],
                    "attachment": bool(d["is_attachment"]),
                    "raw_sha256": digest,
                    "cache": str(cache_root / (cache_key + ".json")),
                    "profile": profile,
                    "tesseract": tesseract,
                    "legacy_dir": str(legacy_dir),
                    "archive_dir": str(archive_dir),
                }
            )
        except (OSError, ValueError):
            results[d["id"]] = Parsed(
                "FAILED", "missing", errors=["raw_missing_or_outside_root"]
            ).as_dict()
    print(f"Parsing {len(jobs)} files with {workers} workers", flush=True)
    with ProcessPoolExecutor(max_workers=workers) as pool:
        futures = {pool.submit(extract_job, job): job["id"] for job in jobs}
        for index, future in enumerate(as_completed(futures), 1):
            ident = futures[future]
            try:
                results[ident] = future.result()
            except Exception as exc:
                results[ident] = Parsed(
                    "FAILED", "unknown", errors=[type(exc).__name__]
                ).as_dict()
            if index % 100 == 0 or index == len(jobs):
                print(f"Parsed {index}/{len(jobs)}", flush=True)
    parent_sets = defaultdict(list)
    for child, parent in relations:
        parent_sets[child].append(parent)
    pii_version = hashlib.sha256(
        Path(__file__).with_name("pii.py").read_bytes()
    ).hexdigest()
    groups = defaultdict(list)
    for index, d in enumerate(docs):
        result = results[d["id"]]
        text = result["text"]
        path = output / "normalized" / (d["id"] + ".md")
        path.parent.mkdir(exist_ok=True)
        if text:
            # Preserve the exact bytes fingerprinted below, including OCR CRLF.
            path.write_bytes(text.encode("utf-8"))
        digest = text_hash(text) if text else ""
        d.update(
            {
                "parse_status": result["status"],
                "parse_format": result["format"],
                "parser_version": PARSER_VERSION,
                "parse_errors": json.dumps(result["errors"]),
                "normalized_path": str(path.resolve()) if text else "",
                "local_file_path": str(path.resolve()) if text else "",
                "content_chars": len(canonical_text(text)),
                "text_length": len(canonical_text(text)),
                "content_hash": digest,
                "raw_sha256": result.get("raw_sha256", ""),
                "normalized_sha256": hashlib.sha256(text.encode()).hexdigest()
                if text
                else "",
                "canonical_document_id": None,
                "admission_version": RULE_VERSION,
                "canonical_url": d["source_url"],
                "updated_at": now,
            }
        )
        # Only explicit publication metadata from original HTML is authoritative.
        if not d["is_attachment"]:
            publish = result.get("metadata", {}).get("publish_date")
            d["publish_date"] = (
                publish if publish and publish <= date.today().isoformat() else None
            )
            d["publish_date_evidence"] = (
                "raw_html_publish_metadata" if d["publish_date"] else ""
            )
        if text and len(text) <= 100_000:
            finding = detect_pii(text, title=d["title"] or "", tables_markdown=text)
            categories = finding.categories
            d["pii_scan_status"] = "HIT" if finding.is_blocking else "CLEAR"
            d["pii_detected"] = int(finding.is_blocking or bool(d["pii_detected"]))
            if finding.is_blocking:
                d["review_status"] = "PII_REVIEW"
            d["pii_categories"] = json.dumps(categories)
        else:
            categories = []
            d["pii_scan_status"] = "NOT_SCANNED"
        d["pii_content_hash"] = digest if d["pii_scan_status"] != "NOT_SCANNED" else ""
        d["pii_rule_version"] = pii_version
        d["relation_status"] = (
            "RESOLVED"
            if parent_sets[d["id"]]
            else "UNRESOLVED"
            if d["is_attachment"]
            else "NOT_APPLICABLE"
        )
        if d["secondary_topic"] == "other" and d["review_status"] == "ACCEPTED":
            d["review_status"] = "UNCERTAIN"
        if result["status"] != "PARSED" and d["review_status"] == "ACCEPTED":
            d["review_status"] = "REVIEW"
        if d["pii_detected"] and d["review_status"] == "ACCEPTED":
            d["review_status"] = "PII_REVIEW"
        if digest and result["status"] == "PARSED":
            groups[digest].append(d)
    # Prefer an already reviewed record as canonical, then an actual webpage.
    for group in groups.values():
        group.sort(
            key=lambda d: (
                d["review_status"] != "ACCEPTED",
                bool(d["is_attachment"]),
                d["id"],
            )
        )
        for duplicate in group[1:]:
            duplicate["canonical_document_id"] = group[0]["id"]
    bundles = {}
    for d in docs:
        parents = sorted(
            parent_sets[d["id"]],
            key=lambda ident: (by_id[ident]["review_status"] != "ACCEPTED", ident),
        )
        if d["is_attachment"]:
            d["parent_page_url"] = by_id[parents[0]]["source_url"] if parents else ""
            d["knowledge_bundle_id"] = (
                "kb_"
                + hashlib.sha256(
                    (school["school_id"] + parents[0]).encode()
                ).hexdigest()[:24]
                if parents
                else ""
            )
            if parents:
                parent = by_id[parents[0]]
                d["publish_date"] = parent["publish_date"]
                d["publish_date_evidence"] = (
                    "parent:" + parent["id"] if parent["publish_date"] else ""
                )
                bundles.setdefault(
                    d["knowledge_bundle_id"], {"parent": parent, "children": []}
                )["children"].append(d["id"])
        else:
            d["parent_page_url"] = d["source_url"]
            d["knowledge_bundle_id"] = ""
        outcome = evaluate(
            d,
            taxonomy,
            school,
            source_active=bool(sources.get(d["source_id"], {}).get("active")),
        )
        d["rag_eligible"] = int(outcome.eligible)
        d["admission_status"] = "READY" if outcome.eligible else "BLOCKED"
        d["admission_reasons"] = json.dumps(outcome.reasons)
        d["rejection_reason"] = ";".join(outcome.reasons)
    for bundle_id, bundle in bundles.items():
        bundle["parent"]["knowledge_bundle_id"] = bundle_id
    with conn:
        for d in docs:
            if conn.execute(
                "SELECT 1 FROM documents WHERE id=?", (d["id"],)
            ).fetchone():
                names = [k for k in d if k != "id"]
                conn.execute(
                    "UPDATE documents SET "
                    + ",".join('"' + k + '"=?' for k in names)
                    + " WHERE id=?",
                    [d[k] for k in names] + [d["id"]],
                )
            else:
                # Use database defaults for unspecified columns on recovered source pages.
                values = {k: v for k, v in d.items() if v is not None}
                conn.execute(
                    "INSERT INTO documents ("
                    + ",".join('"' + k + '"' for k in values)
                    + ") VALUES ("
                    + ",".join("?" for _ in values)
                    + ")",
                    list(values.values()),
                )
            conn.execute(
                "INSERT OR REPLACE INTO clean_pii_scans VALUES (?,?,?,?,?,?)",
                (
                    d["id"],
                    d["pii_content_hash"],
                    pii_version,
                    d["pii_scan_status"],
                    d.get("pii_categories") or "[]",
                    now,
                ),
            )
        # These mutations affect only this new staging catalog.
        conn.execute("DELETE FROM clean_relations")
        conn.execute(
            "DELETE FROM knowledge_bundles WHERE school_id=?", (school["school_id"],)
        )
        for bundle_id, bundle in bundles.items():
            parent = bundle["parent"]
            conn.execute(
                "INSERT INTO knowledge_bundles(id,school_id,title,primary_document_id,document_count,attachment_count,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)",
                (
                    bundle_id,
                    school["school_id"],
                    parent["title"],
                    parent["id"],
                    len(bundle["children"]) + 1,
                    len(bundle["children"]),
                    now,
                    now,
                ),
            )
        for child, parent in relations:
            conn.execute(
                "INSERT INTO clean_relations VALUES (?,?,?)",
                (child, parent, by_id[parent]["source_url"]),
            )
        for d in docs:
            if d["is_attachment"]:
                parents = sorted(
                    parent_sets[d["id"]],
                    key=lambda ident: (
                        by_id[ident]["review_status"] != "ACCEPTED",
                        ident,
                    ),
                )
                conn.execute(
                    "UPDATE attachments SET parent_document_id=?,parent_page_url=?,knowledge_bundle_id=?,local_file_path=?,file_path=?,rag_eligible=?,review_status=?,pii_detected=?,updated_at=? WHERE document_id=?",
                    (
                        parents[0] if parents else None,
                        d["parent_page_url"],
                        d["knowledge_bundle_id"],
                        d["normalized_path"],
                        d["normalized_path"],
                        d["rag_eligible"],
                        d["review_status"],
                        d["pii_detected"],
                        now,
                        d["id"],
                    ),
                )
        # Reparsed text must be re-indexed; stale remote IDs cannot certify this batch.
        conn.execute(
            "UPDATE weknora_mappings SET import_status='PENDING_REVALIDATION' WHERE school_id=?",
            (school["school_id"],),
        )
    violations = conn.execute(
        "SELECT count(*) FROM documents WHERE school_id=? AND rag_eligible=1 AND (review_status!='ACCEPTED' OR secondary_topic='other' OR parse_status!='PARSED' OR pii_scan_status!='CLEAR' OR pii_detected=1 OR content_chars<100 OR content_chars>100000 OR admission_status!='READY')",
        (school["school_id"],),
    ).fetchone()[0]
    if violations or conn.execute("PRAGMA foreign_key_check").fetchall():
        raise RuntimeError("batch_integrity_failed")
    ready = [d for d in docs if d["rag_eligible"]]
    with (output / "ready_documents.jsonl").open("w", encoding="utf-8") as stream:
        for d in ready:
            stream.write(json.dumps(d, ensure_ascii=False) + "\n")
    with (output / "review_queue.jsonl").open("w", encoding="utf-8") as stream:
        for d in docs:
            if not d["rag_eligible"]:
                stream.write(
                    json.dumps(
                        {
                            "id": d["id"],
                            "reasons": json.loads(d["admission_reasons"]),
                            "parse_errors": json.loads(d["parse_errors"]),
                        },
                        ensure_ascii=False,
                    )
                    + "\n"
                )
    summary = {
        "batch_created_at": now,
        "input_sha256": hashlib.sha256(source_db.read_bytes()).hexdigest(),
        "original_documents": original_count,
        "documents": len(docs),
        "link_recovery": link_stats,
        "parse_status": dict(Counter(d["parse_status"] for d in docs)),
        "attachment_parse_status": dict(
            Counter(d["parse_status"] for d in docs if d["is_attachment"])
        ),
        "format_counts": dict(Counter(d["parse_format"] for d in docs)),
        "parse_errors": dict(
            Counter(e for d in docs for e in json.loads(d["parse_errors"]))
        ),
        "ready_documents": len(ready),
        "ready_attachments": sum(bool(d["is_attachment"]) for d in ready),
        "linked_attachments": sum(d["relation_status"] == "RESOLVED" for d in docs),
        "relations": len(relations),
        "bundles": len(bundles),
        "pii_scan_status": dict(Counter(d["pii_scan_status"] for d in docs)),
        "ocr_pages": sum(r.get("ocr_pages", 0) for r in results.values()),
        "admission_reasons": dict(
            Counter(r for d in docs for r in json.loads(d["admission_reasons"]))
        ),
        "gate_violations": violations,
        "knowledge_imported": False,
    }
    json_write(output / "summary.json", summary)
    json_write(
        output / "batch.json",
        {
            "status": "COMPLETE",
            "source_db": str(source_db.resolve()),
            "crawler_dump": str(crawler_dump.resolve()),
            "parser_profile": profile,
            "rules": RULE_VERSION,
        },
    )
    conn.close()
    print(json.dumps(summary, ensure_ascii=False, indent=2), flush=True)
    return summary
