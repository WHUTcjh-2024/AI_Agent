# AskU Architecture V0.9

V0.9 在 V0.8 Citation Trust Chain 上增加 Phase 10A Admin & Observability 只读数据面。产品运行链路不依赖 Admin；统计查询失败不会影响提问、SSE、检索或历史记录。

## 数据流

```text
Mobile -> HTTP/SSE -> Run -> Agent -> Capability Adapters
                         |
                         +-> PostgreSQL facts + immutable run_events
                                                |
Internal Admin client -> independent Admin token +-> read-only aggregation
```

## 边界

- `observability` 只定义报表 Window 与返回模型，不依赖 Agent、Store、API 或业务 Domain。
- `store` 是 PostgreSQL 聚合 Adapter；使用已有长期事实和事件，不反向调用业务服务。
- `api` 只负责 Admin 鉴权、学校范围、时间窗校验和序列化。
- `cmd/api` 仍是唯一组合根，注入 Reporter、Token 与统计时区。
- Admin API 与用户 Bearer Token 隔离；未配置时默认关闭。

## 指标事实源

- 用户/活跃：`users`、`sessions`、`messages`
- Run 状态/路由/TTFT/总耗时/缓存：`agent_runs` + `run_events`
- 质量：`message_citations`、`feedback`
- Token/成本：`usage_records`

Phase 10B 只能消费 Phase 10A 契约，不允许 Admin UI 直接访问 PostgreSQL、Redis 或 Provider。
