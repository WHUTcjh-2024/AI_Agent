"""Normalize the AskU WHUT Gold V2 archive into auditable data layers.

The supplied Q&A rows are evaluation and routing inputs.  They are never
marked as RAG eligible; only independently fetched official source documents
may pass the existing question-first admission pipeline.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import zipfile
from collections import defaultdict
from pathlib import Path, PurePosixPath
from urllib.parse import urlsplit, urlunsplit

SOURCE_FIXES = {
    "SRC014": {
        "url": "http://jwc.whut.edu.cn/uploads/file/20211012/20211012154930_9902.pdf",
        "title": "武汉理工大学普通全日制本科学生学籍管理规定（校教字〔2020〕34号）",
        "published_at": "2020-09-03",
        "fix_reason": "原 naoep 附件返回 404；替换为内容哈希一致的本科生院公开附件",
    },
    "SRC033": {
        "url": "https://nic.whut.edu.cn/fwzn/sw/202410/t20241010_1285244.shtml",
        "fix_reason": "原链接缺少 /sw/ 路径段并返回 404",
    },
    "SRC025": {
        "published_at": "2026-09-04",
        "fix_reason": "按湖北省教育考试院正文落款修正发布日期",
    },
}

PRIVATE_HOSTS = {"jwxk.whut.edu.cn"}
TOOL_HOSTS = {"gis.whut.edu.cn"}
OFFICIAL_EXTERNAL_HOSTS = {"www.hbea.edu.cn"}


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def canonical_url(url: str) -> str:
    parsed = urlsplit(url.strip())
    return urlunsplit((parsed.scheme.lower(), parsed.netloc.lower(), parsed.path or "/", parsed.query, ""))


def load_jsonl_from_zip(archive: zipfile.ZipFile, suffix: str) -> list[dict]:
    names = [name for name in archive.namelist() if name.endswith(suffix)]
    if len(names) != 1:
        raise ValueError(f"expected_one_archive_member:{suffix}:{len(names)}")
    text = archive.read(names[0]).decode("utf-8-sig")
    return [json.loads(line) for line in text.splitlines() if line.strip()]


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")


def write_jsonl(path: Path, rows: list[dict]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        "".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows),
        encoding="utf-8",
        newline="\n",
    )


def validate_archive(path: Path) -> dict:
    with zipfile.ZipFile(path) as archive:
        names: set[str] = set()
        total = 0
        for info in archive.infolist():
            member = PurePosixPath(info.filename.replace("\\", "/"))
            if member.is_absolute() or ".." in member.parts or re.match(r"^[A-Za-z]:", info.filename):
                raise ValueError(f"unsafe_archive_member:{info.filename}")
            if info.filename in names:
                raise ValueError(f"duplicate_archive_member:{info.filename}")
            if info.flag_bits & 0x1:
                raise ValueError(f"encrypted_archive_member:{info.filename}")
            names.add(info.filename)
            total += info.file_size
        if len(names) > 50 or total > 50 * 1024 * 1024:
            raise ValueError("archive_budget_exceeded")
    return {"files": len(names), "uncompressed_bytes": total, "sha256": sha256(path.read_bytes())}


def source_route(url: str) -> str:
    host = urlsplit(url).hostname or ""
    if host in PRIVATE_HOSTS:
        return "AUTHENTICATED_OR_RESTRICTED_SOURCE"
    if host in TOOL_HOSTS:
        return "GEO_TOOL_SOURCE"
    if host in OFFICIAL_EXTERNAL_HOSTS:
        return "OFFICIAL_EXTERNAL_LIVE_SOURCE"
    if host == "whut.edu.cn" or host.endswith(".whut.edu.cn"):
        return "WHUT_OFFICIAL_DOCUMENT_CANDIDATE"
    return "UNTRUSTED_EXTERNAL_REVIEW"


def question_route(row: dict) -> str:
    return {
        "rag_plus_live_refresh": "KNOWLEDGE_PLUS_LIVE_EVAL",
        "static_rag": "STATIC_KNOWLEDGE_EVAL",
        "conditional_rag": "SCOPED_POLICY_EVAL",
        "authenticated_adapter": "AUTHENTICATED_ADAPTER_EVAL",
        "geo_live": "GEO_TOOL_EVAL",
        "guarded_rag": "GUARDED_REFUSAL_EVAL",
    }.get(row["answer_mode"], "MANUAL_REVIEW")


def prepare(source_zip: Path, output: Path) -> dict:
    archive_info = validate_archive(source_zip)
    with zipfile.ZipFile(source_zip) as archive:
        gold = load_jsonl_from_zip(archive, "asku_whut_100_gold_v2.jsonl")
        source_rows = load_jsonl_from_zip(archive, "asku_whut_sources_v2.jsonl")

    if len(gold) != 100 or len({row["record_id"] for row in gold}) != 100:
        raise ValueError("gold_record_contract_failed")

    fix_log: list[dict] = []
    fixed_by_id: dict[str, dict] = {}
    for original in source_rows:
        row = dict(original)
        row["school_id"] = "whut"
        fix = SOURCE_FIXES.get(row["source_id"])
        if fix:
            before = {key: row.get(key) for key in fix if key != "fix_reason"}
            row.update({key: value for key, value in fix.items() if key != "fix_reason"})
            fix_log.append({
                "source_id": row["source_id"],
                "reason": fix["fix_reason"],
                "before": before,
                "after": {key: row.get(key) for key in fix if key != "fix_reason"},
            })
        row["url"] = canonical_url(row["url"])
        row["source_route"] = source_route(row["url"])
        fixed_by_id[row["source_id"]] = row

    grouped: dict[str, list[dict]] = defaultdict(list)
    for row in fixed_by_id.values():
        grouped[row["url"]].append(row)

    source_id_map: dict[str, str] = {}
    clean_sources: list[dict] = []
    for url, rows in sorted(grouped.items()):
        rows.sort(key=lambda row: row["source_id"])
        primary = dict(rows[0])
        aliases = [row["source_id"] for row in rows]
        primary["legacy_source_ids"] = aliases
        if url == "https://www.hbea.edu.cn/html/2026-09/16111.html":
            primary["published_at"] = "2026-09-04"
        for alias in aliases:
            source_id_map[alias] = primary["source_id"]
        clean_sources.append(primary)
        if len(rows) > 1:
            fix_log.append({
                "source_id": primary["source_id"],
                "reason": "duplicate_source_url_merged",
                "merged_source_ids": aliases,
                "url": url,
            })

    clean_by_id = {row["source_id"]: row for row in clean_sources}
    eval_rows: list[dict] = []
    route_rows: list[dict] = []
    review_rows: list[dict] = []
    for original in gold:
        row = dict(original)
        row["school_id"] = "whut"
        row["declared_confidence_score"] = row.pop("confidence_score", None)
        mapped_ids = list(dict.fromkeys(source_id_map[source_id] for source_id in row["source_ids"]))
        row["source_ids"] = mapped_ids
        row["source_titles"] = [clean_by_id[source_id]["title"] for source_id in mapped_ids]
        row["source_urls"] = [clean_by_id[source_id]["url"] for source_id in mapped_ids]
        row["routing_expectation"] = question_route(row)
        row["rag_eligible"] = False
        row["review_status"] = "EVAL_ONLY"
        row["evidence_verification_status"] = (
            "DECLARED_SUMMARY_PRESENT" if (row.get("evidence_summary") or "").strip() else "EVIDENCE_SUMMARY_MISSING"
        )
        eval_rows.append(row)
        route_rows.append({
            "record_id": row["record_id"],
            "question": row["question"],
            "aliases": row["aliases"],
            "routing_expectation": row["routing_expectation"],
            "answer_mode": row["answer_mode"],
            "requires_live_lookup": row["requires_live_lookup"],
            "personal_data_required": row["personal_data_required"],
            "college_sensitive": row["college_sensitive"],
            "expected_source_ids": mapped_ids,
        })
        if row["evidence_verification_status"] != "DECLARED_SUMMARY_PRESENT":
            review_rows.append({
                "record_id": row["record_id"],
                "reason": "missing_evidence_summary",
                "question": row["question"],
                "source_ids": mapped_ids,
            })

    frontier = []
    for source in clean_sources:
        if source["source_route"] != "WHUT_OFFICIAL_DOCUMENT_CANDIDATE":
            continue
        url = source["url"]
        suffix = Path(urlsplit(url).path).suffix.lower()
        kind = "attachment" if suffix in {".pdf", ".doc", ".docx", ".xls", ".xlsx", ".txt", ".csv"} else "page"
        transport = "http:" + url.split(":", 1)[1] if (urlsplit(url).hostname or "") in {"jwc.whut.edu.cn", "nic.whut.edu.cn"} else url
        frontier.append({
            "url": canonical_url(transport),
            "title_hint": source["title"],
            "kind": kind,
            "parent_urls": [],
            "topics": [],
            "source_id": source["source_id"],
        })

    output.mkdir(parents=True, exist_ok=False)
    write_jsonl(output / "sources.jsonl", clean_sources)
    write_jsonl(output / "eval-candidates.jsonl", eval_rows)
    write_jsonl(output / "router-evals.jsonl", route_rows)
    write_jsonl(output / "review-queue.jsonl", review_rows)
    write_jsonl(output / "source-frontier.jsonl", frontier)
    write_json(output / "source-fix-log.json", fix_log)
    summary = {
        "status": "PREPARED_FOR_SOURCE_ADMISSION",
        "school_id": "whut",
        "archive": archive_info,
        "questions": len(eval_rows),
        "unique_sources": len(clean_sources),
        "source_documents_to_fetch": len(frontier),
        "eval_only_questions": len(eval_rows),
        "missing_evidence_summaries": len(review_rows),
        "rag_eligible_qa_rows": 0,
        "rules": [
            "Q&A rows remain evaluation data and are never uploaded as official documents",
            "only independently fetched source documents may enter the admission pipeline",
            "source URLs are deduplicated after audited corrections",
            "school_id is normalized to whut",
        ],
    }
    write_json(output / "summary.json", summary)
    return summary


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-zip", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(prepare(args.source_zip.resolve(), args.output.resolve()), ensure_ascii=False, indent=2))
