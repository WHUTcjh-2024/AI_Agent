# AskU Phase 7：Agent Orchestrator

## 完成范围

Phase 7 把原来的联调 `MockRouter` 升级为正式产品路由，并保持 Run、HTTP、SSE 和移动端不感知具体供应商：

```text
Question
  → AnswerCache port（Phase 8 已接版本化 Redis Cache）
  → PolicyRouter
      ├─ controlled：产品说明等确定性回答
      ├─ knowledge：稳定校园信息 → KnowledgeEngine
      └─ web_search：最新、今年、时间等时效问题 → WebSearchService
  → Grounded LLM Generation
  → persisted SSE events
```

不在本阶段实现复杂 ReAct、自研 RAG、Redis Answer Cache 写入或知识数据导入。

## 解耦边界

- `agent.PolicyRouter`：只决定能力路径，不读取学校配置、不调用供应商。
- `knowledge.Searcher`：Agent 依赖的稳定知识检索端口。
- `knowledge.Gateway`：把 `school_id` 映射为该校官方知识库 ID，并生成学校隔离的 Source ID。
- `knowledge.WeKnoraProvider`：只负责 WeKnora HTTP 协议、认证和响应映射。
- `agent.AnswerCache`：由 Phase 8 的版本化缓存实现；缓存故障必须 fail-open。
- `cmd/api/main.go`：唯一知道具体 Adapter 的组合根。

Knowledge Gateway 会统一丢弃缺少知识 ID/正文的供应商结果，并仅保留当前学校官方域名白名单内的原文链接；这组规则不依赖 WeKnora，替换检索供应商后仍然有效。

新增高校时只增加 SchoolContext 和对应知识库数据，不复制 Router、Agent、API 或 APP 代码。

## WeKnora 官方契约

AskU 使用 WeKnora 官方知识搜索端点：

- Base URL：`/api/v1`
- Endpoint：`POST /api/v1/knowledge-search`
- Auth：`X-API-Key`
- Scope：`knowledge_base_ids`
- Result：chunk content、knowledge id/title、score、metadata

官方参考：

- https://github.com/Tencent/WeKnora
- https://github.com/Tencent/WeKnora/blob/main/docs/api/README.md
- https://github.com/Tencent/WeKnora/blob/main/docs/api/knowledge-search.md

API Key 只从环境注入。学校 YAML 只保存知识库 ID，不保存密钥。AskU 导入数据时应在 WeKnora metadata 中写入 `source_url`、`publisher`、`published_at`，以支持官方原文入口和发布时间展示。

## 安全默认值

默认配置：

```text
ASKU_AGENT_MODE=policy
ASKU_KNOWLEDGE_PROVIDER=disabled
```

在 WeKnora 环境、Scoped API Key 或学校知识库 ID 未准备好时，Knowledge 路径返回 `configured=false` 和空 Evidence。Orchestrator 随后输出“暂时没有找到可靠的学校官方信息”，不会调用 LLM 猜测政策。

## 联调场景

- `奖学金怎么评？` → `knowledge`；默认禁用态返回无可靠来源。
- `今年四六级什么时候报名？` → `web_search`；本地 Mock Search 验证来源、Grounded Generation 和 SSE。
- `你能做什么？` → `controlled`；不调用外部能力。
- AnswerCache 单元测试命中 → `cache`；跳过 Router、Knowledge、Search 和 LLM。

Knowledge、Web Search 和 Cache 路径的完整事件顺序：

```text
run.started
route.resolved
retrieval.started
retrieval.completed
sources.updated
generation.started
message.delta
message.completed
run.completed
```

`controlled` 路径不会伪造检索状态，直接从 `route.resolved` 进入 `generation.started`。

## Phase 8 接入结果

`agent.VersionedAnswerCache` 已在组合根注入。缓存 Key 包含 `school_id`、知识版本和规范化问题；只缓存带官方来源的 Knowledge 答案。Run、API、SSE 和移动端无需修改。
