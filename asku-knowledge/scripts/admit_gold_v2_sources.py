"""Admit the reviewed Gold V2 source documents into an incremental WeKnora pack.

The supplied Q&A records remain evaluation data. Only independently fetched
official source text can be emitted as RAG documents.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import shutil
import zipfile
from collections import Counter, defaultdict
from datetime import UTC, datetime
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit

import yaml

BLOCKING_FLAGS = {
    "parse_failure",
    "no_article_body",
    "unsupported_or_image",
    "unicode_corruption",
    "roster_not_parsed",
    "pdf_ocr_required",
    "pdf_table_layout_review",
    "image_evidence_required",
    "source_scope_unknown",
    "outside_undergraduate_p0",
    "special_audience_requires_review",
    "roster_table",
}

# Every release decision below was checked against the fetched body. Excerpts
# are verified again at build time so an upstream page change cannot silently
# inherit an old approval.
ADMISSION_LEDGER = {
    "SRC002": {
        "topic": "freshman",
        "document_type": "guide",
        "intents_full": ["registration_date", "welcome_required_tasks", "freshman_course_entry"],
        "intents_partial": [],
        "excerpts": [
            "8月30日/31日本科新生网上报到",
            "9月11日前本科新生完成网上报到",
            "9月12日本科新生来校报到",
        ],
    },
    "SRC003": {
        "topic": "freshman",
        "document_type": "guide",
        "intents_full": [
            "online_registration", "registration_materials", "required_documents",
            "late_registration_leave", "welcome_volunteer", "student_archive",
            "league_relation_transfer",
        ],
        "intents_partial": [],
        "excerpts": [
            "请携带录取通知书、高考准考证、身份证等材料备查",
            "因故确不能按期到校者，须向学校招生办公室请假",
            "学校以学院为单位为每位新生安排有学生志愿者",
            "自带档案的新生报到后将档案材料直接交给所在学院",
            "团员须登录“理工青年智能管理系统”录入政治面貌",
        ],
    },
    "SRC005": {
        "topic": "freshman",
        "document_type": "guide",
        "intents_full": ["hukou_transfer"],
        "intents_partial": [],
        "excerpts": ["户口迁移遵循自愿原则", "新生将材料交至录取学院辅导员"],
    },
    "SRC006": {
        "topic": "freshman",
        "document_type": "guide",
        "intents_full": ["department_routing"],
        "intents_partial": [],
        "excerpts": ["遇到任何问题可向辅导员老师寻求帮助", "学生工作部", "本科生院", "财务处"],
    },
    "SRC007": {
        "topic": "freshman",
        "document_type": "guide",
        "intents_full": ["freshman_course_entry", "freshman_course_credit"],
        "intents_partial": ["ligong_zhike"],
        "excerpts": [
            "学校利用“理工智课”平台开设《新生入学教育》网上课程",
            "获得1个课外学分",
            "新生须在9月11日前完成课程学习和考试",
        ],
    },
    "SRC016": {
        "topic": "freshman",
        "document_type": "regulation",
        "intents_full": ["late_registration_leave", "campus_count", "transfer_policy_slogan", "scholarship_types"],
        "intents_partial": [],
        "excerpts": [
            "现有马房山校区、余家头校区和南湖校区",
            "因故不能按时报到者，应提前向我校招生办公室申请办理请假手续",
            "实行“转出无限制，转入有门槛”的灵活的转专业政策",
            "奖学金、助学金、国家助学贷款、勤工助学、困难补助、减免学费",
        ],
    },
    "SRC018": {
        "topic": "transfer_major",
        "document_type": "regulation",
        "intents_full": ["major_division_rule"],
        "intents_partial": [],
        "excerpts": ["按照志愿优先原则", "按第一志愿学生成绩高低排序", "学院范围内公示3个工作日"],
    },
    "SRC019": {
        "topic": "campus_network",
        "document_type": "guide",
        "intents_full": ["ssid_difference"],
        "intents_partial": ["dorm_network"],
        "excerpts": ["教学办公区无线网络名称：WHUT-WLAN", "学生宿舍区无线网络名称：WHUT-DORM"],
    },
    "SRC020": {
        "topic": "campus_network",
        "document_type": "guide",
        "intents_full": ["password_reset"],
        "intents_partial": [],
        "excerpts": ["密码重置必须核实用户身份", "委托修改智慧理工大平台密码申请"],
    },
    "SRC028": {
        "topic": "innovation_project",
        "document_type": "regulation",
        "intents_full": [],
        "intents_partial": ["freshman_competition_dachuang"],
        "excerpts": [
            "本办法适用于我校在籍在读普通全日制本科生",
            "毕业年级学生原则上不再申报",
            "学生申报、导师审核、学院审定提交均在系统中操作",
        ],
    },
    "SRC030": {
        "topic": "program_plan",
        "document_type": "guide",
        "intents_full": ["minor_dual_micro"],
        "intents_partial": [],
        "excerpts": ["第三学期，学生可以申请辅修本校的双学位以及微专业"],
    },
    "SRC032": {
        "topic": "extracurricular_credit",
        "document_type": "regulation",
        "intents_full": ["extracurricular_credits"],
        "intents_partial": ["competition_bonus", "volunteer_student_work_bonus"],
        "excerpts": [
            "本科就读期间至少要取得10个课外学分",
            "参加各类学术科技竞赛",
            "参加社会实践、各类青年志愿者服务",
            "学生干部由团委或其主管单位提供任职证明",
        ],
    },
    "SRC034": {
        "topic": "campus_network",
        "document_type": "guide",
        "intents_full": ["offcampus_database_access"],
        "intents_partial": [],
        "excerpts": ["在校外更安全、更便捷的访问校内业务系统及图书馆数据库资源", "https://webvpn.whut.edu.cn/"],
    },
}


def load_jsonl(path: Path) -> list[dict]:
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")


def write_jsonl(path: Path, rows: list[dict]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows), encoding="utf-8", newline="\n")


def sha256(data: bytes | str) -> str:
    if isinstance(data, str):
        data = data.encode("utf-8")
    return hashlib.sha256(data).hexdigest()


def compact(text: str) -> str:
    return re.sub(r"\s+", "", text or "")


def match_key(url: str) -> str:
    """Match HTTP transport fallbacks to their declared HTTPS source."""
    parsed = urlsplit(url)
    path = re.sub(r"/{2,}", "/", parsed.path or "/")
    return urlunsplit(("", parsed.netloc.lower(), path, parsed.query, ""))


def safe_reset(path: Path) -> None:
    resolved = path.resolve()
    if resolved == Path(resolved.anchor) or len(resolved.parts) < 4:
        raise ValueError(f"refusing_to_reset_unsafe_path:{resolved}")
    if path.exists():
        shutil.rmtree(path)
    path.mkdir(parents=True)


def current_inventory(manifest: Path) -> tuple[set[str], set[str], set[str]]:
    rows = load_jsonl(manifest)
    return (
        {row["external_id"] for row in rows},
        {match_key(row["metadata"]["source_url"]) for row in rows},
        {row["clean_content_sha256"] for row in rows},
    )


def build(args: argparse.Namespace) -> dict:
    sources = load_jsonl(args.prepared / "sources.jsonl")
    questions = load_jsonl(args.prepared / "eval-candidates.jsonl")
    candidates = load_jsonl(args.candidates)
    taxonomy = yaml.safe_load(args.taxonomy.read_text(encoding="utf-8"))
    topics = taxonomy["secondary_topics"]
    current_ids, current_urls, current_hashes = current_inventory(args.current_manifest)

    source_by_key = {match_key(row["url"]): row for row in sources}
    candidate_by_source: dict[str, list[dict]] = defaultdict(list)
    for candidate in candidates:
        source = source_by_key.get(match_key(candidate.get("canonical_url") or candidate["official_url"]))
        if source:
            candidate_by_source[source["source_id"]].append(candidate)

    accepted: list[dict] = []
    review: list[dict] = []
    validation_failures: list[str] = []

    for source in sources:
        sid = source["source_id"]
        ledger = ADMISSION_LEDGER.get(sid)
        rows = candidate_by_source.get(sid, [])
        reasons: list[str] = []
        candidate = next((row for row in rows if match_key(row["canonical_url"]) == match_key(source["url"])), None)
        if source.get("source_route") != "WHUT_OFFICIAL_DOCUMENT_CANDIDATE":
            reasons.append("route_not_static_whut_document")
        if not ledger:
            reasons.append("no_completed_evidence_review")
        if not candidate:
            reasons.append("source_body_not_parsed")
        if candidate:
            reasons.extend("quality:" + flag for flag in sorted(set(candidate.get("quality_flags", [])) & BLOCKING_FLAGS))
            if candidate.get("contains_pii"):
                reasons.append("pii_detected")
            if len(compact(candidate.get("clean_content", ""))) < 120:
                reasons.append("content_too_short")
            if candidate["asku_document_id"] in current_ids or match_key(candidate["canonical_url"]) in current_urls:
                reasons.append("already_in_current_knowledge_base")
            if candidate.get("content_hash") in current_hashes:
                reasons.append("duplicate_current_clean_content")
        if ledger and ledger["topic"] not in topics:
            reasons.append("topic_not_in_taxonomy")
        if ledger and candidate:
            body = compact(candidate["clean_content"])
            for excerpt in ledger["excerpts"]:
                if compact(excerpt) not in body:
                    reasons.append("evidence_excerpt_missing:" + excerpt[:24])
            declared_intents = {
                row["intent"] for row in questions if sid in row.get("source_ids", [])
            }
            unknown = (set(ledger["intents_full"]) | set(ledger["intents_partial"])) - declared_intents
            if unknown:
                reasons.append("intent_not_declared_for_source:" + ",".join(sorted(unknown)))

        if reasons:
            review.append({
                "source_id": sid,
                "title": source["title"],
                "url": source["url"],
                "candidate_ids": [row["asku_document_id"] for row in rows],
                "quality_flags": sorted({flag for row in rows for flag in row.get("quality_flags", [])}),
                "reasons": sorted(set(reasons)),
            })
            continue

        assert candidate is not None and ledger is not None
        topic_cfg = topics[ledger["topic"]]
        published_at = candidate.get("published_at")
        accepted.append({
            **candidate,
            "source_id": sid,
            "declared_source_url": source["url"],
            "source_org": source["org"],
            "authority_grade": source["authority_grade"],
            "topic": ledger["topic"],
            "primary_module": topic_cfg["primary_module"],
            "document_type": ledger["document_type"],
            "audience": "FRESHMAN" if ledger["topic"] == "freshman" else "UNDERGRADUATE",
            "question_tags": ledger["intents_full"] + ledger["intents_partial"],
            "evidence_intents_full": ledger["intents_full"],
            "evidence_intents_partial": ledger["intents_partial"],
            "evidence_excerpts": ledger["excerpts"],
            "version_status": "latest_observed",
            "answer_scope": "TIMELESS_POLICY" if ledger["document_type"] == "regulation" else "DATED_SOURCE_ONLY",
            "published_at": published_at,
            "last_verified_at": source["last_verified_at"],
            "review_status": "ACCEPTED",
            "rag_eligible": True,
        })

    # The curated list itself must remain duplicate free.
    for label, values in {
        "external_id": [row["asku_document_id"] for row in accepted],
        "canonical_url": [match_key(row["canonical_url"]) for row in accepted],
        "clean_content": [row["content_hash"] for row in accepted],
    }.items():
        duplicates = sorted(key for key, count in Counter(values).items() if count > 1)
        validation_failures.extend(f"duplicate_{label}:{key}" for key in duplicates)

    accepted.sort(key=lambda row: (row["topic"], row["title"], row["asku_document_id"]))
    review.sort(key=lambda row: row["source_id"])
    safe_reset(args.output)
    documents = args.output / "documents"
    documents.mkdir()
    manifest: list[dict] = []
    for row in accepted:
        body = f"# {row['title']}\n\n{row['clean_content'].strip()}\n\n---\n来源：{row['declared_source_url']}\n"
        path = documents / f"{row['asku_document_id']}.md"
        path.write_text(body, encoding="utf-8", newline="\n")
        metadata = {
            "title": row["title"],
            "school_id": "whut",
            "source_id": row["source_id"],
            "source_url": row["declared_source_url"],
            "fetched_url": row["canonical_url"],
            "source_org": row["source_org"],
            "authority_grade": row["authority_grade"],
            "published_at": row["published_at"],
            "last_verified_at": row["last_verified_at"],
            "primary_module": row["primary_module"],
            "topic": row["topic"],
            "question_tags": row["question_tags"],
            "evidence_intents_full": row["evidence_intents_full"],
            "evidence_intents_partial": row["evidence_intents_partial"],
            "education_level": row["education_level"],
            "audience": row["audience"],
            "document_type": row["document_type"],
            "version_status": row["version_status"],
            "answer_scope": row["answer_scope"],
            "knowledge_bundle_id": row["knowledge_bundle_id"],
            "review_status": "ACCEPTED",
            "rag_eligible": True,
            "contains_pii": False,
            "raw_sha256": row["raw_sha256"],
        }
        manifest.append({
            "external_id": row["asku_document_id"],
            "file": "documents/" + path.name,
            "file_sha256": sha256(path.read_bytes()),
            "clean_content_sha256": row["content_hash"],
            "metadata": metadata,
        })

    write_jsonl(args.output / "import-manifest.jsonl", manifest)
    write_jsonl(args.output / "metadata.jsonl", [{"external_id": row["external_id"], **row["metadata"]} for row in manifest])
    write_jsonl(args.output / "canary-manifest.jsonl", manifest)
    write_jsonl(args.output / "evidence-review-ledger.jsonl", [{
        "source_id": row["source_id"],
        "external_id": row["asku_document_id"],
        "source_content_sha256": row["content_hash"],
        "full_support_intents": row["evidence_intents_full"],
        "partial_guarded_intents": row["evidence_intents_partial"],
        "verified_excerpts": row["evidence_excerpts"],
        "reviewed_at": datetime.now(UTC).isoformat(),
    } for row in accepted])

    admitted_source_ids = {row["source_id"] for row in accepted}
    source_support: dict[str, dict[str, list[str]]] = defaultdict(lambda: {"full": [], "partial": []})
    for row in accepted:
        for intent in row["evidence_intents_full"]:
            source_support[intent]["full"].append(row["source_id"])
        for intent in row["evidence_intents_partial"]:
            source_support[intent]["partial"].append(row["source_id"])

    eval_rows = []
    for question in questions:
        support = source_support[question["intent"]]
        status = "VERIFIED_EVAL" if support["full"] and question.get("evidence_summary") else "REVIEW_REQUIRED"
        eval_rows.append({
            **question,
            "admitted_source_ids": sorted(admitted_source_ids & set(question.get("source_ids", []))),
            "full_support_source_ids": sorted(support["full"]),
            "partial_guarded_source_ids": sorted(support["partial"]),
            "evidence_verification_status": status,
            "rag_eligible": False,
            "review_status": "EVAL_ONLY",
        })
    write_jsonl(args.output / "evaluations" / "verified.jsonl", [row for row in eval_rows if row["evidence_verification_status"] == "VERIFIED_EVAL"])
    write_jsonl(args.output / "evaluations" / "review.jsonl", [row for row in eval_rows if row["evidence_verification_status"] != "VERIFIED_EVAL"])
    shutil.copy2(args.prepared / "router-evals.jsonl", args.output / "evaluations" / "router-evals.jsonl")
    audit = args.output / "audit"
    audit.mkdir()
    for name in ("sources.jsonl", "source-fix-log.json", "summary.json"):
        shutil.copy2(args.prepared / name, audit / name)
    write_jsonl(args.output / "review-queue.jsonl", review)

    required_metadata = {
        "school_id", "source_id", "source_url", "primary_module", "topic", "question_tags",
        "education_level", "audience", "document_type", "version_status", "answer_scope",
        "review_status", "rag_eligible", "contains_pii",
    }
    for row in manifest:
        path = args.output / row["file"]
        if not path.is_file() or sha256(path.read_bytes()) != row["file_sha256"]:
            validation_failures.append("file_hash_failed:" + row["external_id"])
        missing = required_metadata - row["metadata"].keys()
        if missing:
            validation_failures.append("missing_metadata:" + row["external_id"] + ":" + ",".join(sorted(missing)))
        if "\ufffd" in path.read_text(encoding="utf-8"):
            validation_failures.append("unicode_replacement_character:" + row["external_id"])

    summary = {
        "status": "WEKNORA_INCREMENTAL_IMPORT_READY" if not validation_failures else "FAILED",
        "school_id": "whut",
        "source_archive_sha256": json.loads((args.prepared / "summary.json").read_text(encoding="utf-8"))["archive"]["sha256"],
        "source_registry_records": len(sources),
        "question_records": len(questions),
        "current_kb_documents": len(current_ids),
        "accepted_new_documents": len(manifest),
        "projected_kb_documents_after_import": len(current_ids) + len(manifest),
        "review_or_excluded_sources": len(review),
        "verified_eval_rows": sum(row["evidence_verification_status"] == "VERIFIED_EVAL" for row in eval_rows),
        "review_eval_rows": sum(row["evidence_verification_status"] != "VERIFIED_EVAL" for row in eval_rows),
        "qa_rows_released_as_documents": 0,
        "pii_documents_released": 0,
        "blocking_quality_flags_released": sum(bool(set(row["quality_flags"]) & BLOCKING_FLAGS) for row in accepted),
        "duplicates_against_current_kb_released": 0,
        "generated_at": datetime.now(UTC).isoformat(),
        "imported": False,
    }
    validation = {
        "status": "PASSED" if not validation_failures else "FAILED",
        "documents_checked": len(manifest),
        "unique_external_ids": len({row["external_id"] for row in manifest}),
        "unique_clean_content_hashes": len({row["clean_content_sha256"] for row in manifest}),
        "files_hash_verified": len(manifest) - sum(reason.startswith("file_hash_failed:") for reason in validation_failures),
        "failures": validation_failures,
    }
    write_json(args.output / "export.json", summary)
    write_json(args.report / "data-cleaning-report.json", {**summary, "review_reason_counts": dict(sorted(Counter(reason for row in review for reason in row["reasons"]).items()))})
    write_json(args.report / "import-validation.json", validation)
    report_md = [
        "# Gold V2 数据清洗报告", "",
        f"- 源记录：{len(sources)}",
        f"- 新增准入官方文档：{len(manifest)}",
        f"- 复核或排除来源：{len(review)}",
        f"- 已核验评测题：{summary['verified_eval_rows']}",
        f"- 仍需复核评测题：{summary['review_eval_rows']}",
        "- 问答直接进库：0",
        "- PII 文档进库：0",
        "- 阻断质量标记文档进库：0",
        "- 与当前知识库重复文档进库：0",
        f"- 导入校验：{validation['status']}", "",
        "`documents/` 与 `import-manifest.jsonl` 是增量导入包；`evaluations/` 只用于评测和路由。",
    ]
    args.report.mkdir(parents=True, exist_ok=True)
    (args.report / "data-cleaning-report.md").write_text("\n".join(report_md) + "\n", encoding="utf-8", newline="\n")
    (args.output / "README.md").write_text(
        "# AskU WHUT Gold V2 清洗结果\n\n"
        "本目录是相对当前知识库去重后的 WeKnora 增量导入包。上传 `documents/` 中的 Markdown，"
        "并按 `import-manifest.jsonl` 保留元数据。问答记录位于 `evaluations/`，仅用于评测和路由，禁止作为官方文档上传。\n",
        encoding="utf-8", newline="\n",
    )
    sum_lines = []
    for path in sorted(p for p in args.output.rglob("*") if p.is_file() and p.name != "SHA256SUMS"):
        sum_lines.append(f"{sha256(path.read_bytes())}  {path.relative_to(args.output).as_posix()}")
    (args.output / "SHA256SUMS").write_text("\n".join(sum_lines) + "\n", encoding="ascii", newline="\n")

    if validation_failures:
        raise ValueError("validation_failed:" + ";".join(validation_failures))
    return {"summary": summary, "validation": validation}


def package(cleaned_dir: Path, report_dir: Path, destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(destination, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
        for root, prefix in ((cleaned_dir, "source-import"), (report_dir, "reports")):
            for path in sorted(p for p in root.rglob("*") if p.is_file()):
                archive.write(path, f"{prefix}/{path.relative_to(root).as_posix()}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prepared", type=Path, required=True)
    parser.add_argument("--candidates", type=Path, required=True)
    parser.add_argument("--taxonomy", type=Path, required=True)
    parser.add_argument("--current-manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--zip", type=Path, required=True)
    args = parser.parse_args()
    result = build(args)
    package(args.output, args.report, args.zip)
    result["zip"] = {"path": str(args.zip.resolve()), "sha256": sha256(args.zip.read_bytes()), "bytes": args.zip.stat().st_size}
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
