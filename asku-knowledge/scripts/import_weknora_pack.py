"""Idempotently upload an admission manifest to one WeKnora knowledge base."""

from __future__ import annotations

import argparse
import json
import os
import time
from datetime import UTC, datetime
from pathlib import Path

import requests


def load_jsonl(path: Path) -> list[dict]:
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]


def metadata_form(metadata: dict, manifest_row: dict) -> str:
    values = {
        **metadata,
        "external_id": manifest_row["external_id"],
        "file_sha256": manifest_row["file_sha256"],
        "clean_content_sha256": manifest_row["clean_content_sha256"],
    }
    # WeKnora metadata values are scalar strings. Preserve structured values as
    # compact JSON so callers can decode them without losing their type.
    encoded = {}
    for key, value in values.items():
        if value is None:
            encoded[key] = ""
        elif isinstance(value, bool):
            encoded[key] = str(value).lower()
        elif isinstance(value, (list, dict)):
            encoded[key] = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
        else:
            encoded[key] = str(value)
    return json.dumps(encoded, ensure_ascii=False, separators=(",", ":"))


class WeKnora:
    def __init__(self, base_url: str, api_key: str, timeout: float) -> None:
        self.base_url = base_url.rstrip("/") + "/api/v1"
        self.timeout = timeout
        self.session = requests.Session()
        self.session.headers.update({"X-API-Key": api_key})

    def _json(self, response: requests.Response) -> dict:
        try:
            payload = response.json()
        except ValueError as exc:
            raise RuntimeError(f"WeKnora returned HTTP {response.status_code} with a non-JSON body") from exc
        if not response.ok or not payload.get("success", False):
            message = payload.get("message") or payload.get("error") or response.reason
            raise RuntimeError(f"WeKnora returned HTTP {response.status_code}: {message}")
        return payload

    def list_knowledge(self, knowledge_base_id: str) -> list[dict]:
        page, result = 1, []
        while True:
            response = self.session.get(
                f"{self.base_url}/knowledge-bases/{knowledge_base_id}/knowledge",
                params={"page": page, "page_size": 100},
                timeout=self.timeout,
            )
            payload = self._json(response)
            items = payload.get("data") or []
            result.extend(items)
            if len(result) >= int(payload.get("total", len(result))) or not items:
                return result
            page += 1

    def upload(self, knowledge_base_id: str, file_path: Path, manifest_row: dict) -> dict:
        with file_path.open("rb") as handle:
            response = self.session.post(
                f"{self.base_url}/knowledge-bases/{knowledge_base_id}/knowledge/file",
                files={"file": (file_path.name, handle, "text/markdown")},
                data={
                    "fileName": file_path.name,
                    "metadata": metadata_form(manifest_row["metadata"], manifest_row),
                    "enable_multimodel": "false",
                    "process_config": json.dumps({"enable_multimodel": False}, separators=(",", ":")),
                },
                timeout=max(self.timeout, 60),
            )
        return self._json(response)["data"]

    def get_knowledge(self, knowledge_id: str) -> dict:
        response = self.session.get(f"{self.base_url}/knowledge/{knowledge_id}", timeout=self.timeout)
        return self._json(response)["data"]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    parser.add_argument("--knowledge-base-id", required=True)
    parser.add_argument("--api-key-env", default="WEKNORA_API_KEY")
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--timeout", type=float, default=15)
    parser.add_argument("--wait-seconds", type=int, default=600)
    args = parser.parse_args()

    api_key = os.environ.get(args.api_key_env, "").strip()
    if not api_key:
        raise SystemExit(f"Missing API key environment variable: {args.api_key_env}")
    manifest = load_jsonl(args.manifest)
    root = args.manifest.parent
    client = WeKnora(args.base_url, api_key, args.timeout)

    existing = client.list_knowledge(args.knowledge_base_id)
    by_external_id = {
        (item.get("metadata") or {}).get("external_id"): item
        for item in existing
        if (item.get("metadata") or {}).get("external_id")
    }
    results = []
    for row in manifest:
        external_id = row["external_id"]
        prior = by_external_id.get(external_id)
        if prior:
            prior_hash = (prior.get("metadata") or {}).get("clean_content_sha256")
            if prior_hash != row["clean_content_sha256"]:
                raise RuntimeError(f"Existing external_id has different content: {external_id}")
            results.append({"external_id": external_id, "knowledge_id": prior["id"], "action": "reused"})
            continue
        uploaded = client.upload(args.knowledge_base_id, root / row["file"], row)
        knowledge_id = uploaded.get("id")
        if not knowledge_id:
            raise RuntimeError(f"Upload response is missing knowledge id: {external_id}")
        results.append({"external_id": external_id, "knowledge_id": knowledge_id, "action": "uploaded"})

    deadline = time.monotonic() + args.wait_seconds
    pending = {row["knowledge_id"]: row for row in results}
    while pending and time.monotonic() < deadline:
        for knowledge_id, result in list(pending.items()):
            knowledge = client.get_knowledge(knowledge_id)
            result["parse_status"] = knowledge.get("parse_status")
            result["enable_status"] = knowledge.get("enable_status")
            result["error_message"] = knowledge.get("error_message") or ""
            if result["parse_status"] in {"completed", "failed"}:
                pending.pop(knowledge_id)
        if pending:
            time.sleep(2)

    for result in pending.values():
        result["parse_status"] = "timeout"
        result["enable_status"] = "unknown"
        result["error_message"] = "parse did not finish before deadline"
    failures = [row for row in results if row.get("parse_status") != "completed" or row.get("enable_status") != "enabled"]
    report = {
        "status": "PASSED" if not failures else "FAILED",
        "knowledge_base_id": args.knowledge_base_id,
        "manifest": str(args.manifest.resolve()),
        "documents": len(results),
        "uploaded": sum(row["action"] == "uploaded" for row in results),
        "reused": sum(row["action"] == "reused" for row in results),
        "completed_and_enabled": len(results) - len(failures),
        "generated_at": datetime.now(UTC).isoformat(),
        "results": results,
    }
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    print(json.dumps({key: value for key, value in report.items() if key != "results"}, ensure_ascii=False, indent=2))
    if failures:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
