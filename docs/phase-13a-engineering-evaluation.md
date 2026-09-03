# Phase 13A — 工程评测与联调

本阶段实现 [数据准备期间计划](next-steps-data-pending.md) 中的工程评测框架，数据仍未完成准入。执行方式和报告契约见 [evals/README.md](../evals/README.md)。

## 实现边界

```text
engineering.yaml + golden-questions.yaml
  -> backend/cmd/eval
      -> 精确选择现有离线 Go 测试
      -> 独立 PostgreSQL / Redis + 真实 API Handler / SSE / Store
      -> Go JSON 测试事件
  -> report.json + report.md + go-test.jsonl
```

评测代码独立于生产 API，不向业务接口增加故障注入开关。数据库联调在测试组合根中装配真实业务模块，仅将外部 Provider 换成固定样本。`smoke` Compose Profile 另行验证生产组合根的容器启动与 API 链路。

## 首批交付

- 25 项离线检查，覆盖路由、混合检索、来源门禁、引用和缓存，以及 Provider 包装取消信号的终态规则。
- 9 项数据库/HTTP 联调，覆盖完整回答、Knowledge disabled、断线续传、取消、超时重试、幂等与权限、知识缓存、Catalog 准入和中断恢复。
- 原有 3 个真实知识问题保留人工复核条件，统一标记 `blocked_data`。
- 执行器自身通过真实子进程验证成功、断言失败、测试被重命名/缺失和跳过四种门禁行为。
- CI 新增独立评测 Job，生成并上传报告与原始测试事件。

报告包含 Git 提交与工作区状态、Go 版本、是否启用竞态检测、后端源码 SHA-256、清单与样本配置摘要、Provider 类型及知识版本。报告中的耗时是工程检查执行时间，不是实际供应商的 TTFT 或成本基线。

## 联调发现并修复的问题

检索取消信号可能被 Adapter 包装为 `agent.ExecutionError`。原 Run Service 先处理 Provider 错误类型，会将取消记为 `FAILED` 并返回可重试的 Provider 错误。

现在优先识别 `context.Canceled`（包括包装错误及已取消的执行上下文），落库为 `CANCELLED`，通过 `run.failed` 事件发送 `code=cancelled`、`retryable=false`。增加固定回归用例和检索中实际取消的 HTTP 联调，保持现有 SSE 契约。

## 当前验收与后续

2026-09-03 本地验证：

- `./scripts/eval.ps1`：34 项工程检查通过，0 失败、0 跳过、3 项 `blocked_data`，启用竞态检测。
- `go test -race ./...`、`go vet ./...` 通过，移动端 `npm run typecheck` 通过。
- 编译并运行正式 `cmd/api`，连接独立评测数据库与 Redis；现有 `smoke.ps1` 通过，产生 9 个 SSE 事件、2 条消息、3 条引用，Admin 可见混合路由。
- Docker 镜像构建因 `auth.docker.io` 连接超时未完成；以上正式入口冒烟使用本机编译产物，不代表容器构建已经通过。CI 配置已接入，远端执行待提交后验证。

工程门禁与数据门禁独立。34 项工程检查通过也不能代表知识准确率通过；数据审核、实际 WeKnora 环境验收、真实引用支持性检查和校园试点仍按原计划推进。

下一项可开展 Phase 10B Admin 页面。Phase 13B 待数据准入后增加真实服务调用预算、质量基线及人工复核流程。
