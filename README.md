# AskU Monorepo

- `apps/mobile`：React Native / Expo 移动端
- `backend`：Go API 与 AgentRun/SSE 控制层
- `contracts`：OpenAPI 与 SSE 事件契约
- `config/schools`：学校上下文配置
- `infrastructure/docker`：本地 PostgreSQL / Redis
- `docs`：架构与开发说明

当前版本为 Architecture V0.5：保留既有账号、持久化、SSE、LLM Gateway 与 Web Search Gateway，并将移动端、运行生命周期、Agent 编排和基础设施整理为可替换端口/适配器。默认搜索与模型仍为明确标注的 Mock Adapter；真实微信、WeKnora 与搜索服务需在后续通过适配器接入。

## 启动联调环境

```powershell
docker compose -f infrastructure/docker/docker-compose.yml up -d
```

Compose 会同时启动 Backend、PostgreSQL 和 Redis。仅 Backend 的 `18080` 对宿主机开放，数据库与缓存只在 Docker 内部网络可见。

服务入口：`http://localhost:18080/`；健康检查：`http://localhost:18080/healthz`。Android 模拟器默认通过 `10.0.2.2:18080` 访问本机后端。点击 `四六级什么时候报名？` 或输入 `官网搜索测试` 可验证检索、SSE、来源和历史记录链路。

当前架构与扩展方式见 `docs/architecture-v0.5.md`；历史阶段说明保留在 `docs/phase-*.md`。
