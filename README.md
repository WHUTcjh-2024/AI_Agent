# AskU Monorepo

- `apps/mobile`：React Native / Expo 移动端
- `backend`：Go API 与 AgentRun/SSE 控制层
- `contracts`：OpenAPI 与 SSE 事件契约
- `config/schools`：学校上下文配置
- `infrastructure/docker`：本地 PostgreSQL / Redis
- `docs`：架构与开发说明

当前版本为 Architecture V0.10 / Phase 12 Agent Runtime：时效问题由 Agent 并行组合学校知识库与官方网页，稳定问题仍只走知识库。Phase 11 数据准入仍按 Canary 推进；知识 Provider 默认关闭时，混合路由会安全退化为官方网页检索。

## 启动联调环境

```powershell
docker compose -f infrastructure/docker/docker-compose.yml up -d
```

Compose 会同时启动 Backend、PostgreSQL 和 Redis。仅 Backend 的 `18080` 对宿主机开放，数据库与缓存只在 Docker 内部网络可见。

服务入口：`http://localhost:18080/`；健康检查：`http://localhost:18080/healthz`。Android 模拟器默认通过 `10.0.2.2:18080` 访问本机后端。点击 `四六级什么时候报名？` 或输入 `官网搜索测试` 可验证检索、SSE、来源和历史记录链路。

当前架构与扩展方式见 `docs/architecture-v0.10.md`，Phase 12 实现与联调见 `docs/phase-12-hybrid-agent.md`；历史阶段说明保留在 `docs/phase-*.md`。

## 质量验证

```powershell
cd backend
go test -race ./...
go vet ./...

cd ../apps/mobile
npm run typecheck
npm run doctor
npm run export:android

cd ../..
python -m venv asku-knowledge/.venv
asku-knowledge/.venv/Scripts/python.exe -m pip install -e asku-knowledge
asku-knowledge/.venv/Scripts/python.exe -m unittest discover -s asku-knowledge/tests -v

./scripts/smoke.ps1
```

`smoke.ps1` 会验证健康检查、开发登录、会话、AgentRun、可重连 SSE、引用落库、来源详情和 Admin 指标，并在结束时清理测试会话。GitHub Actions 会对每次 Push 和 Pull Request 执行后端竞态测试、静态检查、容器构建以及移动端类型检查和 Android Bundle 导出。
