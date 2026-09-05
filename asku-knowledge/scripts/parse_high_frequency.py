import argparse
import sys
from importlib import import_module
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

build_candidates = import_module("asku.question_parse").build_candidates

if __name__ == '__main__':
    p = argparse.ArgumentParser()
    for name in ['crawl', 'config', 'inventory', 'output']:
        p.add_argument('--'+name, type=Path, required=True)
    p.add_argument('--rendered', type=Path)
    args = p.parse_args()
    build_candidates(**vars(args))
