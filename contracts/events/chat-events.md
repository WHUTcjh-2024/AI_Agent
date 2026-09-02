# AskU Chat SSE Contract V1

每条事件使用标准 SSE：`id` 为服务端递增序号，`event` 为事件名，`data` 为 JSON。

事件顺序：

1. `run.started`
2. `route.resolved`
3. `retrieval.started`
4. `retrieval.completed`
5. `sources.updated`
6. `generation.started`
7. `message.delta`（0..N）
8. `message.completed`
9. `run.completed` 或 `run.failed`

客户端只展示产品状态，不显示模型思维过程。重连时传 `Last-Event-ID` 或 `?after=<id>`；客户端必须按 `id` 去重。

- Answer Cache 命中时 `route.resolved.route=cache`，检索引擎为 `answer-cache`，并携带 `cacheHit=true`。
- Knowledge Query Cache 命中状态位于 `retrieval.completed.knowledgeStats.queryCacheHit`。
- `controlled` 路径不产生检索事件，直接进入 `generation.started`。

## Citation 约束

- `sources.updated` 可先让客户端展示检索进度与来源卡；它不是最终引用事实。
- `message.completed.payload.message.citations` 是 Citation Source of Truth。
- Citation 的 `index`、文档映射和公开 URL 均由 Backend 根据真实 Retrieval Evidence 生成；客户端不得从正文正则提取 `[1]`，LLM 也不得生成引用编号。
- 无可验证 Evidence 时，`citations` 必须是空数组。内部 `local_file_path` 禁止进入任何 SSE/API Payload。
