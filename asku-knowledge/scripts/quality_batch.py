"""Build or release a local evidence-reviewed batch; never import remotely."""

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from asku.quality_batch import build, release


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ["build", "release"]:
        cmd = sub.add_parser(name)
        cmd.add_argument("--school", type=Path, required=True)
        cmd.add_argument("--taxonomy", type=Path, required=True)
        if name == "build":
            for opt in ["source", "output", "raw-root"]:
                cmd.add_argument("--" + opt, type=Path, required=True)
        else:
            cmd.add_argument("--batch", type=Path, required=True)
            cmd.add_argument("--reviews", type=Path, required=True)
            cmd.add_argument("--raw-root", type=Path, required=True)
    args = parser.parse_args()
    result = (
        build(args.source, args.output, args.school, args.taxonomy, args.raw_root)
        if args.command == "build"
        else release(
            args.batch, args.reviews, args.school, args.taxonomy, args.raw_root
        )
    )
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
