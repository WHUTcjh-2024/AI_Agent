# AskU Architecture V0.10

V0.10 在 V0.9 的 Citation、Admin 与可观测性基础上实现 Phase 12 产品级混合 Agent。它组合两个已有能力端口，不实现向量级 Hybrid Retrieval、RRF、Rerank 或多工具 ReAct；这些通用能力继续由 WeKnora 负责。

## 数据流

```text
Question
  -> PolicyRouter
      -> controlled
      -> knowledge -----------------------> Knowledge Searcher
      -> web_search ----------------------> Web Searcher
      -> hybrid -> Retrieval Coordinator -+-> Knowledge Searcher (stable background)
                                         +-> Web Searcher (freshness authority)
  -> Grounded Generation
  -> backend-owned Citations
  -> Run / SSE / PostgreSQL
```

## 解耦边界

- `PolicyRouter` 只产出能力计划，不读取学校配置、不调用 Provider。
- `retrievalCoordinator` 只组合 `knowledge.Searcher` 与 `websearch.Searcher` 端口；并行、降级和来源合并规则集中在 Agent 内部。
- Knowledge 与 Web Gateway 继续负责学校范围、官方域名、安全抓取、缓存及供应商协议。
- Run、API、SSE、移动端不区分单检索与混合检索，只消费稳定事件和最终 Source/Citation。
- `cmd/api` 仍是唯一组合根。

## 时效安全规则

- 稳定问题：`knowledge`，允许读取和写入按 `knowledge_version` 隔离的 Answer Cache。
- 时效问题：`hybrid`，并行查知识库背景与学校官网最新资料，不读写稳定答案缓存。
- 知识能力未配置或调用失败：只要官网检索成功，混合路由可退化为 Web Grounding，并通过事件元数据暴露降级。
- 官网调用失败：Run 失败；官网零 Evidence：明确返回无可靠来源。两种情况都不得把知识库资料包装成当前安排。

Phase 11 的数据导入和 Canary 可与本阶段并行。启用 WeKnora 后只替换 Knowledge Adapter 的运行配置，不修改 Router、Orchestrator、Run 或 APP。
