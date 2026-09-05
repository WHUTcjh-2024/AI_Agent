"""Collect a bounded set of official topic pages and attachments, resuming by URL."""
import argparse
import asyncio
import json
import sys
from importlib import import_module
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

QuestionCrawler = import_module("asku.question_crawl").QuestionCrawler

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--listings', type=Path, required=True)
    parser.add_argument('--inventory', type=Path)
    args = parser.parse_args()
    asyncio.run(QuestionCrawler(args.config, args.output).run(json.loads(args.listings.read_text(encoding='utf-8')), args.inventory))
