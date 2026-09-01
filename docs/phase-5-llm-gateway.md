# AskU Phase 5：LLM Gateway + Usage

## 模块边界

```text
Agent routing
    ↓ standard llm.Request
LLM Gateway
    ├─ Provider interface → Mock / OpenAI-compatible
    └─ UsageRecorder interface → PostgreSQL
```

- Agent 不知道供应商 URL、密钥和计费规则。
- Provider 不依赖 Agent、数据库或 HTTP Handler。
- PostgreSQL 只实现 `UsageRecorder`，不参与模型调用。
- 移动端继续消费原 REST/SSE 合同，无需改页面。

## Provider Contract

`internal/llm.Provider` 同时提供 `Generate` 与 `Stream`。上层仅使用统一的 `Request`、`Response`、`StreamEvent` 和 `Usage`。

当前实现：

- `MockProvider`：本地联调，Token 为估算值。
- `OpenAICompatibleProvider`：支持 `/chat/completions` 普通响应及 SSE，配置注入，错误脱敏。

## Usage

每次 Gateway 调用最多写入一条 `usage_records`：

- user / run / provider / model
- input_tokens / output_tokens
- latency_ms / status / error_code
- estimated_cost_micro_rmb
- tokens_estimated

成本按环境变量中的“人民币/百万 Token”单价估算。未配置价格时成本为 0，不伪造供应商账单。

## 当前安全边界

WeKnora 与校园检索尚未接入。已有官方来源的受控场景由 Agent 直接回答；无来源场景明确拒答。LLM Gateway 只验证模型调用与计量链路，不被当作学校政策来源。

## 验证

```powershell
cd backend
go test ./...
go vet ./...

cd ..
docker compose -f infrastructure/docker/docker-compose.yml up -d --build
```

容器只向宿主机发布 `18080`；PostgreSQL、Redis 仍仅在 Compose 内网开放。
