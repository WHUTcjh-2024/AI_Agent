"""Scope enforcement for offline canaries. Production adapters must call this gate."""

from __future__ import annotations

import json
import re
from collections.abc import Mapping
from typing import Any


def allowed_for_scope(
    document: Mapping[str, Any],
    *,
    school_id: str,
    topic: str,
    applicable_period: str,
    needs_current_policy: bool = False,
) -> bool:
    """Missing scope or a request for current policy fails closed for offline data."""
    if needs_current_policy or not applicable_period or not topic or not school_id:
        return False
    if any(
        not isinstance(document.get(key), str)
        or re.fullmatch(r"[0-9a-f]{64}", document[key]) is None
        for key in ("content_hash", "source_content_hash", "pii_content_hash")
    ):
        return False
    if (
        document.get("school_id") != school_id
        or document.get("secondary_topic") != topic
        or document.get("admission_status") != "READY"
        or document.get("semantic_status") != "EVIDENCE_REVIEWED"
        or document.get("review_status") != "ACCEPTED"
        or document.get("rag_eligible") != 1
        or document.get("pii_detected")
        or document.get("pii_scan_status") != "CLEAR"
        or document.get("pii_content_hash") != document.get("content_hash")
    ):
        return False
    try:
        review = json.loads(document.get("review_evidence") or "{}")
    except (ValueError, TypeError):
        return False
    if not isinstance(review, dict):
        return False
    return (
        review.get("applicable_period") == applicable_period
        and review.get("source_content_hash") == document.get("source_content_hash")
        and document.get("answer_scope")
        in {"DATED_SOURCE_ONLY", "VERSION_SPECIFIC_ONLY", "FORM_ONLY"}
    )
