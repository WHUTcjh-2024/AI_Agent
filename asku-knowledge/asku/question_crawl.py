"""Bounded question-first discovery; immutable raw artifacts, no database writes."""
from __future__ import annotations

import asyncio
import hashlib
import json
import re
import time
from collections import Counter
from datetime import UTC, datetime
from pathlib import Path
from urllib.parse import parse_qsl, urlencode, urljoin, urlsplit, urlunsplit

import yaml
from bs4 import BeautifulSoup

from .fetcher import Fetcher

EXTENSIONS = {'.pdf', '.doc', '.docx', '.xls', '.xlsx', '.zip', '.txt', '.csv'}
IMAGE_EXTENSIONS = {'.png', '.jpg', '.jpeg', '.gif'}
BLOCK = re.compile(r'/login|/signin|/sso|/cas/|/auth/|/admin/|/manage/|/tpass/|captcha', re.IGNORECASE)
NEWS = re.compile(r'召开|讲座|风采|事迹|先进工作|教职工|教师招聘|教学竞赛|教学成果|导师|博士|硕士|研究生|普通话|夏令营|国际学生|留学生')


def digest(value: bytes | str) -> str:
    return hashlib.sha256(value.encode('utf-8') if isinstance(value, str) else value).hexdigest()


def canonical(url: str) -> str:
    p = urlsplit(url)
    # Preserve identity-bearing query parameters, only remove known trackers.
    query = urlencode(sorted((k, v) for k, v in parse_qsl(p.query, keep_blank_values=True)
                             if not k.lower().startswith('utm_') and k.lower() not in {'from', 'spm'}))
    return urlunsplit((p.scheme.lower(), p.netloc.lower(), p.path or '/', query, ''))


def allowed(url: str) -> bool:
    p = urlsplit(url)
    host = p.hostname or ''
    try:
        port = p.port
    except ValueError:
        return False
    return (p.scheme in {'http', 'https'} and not p.username and port in {None, 80, 443}
            and (host == 'whut.edu.cn' or host.endswith('.whut.edu.cn'))
            and not host.startswith(('mail.', 'vpn.', 'webvpn.', 'sso.', 'jwxt.', 'jwxk.', 'zhlgd.'))
            and not BLOCK.search(p.path))


def transport_url(url: str) -> str:
    # These public CMS sites expose HTTP and have nonworking TLS locally.
    if urlsplit(url).hostname in {'jwc.whut.edu.cn', 'nic.whut.edu.cn'}:
        return 'http:' + url.split(':', 1)[1]
    return url


def read_jsonl(path: Path) -> list[dict]:
    return [json.loads(line) for line in path.read_text(encoding='utf-8').splitlines() if line.strip()] if path.exists() else []


class QuestionCrawler:
    def __init__(self, config: Path, output: Path):
        self.config = yaml.safe_load(config.read_text(encoding='utf-8'))
        self.output = output
        output.mkdir(parents=True, exist_ok=True)
        (output / 'raw').mkdir(exist_ok=True)
        self.ledger_path = output / 'fetch-ledger.jsonl'
        self.ledger = {r['requested_url']: r for r in read_jsonl(self.ledger_path)}
        self.discovered = {r['url']: r for r in read_jsonl(output / 'frontier.jsonl')}
        self.patterns = {t['id']: re.compile(t['title_pattern'], re.IGNORECASE) for t in self.config['topics']}
        self.started = time.monotonic()
        self.counts = Counter()

    def topics(self, title: str) -> list[str]:
        return [k for k, p in self.patterns.items() if p.search(title)]

    def add(self, url: str, title: str = '', parent: str | None = None, kind: str = 'page'):
        url = canonical(transport_url(url))
        if not allowed(url):
            return
        row = self.discovered.setdefault(url, {'url': url, 'title_hint': title, 'kind': kind,
                                               'parent_urls': [], 'topics': self.topics(title)})
        if parent and parent not in row['parent_urls']:
            row['parent_urls'].append(parent)
        if title and not row['title_hint']:
            row['title_hint'] = title

    def checkpoint(self):
        (self.output / 'frontier.jsonl').write_text(''.join(json.dumps(r, ensure_ascii=False)+'\n' for r in self.discovered.values()), encoding='utf-8')

    async def fetch(self, fetcher: Fetcher, url: str) -> dict:
        url = canonical(transport_url(url))
        old = self.ledger.get(url)
        if old:
            return old
        if time.monotonic()-self.started > self.config['budgets']['max_runtime_seconds']:
            return {'ok': False, 'error': 'runtime_budget'}
        result = await fetcher.fetch(url)
        row = {'requested_url': url, 'url': result.final_url or url, 'ok': result.ok,
               'http_status': result.http_status, 'mime_type': result.content_type,
               'encoding': result.encoding, 'error': result.error_type,
               'crawl_at': datetime.now(UTC).isoformat(),
               'sha256': digest(result.content) if result.content else None,
               'content_disposition': result.content_disposition}
        if result.ok:
            ext = Path(urlsplit(row['url']).path).suffix.lower()
            if ext not in EXTENSIONS | IMAGE_EXTENSIONS:
                ext = '.html'
            path = self.output / 'raw' / (row['sha256'] + ext)
            if not path.exists():
                path.write_bytes(result.content)
            row['raw_file'] = 'raw/' + path.name
        self.ledger[url] = row
        with self.ledger_path.open('a', encoding='utf-8') as f:
            f.write(json.dumps(row, ensure_ascii=False)+'\n')
        self.counts['fetched'] += 1
        if self.counts['fetched'] % 20 == 0:
            print(json.dumps(dict(self.counts), ensure_ascii=False), flush=True)
        return row

    def soup(self, row: dict):
        if not row.get('raw_file'):
            return None
        data = (self.output / row['raw_file']).read_bytes()
        from .fetcher import decode_bytes
        return BeautifulSoup(decode_bytes(data, row.get('mime_type', ''))[0], 'lxml')

    def inspect_links(self, row: dict, *, listing: bool):
        soup = self.soup(row)
        if soup is None:
            return
        base = row['url']
        main = soup.select_one('.TRS_Editor, .TRS_UEDITOR, #vsb_content, #vsb_content_2, .v_news_content, #bfArticleContent')
        # CMS download lists commonly sit next to, not inside, the article body.
        if not listing:
            for anchor in soup.select('a[href]'):
                href = urljoin(base, anchor['href'])
                if Path(urlsplit(href).path).suffix.lower() in EXTENSIONS:
                    self.add(href, anchor.get_text(' ', strip=True), base, 'attachment')
        for a in (soup if listing or main is None else main).select('a[href]'):
            url = urljoin(base, a['href'])
            title = a.get_text(' ', strip=True)
            ext = Path(urlsplit(url).path).suffix.lower()
            if ext in EXTENSIONS:
                self.add(url, title, None if listing else base, 'attachment')
            elif self.topics(title) and not NEWS.search(title):
                if re.search(r'20(?:0\d|1\d|20|21)年', title) and '管理' not in title:
                    continue
                self.add(url, title, None, 'page')
        if not listing and main:
            for img in main.select('img[src]'):
                src = urljoin(base, img['src'])
                if not re.search(r'icon|qrcode|share|weixin|logo', src, re.IGNORECASE):
                    self.add(src, img.get('alt') or '正文图片（需图像证据核验）', base, 'image')

    async def run(self, listings: list[str], inventory: Path | None = None):
        if inventory:
            for d in read_jsonl(inventory):
                if d.get('is_attachment') or NEWS.search(d.get('title', '')):
                    continue
                title = d.get('title', '')
                if (
                    self.topics(title)
                    and re.search(r'办法|规定|细则|指南|说明|通知|校历|转专业|综测|借阅|续借', title)
                    and (not d.get('publish_date') or str(d['publish_date']) >= '2022' or re.search(r'办法|规定|细则|指南', title))
                ):
                    self.add(d['source_url'], title)
        b = self.config['budgets']
        async with Fetcher(user_agent='AskU-Knowledge-Pipeline/1.0', max_retries=1, timeout_seconds=18,
                           concurrency_per_host=2, concurrency_total=8, min_interval_per_host=0.8,
                           max_file_size_bytes=b['max_file_size_mb']*1024*1024,
                           redirect_validator=allowed) as fetcher:
            for start in range(0, min(len(listings), b['max_listing_pages']), 8):
                rows = await asyncio.gather(*(self.fetch(fetcher, u) for u in listings[start:start+8]))
                for row in rows:
                    if row.get('ok'):
                        self.inspect_links(row, listing=True)
                self.checkpoint()
            pages = [r for r in self.discovered.values() if r['kind']=='page']
            # Recently published notices first; titles for timeless rules second.
            pages.sort(key=lambda r: ('2026' in r['title_hint'], '2025' in r['title_hint'], r['url']), reverse=True)
            for start in range(0, min(len(pages), b['max_pages_total']), 8):
                rows = await asyncio.gather(*(self.fetch(fetcher, r['url']) for r in pages[start:start+8]))
                for row in rows:
                    if row.get('ok'):
                        self.inspect_links(row, listing=False)
                self.checkpoint()
            artifacts = [r for r in self.discovered.values() if r['kind'] in {'attachment', 'image'}]
            for start in range(0, min(len(artifacts), b['max_attachments_total']), 8):
                await asyncio.gather(*(self.fetch(fetcher, r['url']) for r in artifacts[start:start+8]))
            self.checkpoint()
        print(json.dumps({'done': dict(self.counts), 'frontier': Counter(r['kind'] for r in self.discovered.values()),
                          'success': sum(r['ok'] for r in self.ledger.values())}, ensure_ascii=False), flush=True)
