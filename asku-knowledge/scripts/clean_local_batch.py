"""Create a new offline staging batch. Never overwrite the input or import remotely."""

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from asku.local_cleaning import run_batch


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in (
        "source-db",
        "crawler-dump",
        "school",
        "taxonomy",
        "raw-root",
        "output",
        "legacy-dir",
    ):
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--tesseract", default=None)
    parser.add_argument("--archive-dir", type=Path, default=None)
    parser.add_argument("--workers", type=int, default=4, choices=range(1, 9))
    args = parser.parse_args()
    run_batch(
        source_db=args.source_db,
        crawler_dump=args.crawler_dump,
        school_path=args.school,
        taxonomy_path=args.taxonomy,
        raw_root=args.raw_root,
        output=args.output,
        legacy_dir=args.legacy_dir,
        tesseract=args.tesseract,
        workers=args.workers,
        archive_dir=args.archive_dir,
    )


if __name__ == "__main__":
    main()
