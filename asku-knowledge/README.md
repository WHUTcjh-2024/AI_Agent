# AskU Knowledge Pipeline

该目录负责校园官方资料的采集、标准化、治理与 WeKnora 导入。Backend 只消费通过准入门禁的 `knowledge.*` 数据，不读取本地文件路径。

## 本地验证

```powershell
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install -e .
.\.venv\Scripts\python.exe -m unittest discover -s tests -v
```

## 数据准入

文档只有同时满足以下条件才允许支撑正式答案：

1. `review_status = ACCEPTED`；
2. `rag_eligible = true`；
3. `pii_detected = false`；
4. 来源处于 `active = true`；
5. WeKnora 映射处于 `import_status = IMPORTED`；
6. `secondary_topic`、`audience`、`document_type` 等字段符合 `config/taxonomy.yaml`；
7. 至少存在一个允许域内的公开官方 URL。

`REVIEW`、`UNCERTAIN`、PII 命中、未映射或非官方 URL 默认拒绝进入回答链路。

## 迁移

```powershell
$env:ASKU_KNOWLEDGE_DSN = '<postgresql-dsn>'
.\.venv\Scripts\python.exe scripts\migrate.py --dry-run
.\.venv\Scripts\python.exe scripts\migrate.py
```

迁移必须保持纯新增。已应用迁移发生内容漂移时命令会直接失败，应创建新的迁移文件。

## 抓取安全

`Fetcher` 不会隐式信任 HTTP 重定向。调用方必须把每一跳重新交给 `UrlGate` 校验：

```python
gate = UrlGate(config.school, config.sources)
fetcher = Fetcher(
    user_agent=config.fetch["user_agent"],
    redirect_validator=lambda target: gate.check(
        target,
        max_depth=config.budgets.max_depth,
    ).allowed,
)
```

响应正文按流式大小上限读取；无 `Content-Length` 时仍会在预算处停止。

## 学校配置（V0.11）

唯一学校配置在仓库根目录 `config/schools/{school_id}.yaml`；来源表在
`asku-knowledge/config/sources/{school_id}.yaml`。原有本目录 `config/schools/whut.yaml`
和通用 `config/sources.yaml` 已移除，不做隐式回退。

从本目录运行时设置：

```powershell
$env:ASKU_SCHOOL_CONFIG = (Resolve-Path ../config/schools/whut.yaml).Path
```

`load_config()` 使用该路径；也可显式传入 `school_config=Path(...)`，或传入
`school_id` 与 `schools_dir`。环境中的路径与显式 school_id 不一致会直接失败。
Loader 在抓取前校验学校 ID、active 来源与种子域名、禁止域、知识版本。
`pipeline.yaml` 的 `weknora.enabled: true` 必须搭配学校官方知识库 ID。
数据库统计和批量领取方法必须显式传入 school_id。
