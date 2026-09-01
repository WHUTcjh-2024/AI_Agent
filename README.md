# AskU Monorepo

- `apps/mobile`：React Native / Expo 移动端
- `backend`：Go API 与 AgentRun/SSE 控制层
- `contracts`：OpenAPI 与 SSE 事件契约
- `config/schools`：学校上下文配置
- `infrastructure/docker`：本地 PostgreSQL / Redis
- `docs`：架构与开发说明

当前版本为 Architecture V0.6 / Phase 7：正式 Policy Router 可在 Answer Cache、WeKnora Knowledge、Web Search 与受控回答之间选择路径。WeKnora、搜索和模型均通过独立 Adapter 接入；默认知识引擎为禁用态，不会生成假检索结果。

## 启动联调环境

```powershell
docker compose -f infrastructure/docker/docker-compose.yml up -d
```

Compose 会同时启动 Backend、PostgreSQL 和 Redis。仅 Backend 的 `18080` 对宿主机开放，数据库与缓存只在 Docker 内部网络可见。

服务入口：`http://localhost:18080/`；健康检查：`http://localhost:18080/healthz`。Android 模拟器默认通过 `10.0.2.2:18080` 访问本机后端。点击 `四六级什么时候报名？` 或输入 `官网搜索测试` 可验证检索、SSE、来源和历史记录链路。

当前架构与扩展方式见 `docs/architecture-v0.6.md`，Phase 7 实现见 `docs/phase-7-agent-orchestrator.md`；历史阶段说明保留在 `docs/phase-*.md`。
