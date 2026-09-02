# AskU Monorepo

- `apps/mobile`：React Native / Expo 移动端
- `backend`：Go API 与 AgentRun/SSE 控制层
- `contracts`：OpenAPI 与 SSE 事件契约
- `config/schools`：学校上下文配置
- `infrastructure/docker`：本地 PostgreSQL / Redis
- `docs`：架构与开发说明

当前版本为 Architecture V0.9 / Phase 10A：在 Citation Trust Chain 基础上增加受独立 Token 保护的 Admin & Observability 后端。运营指标从 PostgreSQL 长期事实和不可变 AgentRun 事件聚合，管理查询不进入移动端业务链路。

## 启动联调环境

```powershell
docker compose -f infrastructure/docker/docker-compose.yml up -d
```

Compose 会同时启动 Backend、PostgreSQL 和 Redis。仅 Backend 的 `18080` 对宿主机开放，数据库与缓存只在 Docker 内部网络可见。

服务入口：`http://localhost:18080/`；健康检查：`http://localhost:18080/healthz`。Android 模拟器默认通过 `10.0.2.2:18080` 访问本机后端。点击 `四六级什么时候报名？` 或输入 `官网搜索测试` 可验证检索、SSE、来源和历史记录链路。

当前架构与扩展方式见 `docs/architecture-v0.9.md`，Phase 10A 实现与联调见 `docs/phase-10a-admin-observability.md`；历史阶段说明保留在 `docs/phase-*.md`。

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
