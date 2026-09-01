# AskU Backend V0.5

当前后端采用 Handler → Run Coordinator → Agent Executor → Capability Adapter 的单向依赖。包含 Go API、PostgreSQL 会话、Redis、AgentRun、可重连 SSE、模型/搜索 Provider、用量记录和官方域名内的 Top-N 页面抓取。

## 当前边界

- `ASKU_AGENT_MODE=mock` 是受控路由 Adapter；`ASKU_LLM_PROVIDER=mock` 是默认模型 Adapter。两者均不代表 WeKnora 或真实校园政策已接入。
- `openai-compatible` Provider 可通过环境变量接入兼容 `/chat/completions` 的 API；Base URL、Key、Model 均不写入代码或日志。
- `/v1/auth/wechat` 保留正式接口，但没有 AppID/签名时返回 `wechat_not_configured`。
- `SchoolContext` 从 `config/schools/whut.yaml` 加载，业务 Handler 不写死学校域名和知识库 ID。
- `ASKU_WEB_SEARCH_PROVIDER=mock` 默认不访问公网；切换 `searxng` 只改变 Provider Adapter。官方域名过滤、抓取、提取与三级缓存仍由 Gateway 统一执行。

## 启动

在仓库根目录：

```powershell
docker compose -f infrastructure/docker/docker-compose.yml up -d
```

Compose 中仅发布 Backend 的 `18080`；PostgreSQL 与 Redis 只使用项目内部网络，不暴露宿主机端口。

健康检查：

```powershell
Invoke-RestMethod http://localhost:18080/healthz
```

## 测试登录

开发环境由移动端自动调用：

```http
POST /v1/auth/dev-login
Content-Type: application/json

{"externalId":"local-tester","nickname":"AskU 测试同学"}
```

测试数据会进入 PostgreSQL，Access/Refresh Token 为服务端签发的随机不透明令牌，数据库只保存 SHA-256 Hash。

## SSE

发送问题后返回 `run.id`，再连接：

```http
GET /v1/runs/{run_id}/events?after=0
Accept: text/event-stream
Authorization: Bearer <access_token>
```

事件带递增 `id`。断线后使用 `Last-Event-ID` 或 `after` 续传；历史事件存储在 PostgreSQL，生成任务不依赖当前 APP 连接。

## LLM Gateway

默认 Mock 模式无需密钥。切换兼容 API 时配置：

```text
ASKU_LLM_PROVIDER=openai-compatible
ASKU_LLM_BASE_URL=https://provider.example/v1
ASKU_LLM_API_KEY=<secret>
ASKU_LLM_MODEL=<model>
ASKU_LLM_INPUT_RMB_PER_MTOK=<input price>
ASKU_LLM_OUTPUT_RMB_PER_MTOK=<output price>
```

调用统一写入 `usage_records`，记录 Provider、Model、输入/输出 Token、延迟、状态、错误码和估算成本。Mock Token 明确标记为估算值。

## Web Search Gateway

输入 `官网搜索测试` 可走完整搜索联调链路。详细边界、配置和测试见 `docs/phase-6-web-search-gateway.md`。

## 测试命令

```powershell
go test ./...
go vet ./...
go test -race ./...
```

架构边界和扩展方式见 `../docs/architecture-v0.5.md`。
