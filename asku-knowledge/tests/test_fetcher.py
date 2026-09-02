import unittest

import httpx

from asku.fetcher import Fetcher, RobotsPolicy


def make_fetcher(handler, **overrides) -> Fetcher:
    options = {
        "user_agent": "AskU-Test",
        "robots": RobotsPolicy(enabled=False),
        "max_retries": 1,
        "min_interval_per_host": 0,
        "jitter_seconds": 0,
        "transport": httpx.MockTransport(handler),
    }
    options.update(overrides)
    return Fetcher(**options)


class FetcherTests(unittest.IsolatedAsyncioTestCase):
    async def test_redirect_is_rejected_before_requesting_unapproved_target(self) -> None:
        requested = []

        def handler(request: httpx.Request) -> httpx.Response:
            requested.append(str(request.url))
            return httpx.Response(302, headers={"location": "https://evil.example/private"})

        async with make_fetcher(handler, redirect_validator=lambda target: False) as fetcher:
            result = await fetcher.fetch("https://jwc.whut.edu.cn/start")

        self.assertFalse(result.ok)
        self.assertEqual(result.error_type, "redirect_disallowed")
        self.assertEqual(requested, ["https://jwc.whut.edu.cn/start"])

    async def test_approved_redirect_is_followed(self) -> None:
        requested = []

        def handler(request: httpx.Request) -> httpx.Response:
            requested.append(str(request.url))
            if request.url.path == "/start":
                return httpx.Response(302, headers={"location": "/final"})
            return httpx.Response(200, content="官方通知".encode("utf-8"), headers={"content-type": "text/html; charset=utf-8"})

        validator = lambda target: httpx.URL(target).host == "jwc.whut.edu.cn"
        async with make_fetcher(handler, redirect_validator=validator) as fetcher:
            result = await fetcher.fetch("https://jwc.whut.edu.cn/start")

        self.assertTrue(result.ok)
        self.assertEqual(result.final_url, "https://jwc.whut.edu.cn/final")
        self.assertEqual(len(requested), 2)

    async def test_streaming_body_stops_at_size_limit_without_content_length(self) -> None:
        def handler(request: httpx.Request) -> httpx.Response:
            return httpx.Response(200, content=b"0123456789")

        async with make_fetcher(handler) as fetcher:
            result = await fetcher.fetch("https://jwc.whut.edu.cn/file", max_bytes=5)

        self.assertFalse(result.ok)
        self.assertEqual(result.error_type, "file_too_large")
        self.assertEqual(fetcher.stats["bytes"], 0)


if __name__ == "__main__":
    unittest.main()
