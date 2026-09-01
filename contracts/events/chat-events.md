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
