import os
import tempfile
import unittest
from dataclasses import replace
from pathlib import Path
from unittest.mock import patch

import yaml

from asku.config import CONFIG_DIR, REPO_ROOT, load_config, validate_contract
from asku.url_utils import UrlGate


class SchoolContractTests(unittest.TestCase):
    def setUp(self):
        self.school_path = REPO_ROOT / "evals/fixtures/testu.yaml"
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.config_dir = Path(self.temp.name)
        for name in ("pipeline.yaml", "taxonomy.yaml"):
            (self.config_dir / name).write_bytes((CONFIG_DIR / name).read_bytes())
        (self.config_dir / "sources").mkdir()
        self.registry_path = self.config_dir / "sources/testu.yaml"
        self.registry_path.write_bytes((Path(__file__).parent / "fixtures/sources/testu.yaml").read_bytes())

    def load(self):
        return load_config(self.config_dir, school_config=self.school_path)

    def test_single_school_config_and_source_contract(self):
        config = load_config(school_config=REPO_ROOT / "config/schools/whut.yaml")
        raw = yaml.safe_load((REPO_ROOT / "config/schools/whut.yaml").read_text(encoding="utf-8"))
        self.assertEqual(config.school.allowed_domains, raw["allowed_domains"])
        self.assertFalse((CONFIG_DIR / "schools/whut.yaml").exists())
        validate_contract(config.school, config.sources)

    def test_synthetic_school_uses_same_loader_and_url_gate(self):
        config = self.load()
        self.assertEqual(config.school_id, "testu")
        self.assertEqual(config.school.official_knowledge_base_id, "kb-testu")
        self.assertEqual(config.weknora["knowledge_base_name"], "AskU-testu-Official")
        gate = UrlGate(config.school, config.sources)
        with patch("asku.url_utils._is_private_address", return_value=False):
            self.assertTrue(gate.check("https://example.edu.cn/news"))
            for host in ("whut.edu.cn", "jwc.whut.edu.cn", "example.edu.cn.evil.org", "login.example.edu.cn", "sub.login.example.edu.cn"):
                self.assertFalse(gate.check(f"https://{host}/news"), host)

    def test_environment_selects_school_without_default_fallback(self):
        with patch.dict(os.environ, {"ASKU_SCHOOL_CONFIG": str(self.school_path)}):
            self.assertEqual(load_config(self.config_dir).school_id, "testu")
            with self.assertRaises(ValueError):
                load_config(self.config_dir, school_id="whut")
        with patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(ValueError):
                load_config(self.config_dir)
        with self.assertRaises(ValueError):
            load_config(self.config_dir, school_id="../testu")

    def test_registry_rejects_other_school(self):
        raw = yaml.safe_load(self.registry_path.read_text(encoding="utf-8"))
        raw["school_id"] = "whut"
        self.registry_path.write_text(yaml.safe_dump(raw), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "school_id"):
            self.load()

    def test_rejects_invalid_contracts(self):
        config = self.load()
        school, registry = config.school, config.sources
        source = registry.sources[0]
        cases = [
            (replace(school, knowledge_version=" "), registry),
            (replace(school, seeds=["https://whut.edu.cn/"]), registry),
            (replace(school, seeds=["https://example.edu.cn/login"]), registry),
            (replace(school, domain_suffixes=["whut.edu.cn"]), registry),
            (school, replace(registry, sources=[replace(source, domains=["whut.edu.cn"])])),
            (school, replace(registry, sources=[replace(source, base_url="https://whut.edu.cn/")])),
            (school, replace(registry, sources=[replace(source, domains=["login.example.edu.cn"])])),
        ]
        for bad_school, bad_registry in cases:
            with self.subTest(school=bad_school, registry=bad_registry), self.assertRaises(ValueError):
                validate_contract(bad_school, bad_registry)

    def test_weknora_requires_school_kb_only_when_enabled(self):
        config = self.load()
        school = replace(config.school, official_knowledge_base_id="")
        validate_contract(school, config.sources)
        with self.assertRaisesRegex(ValueError, "official_knowledge_base_id"):
            validate_contract(school, config.sources, weknora_enabled=True)
        validate_contract(config.school, config.sources, weknora_enabled=True)
