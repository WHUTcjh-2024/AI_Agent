"""Bind an explicitly authored review plan to immutable source bytes and quotes."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from asku.admission import canonical_text
from asku.quality_batch import file_text, read_db, sha, write_json


def quote(text: str, start: str, end: str) -> str:
    def pattern(value):
        return re.compile(r"\s*".join(re.escape(c) for c in re.sub(r"\s+", "", value)))

    first = pattern(start).search(text)
    if first is None:
        raise ValueError("quote_start_not_found:" + start)
    # End may start inside the first phrase, but must not precede it.
    last = pattern(end).search(text, first.start())
    if last is None:
        raise ValueError("quote_end_not_found:" + end)
    return canonical_text(text[first.start() : max(first.end(), last.end())])


def load_plan(path: Path) -> dict:
    plan = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not plan.get("extends"):
        return plan
    base = load_plan((path.parent / plan["extends"]).resolve())
    dropped = set(plan.get("drop_context_documents", []))
    return {
        **base,
        **{
            key: value
            for key, value in plan.items()
            if key not in {"extends", "reviews"}
        },
        "reviews": [*base.get("reviews", []), *plan.get("reviews", [])],
        "context_decisions": [
            item
            for item in base.get("context_decisions", [])
            if item["document_id"] not in dropped
        ],
    }


def evidence_window(text: str, needle: str, radius: int = 100) -> str:
    compact = canonical_text(text)
    match = re.search(
        r"\s*".join(re.escape(c) for c in re.sub(r"\s+", "", needle)), compact
    )
    if match is None:
        raise ValueError("evidence_needle_not_found:" + needle)
    return compact[
        max(0, match.start() - radius) : min(len(compact), match.end() + radius)
    ]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--evidence-source", type=Path)
    parser.add_argument("--plan", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    plan = load_plan(args.plan)
    if sha((args.source / "catalog.sqlite").read_bytes()) != plan["input_sha256"]:
        raise ValueError("review_plan_source_changed")
    with read_db(args.source / "catalog.sqlite") as conn:
        docs = {d["id"]: dict(d) for d in conn.execute("SELECT * FROM documents")}
    evidence_source = args.evidence_source or args.source
    with read_db(evidence_source / "catalog.sqlite") as conn:
        evidence_docs = {
            d["id"]: dict(d) for d in conn.execute("SELECT * FROM documents")
        }
    reviews = []
    cases = []
    for entry in plan["reviews"]:
        d = docs[entry["id"]]
        text = file_text(evidence_docs[d["id"]], evidence_source)
        proofs = []
        facts = entry.get("facts")
        if facts is None:
            facts = []
            for item in entry.get("evidence_needles", []):
                needle = item["needle"] if isinstance(item, dict) else str(item)
                evidence = evidence_window(text, needle)
                facts.append(
                    (
                        item.get(
                            "question",
                            f"《{entry.get('title') or d['title']}》的来源材料如何表述？",
                        )
                        if isinstance(item, dict)
                        else f"《{entry.get('title') or d['title']}》的来源材料如何表述？",
                        None,
                        None,
                        item.get("answer", evidence)
                        if isinstance(item, dict)
                        else evidence,
                        needle,
                    )
                )
        for q, start, end, answer, token in facts:
            evidence = (
                evidence_window(text, token)
                if start is None
                else quote(text, start, end)
            )
            if re.sub(r"\s+", "", str(token)) not in re.sub(r"\s+", "", evidence):
                raise ValueError("expected_token_not_in_evidence:" + q)
            proofs.append(evidence)
            cases.append(
                {
                    "id": f"whut-v4-{len(cases) + 1:03d}",
                    "question": q,
                    "document_id": d["id"],
                    "expected_topic": entry["topic"],
                    "applicable_period": entry["period"],
                    "scope_note": entry["scope"],
                    "answer_reference": answer,
                    "required_source_token": str(token),
                    "source_quote": evidence,
                    "source_url": d["source_url"],
                    "source_content_hash": d["content_hash"],
                    "reviewer_type": plan["reviewer_type"],
                }
            )
        education = entry["education"]
        review = {
            "document_id": d["id"],
            "source_content_hash": d["content_hash"],
            "source_url": d["source_url"],
            "reviewer_type": plan["reviewer_type"],
            "secondary_topic": entry["topic"],
            "education_level": education,
            "audience": "ALL" if education == "BOTH" else education,
            "document_type": entry["type"],
            "answer_scope": "FORM_ONLY"
            if entry["type"] == "FORM_TEMPLATE"
            else "DATED_SOURCE_ONLY",
            "scope_note": entry["scope"],
            "applicable_period": entry["period"],
            "evidence": proofs,
        }
        if entry.get("parent"):
            review["parent_document_id"] = entry["parent"]
        if entry.get("title"):
            review["title"] = entry["title"]
        for field in ("version_label", "version_family", "valid_from", "valid_to"):
            if entry.get(field) is not None:
                review[field] = entry[field]
        if entry.get("pii_resolution"):
            resolution = dict(entry["pii_resolution"])
            if resolution.get("needle"):
                resolution["evidence"] = evidence_window(text, resolution.pop("needle"))
            else:
                resolution["evidence"] = quote(
                    text, resolution.pop("start"), resolution.pop("end")
                )
            review["pii_resolution"] = resolution
        reviews.append(review)
    contexts = []
    for entry in plan.get("context_decisions", []):
        context = dict(entry)
        evidence_doc = docs[context["evidence_document_id"]]
        context["evidence"] = quote(
            file_text(evidence_docs[evidence_doc["id"]], evidence_source),
            context.pop("start"),
            context.pop("end"),
        )
        context["evidence_content_hash"] = evidence_doc["content_hash"]
        context["target_content_hash"] = docs[context["document_id"]]["content_hash"]
        contexts.append(context)
    args.output.mkdir(parents=True, exist_ok=True)
    write_json(
        args.output / "review-ledger.json",
        {
            "version": 1,
            "school_id": plan["school_id"],
            "input_sha256": plan["input_sha256"],
            "review_plan_sha256": sha(
                json.dumps(plan, ensure_ascii=False, sort_keys=True).encode("utf-8")
            ),
            "reviews": reviews,
            "context_decisions": contexts,
        },
    )
    write_json(
        args.output / "evidence-cases.json",
        {
            "version": 1,
            "evaluation_kind": "SOURCE_EVIDENCE_ACCEPTANCE_NOT_PRODUCTION_RAG",
            "cases": cases,
        },
    )
    print(
        f"Bound {len(reviews)} reviewed documents and {len(cases)} source-evidence cases."
    )


if __name__ == "__main__":
    main()
