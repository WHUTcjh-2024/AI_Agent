"""Finalize mirror exports and verify every local text hash and admission decision."""

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from asku.batch_validation import finalize_and_verify


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--batch", type=Path, required=True)
    parser.add_argument("--school", type=Path, required=True)
    parser.add_argument("--taxonomy", type=Path, required=True)
    args = parser.parse_args()
    print(
        json.dumps(
            finalize_and_verify(args.batch, args.school, args.taxonomy),
            ensure_ascii=False,
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
