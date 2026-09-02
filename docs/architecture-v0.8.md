# AskU Architecture V0.8

V0.8 在 V0.7 的 Agent、WeKnora、官方网页搜索和 Redis 边界之上增加 Citation Trust Chain。目标不是“显示几个链接”，而是保证回答、证据、官方来源和原文入口可追溯。

## 单向数据流

```text
Mobile UI
  -> ChatService (Mock / API Adapter)
  -> HTTP + replayable SSE
  -> Run Coordinator
  -> Agent Orchestrator
  -> Knowledge/Search ports
  -> WeKnora/Web adapters

Crawler metadata -> knowledge.* catalog -> Knowledge Gateway -> Evidence
Evidence -> Citation Builder -> Message Citation Snapshot -> Mobile Citation UI
```

## 模块责任

- `knowledge`：定义检索与 Catalog 端口；WeKnora 只提供 `knowledge_id/chunk_id/content/score`，Crawler Catalog 提供文档、部门、日期和公开 URL。
- `citation`：纯构建模块，只依赖 `domain`；校验公开 URL、证据文本、去重并分配连续编号。
- `agent`：只把真正送入生成上下文的 Evidence 交给 Citation Builder；LLM Prompt 明确禁止生成引用编号。
- `run`：把最终回答、来源和 Citation 一次性提交给 Repository，并通过 `message.completed` 发布。
- `store`：PostgreSQL Adapter 原子保存消息、来源关系和 Citation 快照；历史读取不重新计算 Citation。
- `mobile`：只消费 `message.citations`；Citation Pill 和来源卡都导航到 Source Detail，不解析回答文本。

## 多学校隔离

- WeKnora 映射主键包含 `school_id`；Document、Source 与 Mapping 查询同时校验学校。
- Source Detail 继续按当前用户 `current_school_id` 授权。
- Citation ID 来自学校内已隔离的 Source/Document/Chunk 身份；不得在 Router 或 UI 写死武汉理工配置。

## 可信边界

- 配置 Catalog 后，没有 `knowledge.weknora_mappings` 的 WeKnora Hit 会被丢弃。
- 没有学校允许域内 HTTP(S) URL 的 Evidence 不进入答案。
- 公开 API Domain 不定义 `local_file_path`；数据库解析也从不选择该字段。
- 官方附件优先于官方网页，网页优先于批准的公开备份；当前阶段不提供公开备份时不会回退到服务器路径。
- 无可靠 Evidence 时返回明确的无来源回答，`citations: []`。

## 可替换边界

Crawler、WeKnora、LLM、Web Search、Redis、PostgreSQL 和移动端 Service 均通过端口/Adapter 连接。推广到其他高校时新增学校配置与 Catalog 数据，不复制 Agent、Citation 或 UI 业务代码。
