import unittest

from asku.normalizer import _format_date, normalize_html


class NormalizerTests(unittest.TestCase):
    def test_invalid_calendar_date_is_not_emitted(self) -> None:
        self.assertEqual(_format_date("2026", "02", "31"), "")

    def test_attachment_url_hints_support_configured_regexes(self) -> None:
        page = normalize_html(
            '<html><body><main><h1>通知</h1><p>正文内容足够用于测试。</p>'
            '<a href="download.jsp?id=1">附件下载</a></main></body></html>',
            base_url="https://jwc.whut.edu.cn/notices/1.html",
            attachment_url_hints=[r"\.jsp\?"],
        )

        self.assertEqual(len(page.attachment_links), 1)
        self.assertEqual(page.attachment_links[0]["url"], "https://jwc.whut.edu.cn/notices/download.jsp?id=1")


if __name__ == "__main__":
    unittest.main()
