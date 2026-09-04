"""One fail-closed technical gate for local statistics, export and promotion.

READY means technically eligible for a reviewed batch; it never bypasses Canary.
No detector result implicitly approves a previously held document.
"""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from typing import Any, Mapping
from urllib.parse import urlsplit

RULE_VERSION = "admission-v1"
MIN_CHARS = 100
MAX_CHARS = 100_000


def canonical_text(text: str) -> str:
    return re.sub(r"\s+", " ", text).strip()


def text_hash(text: str) -> str:
    return hashlib.sha256(canonical_text(text).encode("utf-8")).hexdigest()


def official_url(url: str, school: Mapping[str, Any]) -> bool:
    try:
        parsed = urlsplit(url)
        host = (parsed.hostname or "").lower()
        if parsed.scheme not in {"http", "https"} or parsed.username or parsed.password:
            return False
        if not any(
            host == d or host.endswith("." + d) for d in school["allowed_domains"]
        ):
            return False
        if any(
            host == d or host.endswith("." + d)
            for d in school.get("forbidden_domains", [])
        ):
            return False
        return not any(
            p.lower() in parsed.path.lower()
            for p in school.get("forbidden_path_patterns", [])
        )
    except (ValueError, TypeError):
        return False


@dataclass(frozen=True)
class Admission:
    reasons: tuple[str, ...]

    @property
    def eligible(self) -> bool:
        return not self.reasons


def evaluate(
    document: Mapping[str, Any],
    taxonomy: Mapping[str, Any],
    school: Mapping[str, Any],
    *,
    source_active: bool,
) -> Admission:
    reasons: list[str] = []
    if document.get("school_id") != school["school_id"]:
        reasons.append("school_mismatch")
    if document.get("review_status") != "ACCEPTED":
        reasons.append("review_required")
    if not source_active:
        reasons.append("source_inactive")
    if document.get("parse_status") != "PARSED":
        reasons.append("parse_not_verified")
    chars = document.get("content_chars", 0)
    if not isinstance(chars, int) or not MIN_CHARS <= chars <= MAX_CHARS:
        reasons.append("invalid_text_length")
    if not document.get("normalized_path") or not document.get("content_hash"):
        reasons.append("text_artifact_missing")
    if document.get("pii_scan_status") != "CLEAR" or document.get("pii_detected"):
        reasons.append("pii_not_cleared")
    if document.get("pii_content_hash") != document.get("content_hash"):
        reasons.append("pii_scan_stale")
    if not official_url(document.get("source_url", ""), school):
        reasons.append("untrusted_source_url")
    if not official_url(document.get("canonical_url", ""), school):
        reasons.append("untrusted_canonical_url")
    for field, key in (
        ("audience", "audiences"),
        ("education_level", "education_levels"),
        ("secondary_topic", "secondary_topics"),
        ("primary_module", "primary_modules"),
        ("document_type", "document_types"),
    ):
        if document.get(field) not in taxonomy[key]:
            reasons.append("invalid_" + field)
    topic = document.get("secondary_topic")
    if topic == "other":
        reasons.append("unclassified_topic")
    if topic in taxonomy["secondary_topics"] and taxonomy["secondary_topics"][topic][
        "primary_module"
    ] != document.get("primary_module"):
        reasons.append("topic_module_mismatch")
    if document.get("document_type") in taxonomy["non_rag_document_types"]:
        reasons.append("discovery_document")
    if document.get("canonical_document_id"):
        reasons.append("duplicate_content")
    if document.get("is_attachment") and (
        document.get("relation_status") != "RESOLVED"
        or not document.get("knowledge_bundle_id")
        or not official_url(document.get("parent_page_url", ""), school)
    ):
        reasons.append("attachment_parent_unresolved")
    return Admission(tuple(reasons))
