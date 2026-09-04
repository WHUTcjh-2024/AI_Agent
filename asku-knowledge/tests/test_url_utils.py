import unittest
from pathlib import Path
from unittest.mock import patch

import yaml

from asku.config import load_config
from asku.url_utils import UrlGate, canonicalize


class CanonicalizeTests(unittest.TestCase):
    def test_only_removes_scheme_default_ports(self) -> None:
        self.assertEqual(canonicalize("http://Example.com:80/a"), "http://example.com/a")
        self.assertEqual(canonicalize("https://Example.com:443/a"), "https://example.com/a")
        self.assertEqual(canonicalize("http://Example.com:443/a"), "http://example.com:443/a")
        self.assertEqual(canonicalize("https://Example.com:80/a"), "https://example.com:80/a")

    def test_url_gate_rejects_userinfo_before_network_resolution(self) -> None:
        config = load_config(school_config=Path(__file__).resolve().parents[2] / "config/schools/whut.yaml")
        gate = UrlGate(config.school, config.sources)

        decision = gate.check("https://user:secret@jwc.whut.edu.cn/notice")

        self.assertFalse(decision.allowed)
        self.assertEqual(decision.reason, "userinfo_not_allowed")

    def test_knowledge_domains_cover_backend_domains(self) -> None:
        config = load_config(school_config=Path(__file__).resolve().parents[2] / "config/schools/whut.yaml")
        repository_root = Path(__file__).resolve().parents[2]
        backend_config = yaml.safe_load(
            (repository_root / "config" / "schools" / "whut.yaml").read_text(encoding="utf-8")
        )

        self.assertTrue(set(backend_config["allowed_domains"]).issubset(config.school.allowed_domains))
        self.assertTrue(config.taxonomy.is_valid_secondary_topic("other"))

    def test_inactive_registered_source_is_not_crawled(self) -> None:
        config = load_config(school_config=Path(__file__).resolve().parents[2] / "config/schools/whut.yaml")
        gate = UrlGate(config.school, config.sources)

        with patch("asku.url_utils._is_private_address", return_value=False):
            decision = gate.check("https://english.whut.edu.cn/news/1.html")

        self.assertFalse(decision.allowed)
        self.assertEqual(decision.reason, "source_inactive")


if __name__ == "__main__":
    unittest.main()
