"""配置加载：SchoolConfig / SourceRegistry / Taxonomy / Pipeline 预算。

§0 反硬编码：学校相关的任何字面量只允许出现在 config/ 下的 YAML 里。
本模块只负责把这些 YAML 读成强类型对象，代码其它部分不得内联学校信息。

约束 #4：所有抓取预算（max_urls_total / max_urls_per_source / max_depth /
max_runtime / max_disk_usage / max_file_size）集中在此解析，触顶即停。
"""

from __future__ import annotations

import os
import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional

import yaml

MODULE_ROOT = Path(__file__).resolve().parent.parent
CONFIG_DIR = MODULE_ROOT / "config"
DATA_DIR = MODULE_ROOT / "data"
REPORTS_DIR = MODULE_ROOT / "reports"
EXPORTS_DIR = MODULE_ROOT / "exports"
EVAL_DIR = MODULE_ROOT / "eval"
MIGRATIONS_DIR = MODULE_ROOT / "database" / "migrations"

# 与 AskU 主仓库的 backend 配置的相对位置，用于契约一致性测试
REPO_ROOT = MODULE_ROOT.parent
BACKEND_SCHOOL_CONFIG = REPO_ROOT / "config" / "schools"


# ---------------------------------------------------------------------------
# 预算解析
# ---------------------------------------------------------------------------

_DURATION_RE = re.compile(r"^\s*(\d+(?:\.\d+)?)\s*(s|m|h|d)?\s*$", re.IGNORECASE)
_SIZE_RE = re.compile(r"^\s*(\d+(?:\.\d+)?)\s*(b|kb|mb|gb|tb)?\s*$", re.IGNORECASE)

_DURATION_UNIT = {"s": 1.0, "m": 60.0, "h": 3600.0, "d": 86400.0}
_SIZE_UNIT = {"b": 1, "kb": 1024, "mb": 1024**2, "gb": 1024**3, "tb": 1024**4}


def parse_duration(value: int | float | str) -> float:
    """把 '8h' / '30m' / 45 / '45s' 解析成秒。"""
    if isinstance(value, (int, float)):
        return float(value)
    match = _DURATION_RE.match(str(value))
    if not match:
        raise ValueError(f"无法解析时间预算: {value!r}")
    number, unit = match.groups()
    return float(number) * _DURATION_UNIT[(unit or "s").lower()]


def parse_size(value: int | float | str) -> int:
    """把 '20GB' / '50MB' / 1048576 解析成字节数。"""
    if isinstance(value, (int, float)):
        return int(value)
    match = _SIZE_RE.match(str(value))
    if not match:
        raise ValueError(f"无法解析容量预算: {value!r}")
    number, unit = match.groups()
    return int(float(number) * _SIZE_UNIT[(unit or "b").lower()])


@dataclass(frozen=True)
class Budgets:
    """约束 #4 的六项硬性预算。任意一项触顶都必须 checkpoint 后正常退出。"""

    max_urls_total: int
    max_urls_per_source: int
    max_depth: int
    max_runtime_seconds: float
    max_disk_usage_bytes: int
    max_file_size_bytes: int

    @classmethod
    def from_dict(cls, raw: Dict[str, Any]) -> "Budgets":
        return cls(
            max_urls_total=int(raw["max_urls_total"]),
            max_urls_per_source=int(raw["max_urls_per_source"]),
            max_depth=int(raw["max_depth"]),
            max_runtime_seconds=parse_duration(raw["max_runtime"]),
            max_disk_usage_bytes=parse_size(raw["max_disk_usage"]),
            max_file_size_bytes=parse_size(raw["max_file_size"]),
        )


# ---------------------------------------------------------------------------
# SchoolConfig
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class SchoolConfig:
    school_id: str
    school_name: str
    short_name: str
    domain_suffixes: List[str]
    allowed_domains: List[str]
    forbidden_domains: List[str]
    forbidden_path_patterns: List[str]
    official_knowledge_base_id: str
    community_knowledge_base_id: str
    knowledge_version: str
    education_levels: List[str]
    academic_calendar: Dict[str, int]
    seeds: List[str]

    def is_official_domain(self, host: str) -> bool:
        """官方域名判定：显式登记 或 命中官方后缀。"""
        host = (host or "").lower().strip().rstrip(".")
        if not host:
            return False
        if host in self.forbidden_domains:
            return False
        if host in self.allowed_domains:
            return True
        return any(host == s or host.endswith("." + s) for s in self.domain_suffixes)

    def is_forbidden(self, url: str) -> bool:
        """登录页 / 验证码 / 后台管理等禁止访问的路径（约束 #3）。"""
        lowered = (url or "").lower()
        return any(pattern.lower() in lowered for pattern in self.forbidden_path_patterns)


@dataclass(frozen=True)
class SourceEntry:
    source_key: str
    name: str
    department: str
    base_url: str
    domains: List[str]
    authority_type: str
    priority: str
    education_level: str
    active: bool
    name_confirmed: bool
    discovered_from: str


@dataclass(frozen=True)
class SourceRegistry:
    school_id: str
    sources: List[SourceEntry]

    def active_sources(self) -> List[SourceEntry]:
        return [s for s in self.sources if s.active]

    def by_key(self, key: str) -> Optional[SourceEntry]:
        for source in self.sources:
            if source.source_key == key:
                return source
        return None

    def host_to_source(self) -> Dict[str, SourceEntry]:
        mapping: Dict[str, SourceEntry] = {}
        for source in self.sources:
            for domain in source.domains:
                mapping[domain.lower()] = source
        return mapping


@dataclass(frozen=True)
class Taxonomy:
    education_levels: List[str]
    primary_modules: List[str]
    secondary_topics: Dict[str, Dict[str, Any]]
    document_types: List[str]
    non_rag_document_types: List[str]
    key_document_types: List[str]
    audiences: List[str]
    topic_keywords: Dict[str, List[str]]
    document_type_keywords: Dict[str, List[str]]
    low_value_keywords: List[str]
    student_value_keywords: List[str]
    weknora_tags: Dict[str, List[str]]

    def is_valid_secondary_topic(self, topic: str) -> bool:
        return topic in self.secondary_topics

    def is_valid_document_type(self, doc_type: str) -> bool:
        return doc_type in self.document_types


# ---------------------------------------------------------------------------
# Pipeline 配置总入口
# ---------------------------------------------------------------------------


def _load_yaml(path: Path) -> Dict[str, Any]:
    with path.open("r", encoding="utf-8") as handle:
        return yaml.safe_load(handle) or {}


@dataclass
class PipelineConfig:
    school: SchoolConfig
    sources: SourceRegistry
    taxonomy: Taxonomy
    budgets: Budgets
    fetch: Dict[str, Any]
    robots: Dict[str, Any]
    attachments: Dict[str, Any]
    classification: Dict[str, Any]
    quality: Dict[str, Any]
    reporting: Dict[str, Any]
    coverage: Dict[str, Any]
    pii: Dict[str, Any]
    weknora: Dict[str, Any]

    # ---- 便捷访问 ----
    @property
    def school_id(self) -> str:
        return self.school.school_id

    @property
    def data_root(self) -> Path:
        return DATA_DIR / self.school_id

    @property
    def raw_root(self) -> Path:
        return self.data_root / "raw"

    @property
    def normalized_root(self) -> Path:
        return self.data_root / "normalized"

    @property
    def staging_root(self) -> Path:
        return self.data_root / "staging"

    @property
    def weknora_api_key(self) -> str:
        return os.environ.get(self.weknora.get("api_key_env", "WEKNORA_API_KEY"), "")


def load_config(
    config_dir: Path = CONFIG_DIR,
    school_id: Optional[str] = None,
) -> PipelineConfig:
    pipeline_raw = _load_yaml(config_dir / "pipeline.yaml")
    taxonomy_raw = _load_yaml(config_dir / "taxonomy.yaml")
    sources_raw = _load_yaml(config_dir / "sources.yaml")

    school_id = school_id or sources_raw.get("school_id") or "whut"
    school_raw = _load_yaml(config_dir / "schools" / f"{school_id}.yaml")

    school = SchoolConfig(
        school_id=school_raw["school_id"],
        school_name=school_raw["school_name"],
        short_name=school_raw.get("short_name", ""),
        domain_suffixes=[d.lower() for d in school_raw.get("domain_suffixes", [])],
        allowed_domains=[d.lower() for d in school_raw.get("allowed_domains", [])],
        forbidden_domains=[d.lower() for d in school_raw.get("forbidden_domains", [])],
        forbidden_path_patterns=list(school_raw.get("forbidden_path_patterns", [])),
        official_knowledge_base_id=school_raw.get("official_knowledge_base_id", ""),
        community_knowledge_base_id=school_raw.get("community_knowledge_base_id", ""),
        knowledge_version=school_raw.get("knowledge_version", "v1"),
        education_levels=list(school_raw.get("education_levels", [])),
        academic_calendar=dict(school_raw.get("academic_calendar", {})),
        seeds=list(school_raw.get("seeds", [])),
    )

    sources = SourceRegistry(
        school_id=sources_raw.get("school_id", school_id),
        sources=[
            SourceEntry(
                source_key=str(entry["source_key"]),
                name=entry.get("name", ""),
                department=entry.get("department", ""),
                base_url=entry["base_url"],
                domains=[d.lower() for d in entry.get("domains", [])],
                authority_type=entry.get("authority_type", "OFFICIAL_DEPARTMENT"),
                priority=entry.get("priority", "P2"),
                education_level=entry.get("education_level", "UNKNOWN"),
                active=bool(entry.get("active", False)),
                name_confirmed=bool(entry.get("name_confirmed", False)),
                discovered_from=entry.get("discovered_from", ""),
            )
            for entry in sources_raw.get("sources", [])
        ],
    )

    taxonomy = Taxonomy(
        education_levels=list(taxonomy_raw.get("education_levels", [])),
        primary_modules=list(taxonomy_raw.get("primary_modules", [])),
        secondary_topics=dict(taxonomy_raw.get("secondary_topics", {}) or {}),
        document_types=list(taxonomy_raw.get("document_types", [])),
        non_rag_document_types=list(taxonomy_raw.get("non_rag_document_types", [])),
        key_document_types=list(taxonomy_raw.get("key_document_types", [])),
        audiences=list(taxonomy_raw.get("audiences", [])),
        topic_keywords=dict(taxonomy_raw.get("topic_keywords", {}) or {}),
        document_type_keywords=dict(taxonomy_raw.get("document_type_keywords", {}) or {}),
        low_value_keywords=list(taxonomy_raw.get("low_value_keywords", [])),
        student_value_keywords=list(taxonomy_raw.get("student_value_keywords", [])),
        weknora_tags=dict(taxonomy_raw.get("weknora_tags", {}) or {}),
    )

    return PipelineConfig(
        school=school,
        sources=sources,
        taxonomy=taxonomy,
        budgets=Budgets.from_dict(pipeline_raw["budgets"]),
        fetch=dict(pipeline_raw.get("fetch", {})),
        robots=dict(pipeline_raw.get("robots", {})),
        attachments=dict(pipeline_raw.get("attachments", {})),
        classification=dict(pipeline_raw.get("classification", {})),
        quality=dict(pipeline_raw.get("quality", {})),
        reporting=dict(pipeline_raw.get("reporting", {})),
        coverage=dict(pipeline_raw.get("coverage", {})),
        pii=dict(pipeline_raw.get("pii", {})),
        weknora=dict(pipeline_raw.get("weknora", {})),
    )
