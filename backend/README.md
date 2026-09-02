# AskU Backend V0.10

当前后端采用 Handler → Run Coordinator → Agent Orchestrator → Capability Adapter 的单向依赖。包含 Go API、PostgreSQL 会话、Redis 成本控制、AgentRun、可重连 SSE、Policy Router、WeKnora Knowledge Adapter、模型/搜索 Provider 和用量记录。

V0.10 增加 Phase 12 混合 Agent 编排。WeKnora ID 经 `knowledge.*` Catalog 映射为 Crawler 保存的官方元数据，Backend 根据真实 Evidence 生成引用编号；最终 Citation 与消息原子持久化，`message.completed` 是 APP 的引用事实源。

## 当前边界

- `ASKU_AGENT_MODE=policy` 是默认正式路由；`mock` 仅保留开发场景。稳定校园问题只走知识库，时效问题并行执行知识库与官方网页检索。
- 混合路由以官方网页作为时效事实依据；知识检索失败可退化为网页结果，网页失败或零结果时不会拿知识库旧资料冒充最新信息。
- `ASKU_KNOWLEDGE_PROVIDER=disabled` 是安全默认值：没有知识库时明确返回无可靠来源，不提供假检索结果。
- `openai-compatible` Provider 可通过环境变量接入兼容 `/chat/completions` 的 API；Base URL、Key、Model 均不写入代码或日志。
- `/v1/auth/wechat` 保留正式接口，但没有 AppID/签名时返回 `wechat_not_configured`。
- `SchoolContext` 从 `config/schools/whut.yaml` 加载，业务 Handler 不写死学校域名和知识库 ID。
- `ASKU_WEB_SEARCH_PROVIDER=mock` 默认不访问公网；切换 `searxng` 只改变 Provider Adapter。官方域名过滤、抓取、提取与三级缓存仍由 Gateway 统一执行。
- Redis 只实现 JSON Cache、Rate Limit 和 Idempotency 等基础端口；答案是否可缓存、如何按学校知识版本失效，由 Agent 与 SchoolContext 决定。
- `knowledge` Catalog 未找到映射或没有允许域内公开 URL 时，Evidence 不得进入正式答案；任何 `local_file_path` 都不会被 API 查询或返回。

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

## WeKnora Knowledge Adapter

Adapter 使用 WeKnora 官方 `POST /api/v1/knowledge-search`，并通过 `X-API-Key` 认证。启用前需要同时配置：

```text
ASKU_KNOWLEDGE_PROVIDER=weknora
ASKU_WEKNORA_BASE_URL=http://weknora.example:8080
ASKU_WEKNORA_API_KEY=<scoped-api-key>
ASKU_WEKNORA_TIMEOUT=12s
ASKU_KNOWLEDGE_TOP_N=4
```

每所学校的知识库 ID 只写在 `config/schools/<school>.yaml` 的 `official_knowledge_base_id`，不得写入 Router、Handler 或 Provider。详细说明见 `docs/phase-7-agent-orchestrator.md`。

## Redis Cost Control

```text
ASKU_KNOWLEDGE_QUERY_CACHE_TTL=10m
ASKU_ANSWER_CACHE_TTL=30m
ASKU_QUESTION_RATE_LIMIT_PER_MINUTE=30
```

- Knowledge Query Cache 按学校、知识版本、Provider、知识库 ID、规范化问题和 Top-N 隔离。
- Answer Cache Key 包含学校和 `knowledge_version`；更新校园资料后提升该校版本即可使旧答案自然失效。
- 只有具有官方来源的稳定 Knowledge 答案进入 Answer Cache；混合检索、实时搜索、受控回答和无可靠来源回答不读写 Answer Cache。
- Redis 故障时 Query/Answer Cache 均 fail-open，不阻断正常检索和生成。

详细规则见 `docs/phase-8-redis-cost-control.md`。

## Phase 10A Admin & Observability

`GET /v1/admin/overview` 以只读事务聚合用户活跃、留存、问题量、Run 质量、TTFT/总耗时、Token/成本、缓存命中、路由、错误码和每日趋势。接口使用独立 Admin Token，普通用户 Access Token 无权限：

```powershell
$headers = @{ Authorization = 'Bearer asku-local-admin-do-not-use-in-production' }
Invoke-RestMethod -Headers $headers 'http://localhost:18080/v1/admin/overview'
```

未设置 `ASKU_ADMIN_TOKEN` 时接口隐藏为 404。生产环境必须设置高熵 Token；统计时区由 `ASKU_REPORTING_TIMEZONE` 控制，默认 `Asia/Shanghai`。契约与口径见 `../docs/phase-10a-admin-observability.md`。

## 测试命令

```powershell
go test ./...
go vet ./...
go test -race ./...
```

架构边界和扩展方式见 `../docs/architecture-v0.10.md`，混合 Agent 规则见 `../docs/phase-12-hybrid-agent.md`。
