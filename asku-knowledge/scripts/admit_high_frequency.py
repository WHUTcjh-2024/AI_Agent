"""Build strict question-first reports and a WeKnora import package."""

from __future__ import annotations

import argparse
import json
import sys
from importlib import import_module
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

build = import_module("asku.question_admit").build


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--candidates", type=Path, action="append", required=True)
    parser.add_argument("--config", type=Path, required=True)
    parser.add_argument("--batch", type=Path, required=True)
    parser.add_argument("--reports", type=Path, required=True)
    parser.add_argument("--export", type=Path, required=True)
    parser.add_argument("--evals", type=Path, required=True)
    result = build(**vars(parser.parse_args()))
    print(json.dumps(result, ensure_ascii=False, indent=2))
