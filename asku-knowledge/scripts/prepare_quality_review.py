"""Bind an explicitly authored review plan to immutable source bytes and quotes."""

from __future__ import annotations

import argparse
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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--plan", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    plan = yaml.safe_load(args.plan.read_text(encoding="utf-8"))
    if sha((args.source / "catalog.sqlite").read_bytes()) != plan["input_sha256"]:
        raise ValueError("review_plan_source_changed")
    with read_db(args.source / "catalog.sqlite") as conn:
        docs = {d["id"]: dict(d) for d in conn.execute("SELECT * FROM documents")}
    reviews = []
    cases = []
    for entry in plan["reviews"]:
        d = docs[entry["id"]]
        text = file_text(d, args.source)
        proofs = []
        for q, start, end, answer, token in entry["facts"]:
            evidence = quote(text, start, end)
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
        if entry.get("pii_resolution"):
            resolution = dict(entry["pii_resolution"])
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
            file_text(evidence_doc, args.source),
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
            "review_plan_sha256": sha(args.plan.read_bytes()),
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
