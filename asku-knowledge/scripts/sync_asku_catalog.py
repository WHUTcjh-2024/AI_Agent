"""Upsert a completed WeKnora import report into the AskU knowledge catalog."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
from pathlib import Path
from urllib.parse import urlsplit


def load_jsonl(path: Path) -> list[dict]:
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]


def sql_text(value: object | None) -> str:
    if value is None:
        return "NULL"
    return "'" + str(value).replace("'", "''") + "'"


def sql_json(value: object) -> str:
    return sql_text(json.dumps(value, ensure_ascii=False, separators=(",", ":"))) + "::jsonb"


def sql_date(value: object | None) -> str:
    text = str(value or "")
    return sql_text(text) + "::date" if re.fullmatch(r"\d{4}-\d{2}-\d{2}", text) else "NULL"


def authority(source: dict) -> str:
    level = source.get("source_level", "")
    if "college" in level:
        return "OFFICIAL_COLLEGE"
    if "policy" in level:
        return "OFFICIAL_SCHOOL"
    return "OFFICIAL_DEPARTMENT"


def department(source: dict) -> str:
    org = source.get("org", "")
    for name in ("网络信息中心", "本科生招生办公室", "迎新网站", "交通与物流工程学院", "本科生院"):
        if name in org:
            return name
    return ""


def build_sql(manifest_path: Path, import_report_path: Path) -> tuple[str, dict]:
    manifest = load_jsonl(manifest_path)
    report = json.loads(import_report_path.read_text(encoding="utf-8"))
    if report.get("status") != "PASSED":
        raise ValueError("weknora_import_report_not_passed")
    imported = {row["external_id"]: row for row in report["results"]}
    if set(imported) != {row["external_id"] for row in manifest}:
        raise ValueError("manifest_and_import_report_do_not_match")
    if any(row.get("parse_status") != "completed" or row.get("enable_status") != "enabled" for row in imported.values()):
        raise ValueError("weknora_documents_not_ready")

    sources = {
        row["source_id"]: row
        for row in load_jsonl(manifest_path.parent / "audit" / "sources.jsonl")
    }
    statements = ["BEGIN;"]
    for row in manifest:
        metadata = row["metadata"]
        source = sources[metadata["source_id"]]
        parsed = urlsplit(source["url"])
        source_authority = authority(source)
        college_slug = parsed.hostname.split(".")[0] if source.get("scope") == "college" and parsed.hostname else ""
        statements.append(
            "INSERT INTO knowledge.sources "
            "(id,school_id,source_name,department,source_type,authority,official_url,canonical_url,source_key,base_url,domains,authority_type,priority,education_level,active,name_confirmed,discovered_from,last_checked_at,college_slug) VALUES ("
            + ",".join([
                sql_text(source["source_id"]), sql_text("whut"), sql_text(source["org"]), sql_text(department(source)),
                sql_text(source.get("source_level", "official")), sql_text(source_authority), sql_text(source["url"]),
                sql_text(source["url"]), sql_text("gold-v2:" + source["source_id"].lower()),
                sql_text(f"{parsed.scheme}://{parsed.netloc}"), sql_json([parsed.hostname] if parsed.hostname else []),
                sql_text(source_authority), sql_text("P1"), sql_text(metadata["education_level"]), "TRUE", "TRUE",
                sql_text("AskU_WHUT_100_Gold_V2_2026-09-06.zip"), sql_text(source["last_verified_at"] + "T00:00:00Z"),
                sql_text(college_slug),
            ])
            + ") ON CONFLICT (id) DO UPDATE SET source_name=EXCLUDED.source_name,department=EXCLUDED.department,"
            "source_type=EXCLUDED.source_type,authority=EXCLUDED.authority,official_url=EXCLUDED.official_url,"
            "canonical_url=EXCLUDED.canonical_url,active=TRUE,name_confirmed=TRUE,last_checked_at=EXCLUDED.last_checked_at,updated_at=now();"
        )

        document_path = (manifest_path.parent / row["file"]).resolve()
        content = document_path.read_text(encoding="utf-8")
        if "\ufffd" in content:
            raise ValueError("unicode_corruption:" + row["external_id"])
        mirror_urls = [] if metadata["source_url"] == metadata.get("fetched_url") else [metadata.get("fetched_url")]
        source_authority = authority(source)
        statements.append(
            "INSERT INTO knowledge.documents "
            "(id,school_id,source_id,title,publish_date,document_type,parent_page_url,knowledge_bundle_id,freshness,local_file_path,education_level,primary_module,secondary_topic,audience,source_url,canonical_url,academic_year,freshness_type,topic_family,version_family,is_current,current_confidence,source_authority,authority_score,quality_score,quality_band,classification_confidence,raw_path,normalized_path,mime_type,file_hash,content_hash,simhash,rag_eligible,review_status,rejection_reason,mirror_urls,pii_detected,pii_categories,attachment_count,depth,content_chars,is_attachment,text_length,fetched_at) VALUES ("
            + ",".join([
                sql_text(row["external_id"]), sql_text("whut"), sql_text(source["source_id"]), sql_text(metadata["title"]),
                sql_date(metadata.get("published_at")), sql_text(metadata["document_type"]), sql_text(""),
                sql_text(metadata["knowledge_bundle_id"]), sql_text(metadata["version_status"]), sql_text(document_path),
                sql_text(metadata["education_level"]), sql_text(metadata["primary_module"]), sql_text(metadata["topic"]),
                sql_text(metadata["audience"]), sql_text(metadata["source_url"]), sql_text(metadata.get("fetched_url") or metadata["source_url"]),
                sql_text(str(metadata.get("published_at") or "")[:4]), sql_text(metadata["answer_scope"]), sql_text(metadata["topic"]),
                sql_text(metadata["topic"]), "TRUE", "1.000", sql_text(source_authority), "100.00", "100", sql_text("A"),
                "1.000", sql_text(""), sql_text(document_path), sql_text("text/markdown"), sql_text(row["file_sha256"]),
                sql_text(row["clean_content_sha256"]), sql_text(""), "TRUE", sql_text("ACCEPTED"), sql_text(""),
                sql_json(mirror_urls), "FALSE", sql_json([]), "0", "0", str(len(content)), "FALSE", str(len(content)),
                sql_text(metadata["last_verified_at"] + "T00:00:00Z"),
            ])
            + ") ON CONFLICT (id) DO UPDATE SET source_id=EXCLUDED.source_id,title=EXCLUDED.title,publish_date=EXCLUDED.publish_date,"
            "document_type=EXCLUDED.document_type,knowledge_bundle_id=EXCLUDED.knowledge_bundle_id,freshness=EXCLUDED.freshness,"
            "local_file_path=EXCLUDED.local_file_path,education_level=EXCLUDED.education_level,primary_module=EXCLUDED.primary_module,"
            "secondary_topic=EXCLUDED.secondary_topic,audience=EXCLUDED.audience,source_url=EXCLUDED.source_url,canonical_url=EXCLUDED.canonical_url,"
            "is_current=TRUE,current_confidence=1.000,source_authority=EXCLUDED.source_authority,authority_score=100.00,quality_score=100,"
            "quality_band='A',classification_confidence=1.000,normalized_path=EXCLUDED.normalized_path,mime_type='text/markdown',"
            "file_hash=EXCLUDED.file_hash,content_hash=EXCLUDED.content_hash,rag_eligible=TRUE,review_status='ACCEPTED',rejection_reason='',"
            "pii_detected=FALSE,pii_categories='[]'::jsonb,content_chars=EXCLUDED.content_chars,text_length=EXCLUDED.text_length,updated_at=now();"
        )

        result = imported[row["external_id"]]
        statements.append(
            "INSERT INTO knowledge.weknora_mappings "
            "(school_id,weknora_knowledge_id,asku_document_id,attachment_id,weknora_knowledge_base_id,import_status,imported_at,last_sync_at,file_hash,last_error,knowledge_base_id) VALUES ("
            + ",".join([
                sql_text("whut"), sql_text(result["knowledge_id"]), sql_text(row["external_id"]), "NULL",
                sql_text(report["knowledge_base_id"]), sql_text("IMPORTED"), "now()", "now()", sql_text(row["file_sha256"]),
                sql_text(""), sql_text(report["knowledge_base_id"]),
            ])
            + ") ON CONFLICT (school_id,weknora_knowledge_id) DO UPDATE SET asku_document_id=EXCLUDED.asku_document_id,"
            "weknora_knowledge_base_id=EXCLUDED.weknora_knowledge_base_id,knowledge_base_id=EXCLUDED.knowledge_base_id,"
            "import_status='IMPORTED',imported_at=now(),last_sync_at=now(),file_hash=EXCLUDED.file_hash,last_error='',updated_at=now();"
        )
    statements.extend([
        "DO $$ BEGIN IF (SELECT count(*) FROM knowledge.weknora_mappings WHERE school_id='whut' AND knowledge_base_id="
        + sql_text(report["knowledge_base_id"])
        + " AND import_status='IMPORTED' AND asku_document_id IN ("
        + ",".join(sql_text(row["external_id"]) for row in manifest)
        + f")) <> {len(manifest)} THEN RAISE EXCEPTION 'mapping_count_mismatch'; END IF; END $$;",
        "COMMIT;",
    ])
    return "\n".join(statements) + "\n", {"sources": len(manifest), "documents": len(manifest), "mappings": len(manifest)}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--import-report", type=Path, required=True)
    parser.add_argument("--postgres-container", default="asku-phase2-postgres-1")
    parser.add_argument("--database", default="asku")
    parser.add_argument("--user", default="asku")
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()

    sql, counts = build_sql(args.manifest, args.import_report)
    process = subprocess.run(
        ["docker", "exec", "-i", args.postgres_container, "psql", "-v", "ON_ERROR_STOP=1", "-U", args.user, "-d", args.database, "-q"],
        input=sql.encode("utf-8"),
        capture_output=True,
        check=False,
    )
    if process.returncode:
        error = process.stderr.decode("utf-8", errors="replace").strip()
        raise RuntimeError("AskU catalog transaction failed: " + error)
    result = {"status": "PASSED", **counts, "knowledge_base_id": json.loads(args.import_report.read_text(encoding="utf-8"))["knowledge_base_id"]}
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
