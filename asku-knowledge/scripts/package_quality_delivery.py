"""Package only verified, scoped evidence; quarantine never enters the ZIP."""

from __future__ import annotations

import argparse
import json
import sys
import zipfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from asku.quality_batch import file_text, sha


def package(batch: Path, report: Path, output: Path) -> dict:
    batch, report, output = batch.resolve(), report.resolve(), output.resolve()
    contract = json.loads(
        (batch / "delivery-contract.json").read_text(encoding="utf-8")
    )
    acceptance = json.loads((report / "acceptance.json").read_text(encoding="utf-8"))
    validation = json.loads((batch / "validation.json").read_text(encoding="utf-8"))
    if (
        contract.get("status") != "CANARY_READY"
        or acceptance.get("status") != "PASSED"
        or validation.get("status") != "PASSED"
    ):
        raise ValueError("delivery_not_accepted")
    expected = {
        batch / "catalog.sqlite": contract["catalog_sha256"],
        batch / "reviewed_evidence.jsonl": contract["evidence_sha256"],
        report / "acceptance.json": contract["acceptance_sha256"],
        report / "evidence-cases.json": contract["evidence_cases_sha256"],
        batch / "review-ledger.json": validation["review_ledger_sha256"],
        **{
            batch / name: digest for name, digest in validation["export_sha256"].items()
        },
    }
    for path, digest in expected.items():
        if sha(path.read_bytes()) != digest:
            raise ValueError("verified_artifact_changed:" + path.name)

    entries = {}
    for name in (
        "reviewed_evidence.jsonl",
        "review-ledger.json",
        "validation.json",
        "delivery-contract.json",
        "ready_relations.jsonl",
        "ready_bundles.jsonl",
        "context_decisions.jsonl",
    ):
        entries[name] = (batch / name).read_bytes()
    for name in ("acceptance.json", "evidence-cases.json"):
        entries[name] = (report / name).read_bytes()
    root = Path(__file__).resolve().parents[2]
    entries["README.md"] = (root / "docs/knowledge-quality-v4.md").read_bytes()
    entries["scope_gate.py"] = (
        root / "asku-knowledge/asku/scoped_evidence.py"
    ).read_bytes()

    documents = []
    for line in (
        (batch / "ready_documents.jsonl").read_text(encoding="utf-8").splitlines()
    ):
        document = json.loads(line)
        if (
            document["admission_status"] != "READY"
            or document["semantic_status"] != "EVIDENCE_REVIEWED"
        ):
            raise ValueError("unreviewed_document_in_export")
        file_text(document, batch)
        # Filenames come from validated local paths rather than arbitrary IDs.
        path = Path(document["normalized_path"])
        relative = "documents/" + path.name
        if relative in entries:
            raise ValueError("duplicate_package_path:" + relative)
        entries[relative] = path.read_bytes()
        document.pop("raw_path", None)
        document["normalized_path"] = relative
        document["local_file_path"] = relative
        documents.append(document)
    entries["documents.jsonl"] = "".join(
        json.dumps(d, ensure_ascii=False) + "\n" for d in documents
    ).encode("utf-8")
    manifest = {
        "status": "OFFLINE_SCOPED_EVIDENCE_ONLY",
        "documents": len(documents),
        "evidence_units": contract["source_units"],
        "production_ready": False,
        "quarantine_included": False,
        "upstream_catalog_sha256": contract["catalog_sha256"],
        "original_document_export_sha256": validation["export_sha256"][
            "ready_documents.jsonl"
        ],
        "files": {name: sha(data) for name, data in sorted(entries.items())},
    }
    entries["package-manifest.json"] = json.dumps(
        manifest, ensure_ascii=False, indent=2
    ).encode("utf-8")
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_suffix(output.suffix + ".tmp")
    with zipfile.ZipFile(temporary, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for name, data in sorted(entries.items()):
            archive.writestr(name, data)
    with zipfile.ZipFile(temporary) as archive:
        if archive.testzip() is not None:
            raise ValueError("zip_integrity_failed")
        for name, digest in manifest["files"].items():
            if sha(archive.read(name)) != digest:
                raise ValueError("zip_hash_mismatch:" + name)
    temporary.replace(output)
    return {"path": str(output), "sha256": sha(output.read_bytes()), **manifest}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("batch", "report", "output"):
        parser.add_argument("--" + name, type=Path, required=True)
    args = parser.parse_args()
    print(
        json.dumps(
            package(args.batch, args.report, args.output), ensure_ascii=False, indent=2
        )
    )
