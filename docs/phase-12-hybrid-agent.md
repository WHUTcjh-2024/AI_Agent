# Phase 12 — Hybrid Knowledge + Web Agent

## 范围

本阶段实现开发方案要求的“稳定知识走 KB，时效问题 KB+Web”。这里的 Hybrid 是产品能力编排，不是自研 BM25、向量检索、RRF 或 Rerank。

```text
PolicyRouter
  ├─ stable -> knowledge
  └─ fresh  -> hybrid
                 ├─ knowledge.Searcher
                 └─ websearch.Searcher
```

## 执行规则

1. Router 根据“最新、今年、当前、什么时候、几点、校历”等时效标记生成 `hybrid` 计划。
2. Retrieval Coordinator 并行调用 Knowledge 与 Web 端口，并在一个 `retrieval.started/completed` 生命周期内合并结果。
3. 官网 Evidence 是日期、当前安排和有效状态的唯一时效依据；知识 Evidence 只补充稳定背景。
4. Knowledge 未配置或失败时 fail-open 到官方 Web，并记录 `degradedCapabilities=["knowledge"]`。
5. Web 调用失败时返回可重试错误；Web 零结果时返回无可靠来源，清除知识来源与 Citation，避免旧资料冒充最新结果。
6. Citation 由 Backend 从两类真实 Evidence 统一编号；LLM 不生成编号或链接。
7. Hybrid 不读取、不写入稳定 Answer Cache，防止时效问题命中过期答案。

## 联调矩阵

- 稳定问题 + Knowledge 命中：仅 Knowledge，生成并缓存有 Citation 的答案。
- 时效问题 + Knowledge/Web 均命中：混合 Grounding，返回两类来源和 Citation。
- 时效问题 + Knowledge disabled/失败 + Web 命中：Web 降级成功，事件可观测。
- 时效问题 + Web 失败：Run 失败，不生成答案。
- 时效问题 + Web 零结果：受控拒答，Sources/Citations 为空。
- `官网搜索测试`：保留纯 Web 路由，便于单能力联调。

## 数据接入边界

WorkBuddy 数据通过 Catalog 准入和 WeKnora Mapping 后才进入 Knowledge Gateway。数据尚未通过 Canary 时保持 `ASKU_KNOWLEDGE_PROVIDER=disabled`；Phase 12 代码无需等待数据，也不放宽 `rag_eligible/review_status/pii_detected/import_status/source.active` 的运行时门禁。
