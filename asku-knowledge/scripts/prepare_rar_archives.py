"""Convert bounded RAR members into a ZIP cache without extracting member paths."""

import argparse
import hashlib
import json
import subprocess
import tempfile
import zipfile
from collections import Counter
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--sevenzip", required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    statuses = Counter()
    for path in args.input.rglob("*"):
        if not path.is_file() or path.stat().st_size > 64 * 1024 * 1024:
            continue
        with path.open("rb") as f:
            if not f.read(6).startswith(b"Rar!"):
                continue
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        target = args.output / (digest + ".zip")
        if target.exists():
            statuses["cached"] += 1
            continue
        try:
            result = subprocess.run(
                [
                    args.sevenzip,
                    "l",
                    "-slt",
                    "-ba",
                    "-sccUTF-8",
                    "-p__asku_no_password__",
                    "--",
                    str(path),
                ],
                capture_output=True,
                check=True,
                timeout=30,
            )
            records = []
            for block in result.stdout.decode("utf-8").replace("\r", "").split("\n\n"):
                record = dict(
                    line.split(" = ", 1) for line in block.splitlines() if " = " in line
                )
                if record.get("Path") and record.get("Folder") != "+":
                    records.append(record)
            if (
                not records
                or len(records) > 100
                or sum(int(r["Size"]) for r in records) > 128 * 1024 * 1024
            ):
                raise ValueError("archive_budget_exceeded")
            if any(
                int(r["Size"]) > 64 * 1024 * 1024 or r.get("Encrypted") == "+"
                for r in records
            ):
                raise ValueError("encrypted_or_oversized_member")
            with tempfile.TemporaryDirectory(prefix="asku-rar-") as temp:
                archive_path = Path(temp) / "members.zip"
                with zipfile.ZipFile(
                    archive_path, "w", zipfile.ZIP_DEFLATED
                ) as archive:
                    for r in records:
                        spool = Path(temp) / "member.bin"
                        with spool.open("wb") as stream:
                            subprocess.run(
                                [
                                    args.sevenzip,
                                    "x",
                                    "-so",
                                    "-bd",
                                    "-y",
                                    "-p__asku_no_password__",
                                    "--",
                                    str(path),
                                    r["Path"],
                                ],
                                stdout=stream,
                                stderr=subprocess.DEVNULL,
                                check=True,
                                timeout=30,
                            )
                        if spool.stat().st_size != int(r["Size"]):
                            raise ValueError("member_size_mismatch")
                        archive.writestr(
                            r["Path"].replace("\\", "/"), spool.read_bytes()
                        )
                target.write_bytes(archive_path.read_bytes())
            statuses["converted"] += 1
        except (
            ValueError,
            KeyError,
            UnicodeError,
            OSError,
            subprocess.SubprocessError,
        ) as exc:
            statuses["failed"] += 1
            (args.output / (digest + ".error.json")).write_text(
                json.dumps({"error": type(exc).__name__}), encoding="utf-8"
            )
    (args.output / "summary.json").write_text(
        json.dumps(statuses, indent=2), encoding="utf-8"
    )
    print(json.dumps(statuses), flush=True)


if __name__ == "__main__":
    main()
