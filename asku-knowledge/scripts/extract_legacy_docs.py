"""Run inside an isolated offline container with antiword and LibreOffice.

Input mount must be read-only. Outputs are keyed by original file SHA256;
no filename, document text or personal information is logged.
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import os
import subprocess
import tempfile
import zipfile
from collections import Counter
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path


def candidates(path: Path):
    data = path.read_bytes()
    if data.startswith(bytes.fromhex("d0cf11e0a1b11ae1")):
        yield data
    elif data.startswith(b"PK"):
        try:
            with zipfile.ZipFile(io.BytesIO(data)) as archive:
                members = archive.infolist()
                if (
                    len(members) > 100
                    or sum(m.file_size for m in members) > 128 * 1024 * 1024
                ):
                    return
                for member in members:
                    if not member.is_dir() and member.file_size <= 64 * 1024 * 1024:
                        child = archive.read(member)
                        if child.startswith(bytes.fromhex("d0cf11e0a1b11ae1")):
                            yield child
        except (zipfile.BadZipFile, RuntimeError):
            pass


def convert(data: bytes, output: Path) -> str:
    digest = hashlib.sha256(data).hexdigest()
    target = output / (digest + ".txt")
    if target.exists():
        return "cached"
    # A cheap OLE directory-name prefilter; antiword still validates the format.
    if "WordDocument".encode("utf-16le") not in data:
        return "not_word"
    with tempfile.TemporaryDirectory(prefix="asku-doc-") as temp:
        path = Path(temp) / "input.doc"
        path.write_bytes(data)
        try:
            result = subprocess.run(
                ["antiword", "-m", "UTF-8.txt", str(path)],
                capture_output=True,
                timeout=30,
            )
            text = (
                result.stdout.decode("utf-8", errors="strict")
                if result.returncode == 0
                else ""
            )
            if not text.strip():
                # Each converter uses an isolated profile, avoiding the live service profile.
                subprocess.run(
                    [
                        "soffice",
                        "-env:UserInstallation=" + (Path(temp) / "profile").as_uri(),
                        "--headless",
                        "--convert-to",
                        "txt:Text (encoded):UTF8",
                        "--outdir",
                        temp,
                        str(path),
                    ],
                    capture_output=True,
                    timeout=45,
                    check=True,
                )
                converted = Path(temp) / "input.txt"
                text = (
                    converted.read_text(encoding="utf-8-sig")
                    if converted.exists()
                    else ""
                )
            if not text.strip():
                return "empty"
            temp_path = output / (digest + ".tmp")
            temp_path.write_text(text, encoding="utf-8")
            os.replace(temp_path, target)
            return "converted"
        except (subprocess.SubprocessError, UnicodeError, OSError):
            return "failed"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    unique = {}
    for path in args.input.rglob("*"):
        if path.is_file() and path.stat().st_size <= 64 * 1024 * 1024:
            for data in candidates(path):
                unique[hashlib.sha256(data).hexdigest()] = data
    counts = Counter()
    with ThreadPoolExecutor(max_workers=3) as pool:
        for status in pool.map(
            lambda data: convert(data, args.output), unique.values()
        ):
            counts[status] += 1
    (args.output / "summary.json").write_text(
        json.dumps(counts, indent=2), encoding="utf-8"
    )
    print(json.dumps(counts), flush=True)


if __name__ == "__main__":
    main()
