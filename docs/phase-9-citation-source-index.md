# Phase 9 — Citation / Source Index

## 已实现

- 标准 `Citation` 契约：文档/知识/Chunk 身份、部门、日期、来源类型、证据片段、权威度、时效和 Knowledge Bundle。
- `knowledge.sources/documents/attachments/weknora_mappings` Catalog Schema；保留内部路径但不进入公开查询和 API。
- WeKnora Retrieval ID 到 AskU Document 的严格映射；配置 Catalog 后，缺映射结果不能生成答案。
- Backend Citation Builder 负责 URL 校验、去重和编号，LLM 不生成 `[1]`。
- 消息、来源关系、Citation Snapshot 在同一事务中完成；历史消息保留当时的证据快照。
- `sources.updated` 提供过程来源，`message.completed.message.citations` 是最终事实源。
- APP 使用结构化 Citation Pill；来源详情展示部门、日期、证据、官方附件、学校原文和原通知。

## URL 策略

```text
official attachment URL
  -> official/canonical source page
  -> approved public backup (future)
```

`/data/...`、Windows Path、无 Host URL 和非 HTTP(S) URL 均无法通过 Citation Builder。

## Crawler 接入要求

Crawler 写入 `knowledge.*`，WeKnora Adapter 不负责推断原始 URL：

1. `knowledge.sources` 保存来源部门、权威度、official/canonical URL。
2. `knowledge.documents` 保存标题、发布时间、文档类型、parent page、bundle 与内部文件路径。
3. `knowledge.attachments` 保存官方附件 URL、parent page 和内部文件路径。
4. 上传 WeKnora 后写入 `knowledge.weknora_mappings`。
5. 更新资料后提升学校 `knowledge_version`，使 Query/Answer Cache 自然失效。

## 验收

- 正常 Evidence：最终消息含连续 Citation，来源详情可打开官方 URL。
- 附件 Evidence：附件为首选入口，同时保留“查看原通知”。
- 缺映射、跨校、外域或内部路径：不产生 Citation。
- 无可靠来源：`citations` 为空，不挂无关页面。
- 历史恢复：Citation 内容与生成完成时一致，不依赖当前检索结果。
