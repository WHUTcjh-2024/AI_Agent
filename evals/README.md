# AskU 工程评测

Phase 13A 验证产品控制层的确定性行为。测试 Provider 的 Evidence 是合成样本，不表示校园问题答对，也不会导入合作伙伴知识库。

## 一键执行

在仓库根目录，安装 Go 及 Docker 后运行：

```powershell
./scripts/eval.ps1
```

默认执行 `all`，启用竞态检测，自动启动独立的 `asku-eval` PostgreSQL/Redis。脚本新启动的服务在结束时自动关闭；已存在的评测服务保留。`-KeepServices` 可保留本次启动的服务。

竞态检测需要 C 编译器（Windows 可使用已配置的 MinGW）。没有编译器时使用 `-NoRace`，报告会明确记录 `race=false`。

只跑不依赖数据库和 Docker 的固定样本回归：

```powershell
./scripts/eval.ps1 -Suite offline
```

固定输出目录用于本地查看：

```powershell
./scripts/eval.ps1 -OutputDirectory evals/reports/latest
```

省略输出目录会生成唯一 UTC 时间目录，保留各轮结果。`evals/reports/` 已加入 Git 忽略规则。

## Go 入口与 CI

在 `backend` 目录：

```sh
go run ./cmd/eval --suite offline
go run ./cmd/eval --suite all --race --out ../evals/reports/ci
```

`integration` 和 `all` 需要独立评测服务环境变量：

| 变量 | 本地评测值 |
| --- | --- |
| `ASKU_EVAL_POSTGRES_URL` | `postgres://asku_eval:asku_eval_local@127.0.0.1:15433/asku_eval?sslmode=disable` |
| `ASKU_EVAL_REDIS_ADDR` | `127.0.0.1:16380` |
| `ASKU_EVAL_REDIS_PASSWORD` | 本地 Compose 留空 |

可通过 `docker compose -f infrastructure/docker/docker-compose.eval.yml up -d --wait` 启动。每个联调用例从名为 `asku_eval` 的专用引导数据库创建自己的随机数据库，使用真实迁移，并在结束时仅删除该随机数据库。Redis 测试键有独立前缀或唯一用户 ID，并设置过期时间；不执行 FlushDB。评测服务与日常开发服务使用不同项目名和端口。

普通 `go test ./...` 默认跳过这些数据库联调用例。评测执行器显式启用它们；选中了 `integration` 却没有服务配置时会失败，不会跳过后报绿。

GitHub Actions 的 `evaluation` Job 执行全部工程检查和竞态检测，不需要供应商凭证；无论检查成功与否都会上传 `engineering-evaluation` Artifact。

## 清单与结果契约

- `engineering.yaml`：每项包含唯一 `id`、`suite`、说明及精确 Go package/test 名称；复用现有测试断言，避免维护另一套业务规则。
- `fixtures/school.yaml`：隔离联调的合成学校配置，域名为 `university.example`；固定 Fetcher 直接返回测试 HTML，不访问该域名。
- `golden-questions.yaml`：保留真实问题和人工审核标准，当前均作为 `blocked_data` 输出。

运行产物：

| 文件 | 用途 |
| --- | --- |
| `report.json` | 每项状态与耗时、双门禁、代码提交、工作区脏状态、Go/竞态配置、源码摘要、输入文件摘要、Provider 类型、学校及知识版本、人工审核条件 |
| `report.md` | 可读汇总与失败定位入口 |
| `go-test.jsonl` | Go 原始结构化测试事件，可按 package/test 定位断言、构建错误与子用例 |

`passed`、`failed`、`skipped`、`blocked_data` 分别计数，不把跳过计为通过。未选中的套件记录为 `skipped`；已选测试缺失、失败、进程错误或测试/子测试被跳过时，工程门禁失败。数据门禁在 Phase 13A 始终是 `blocked_data`。

Go 执行器退出码：`0` 为所选工程门禁通过，`1` 为工程检查失败，`2` 为配置、清单或报告写入错误。`go run` 会将程序非零退出映射为自身非零状态；CI 与 PowerShell 入口均据此失败。默认总执行期限 5 分钟，单个测试包期限 120 秒。

本阶段不提供真实 Provider 执行开关。数据准入完成后，在 Phase 13B 单独增加有调用预算的真实评测及人工复核；不能通过改一个状态字段将这些待办视为通过。

## 联调覆盖

- 稳定路由、混合路由、知识关闭/失败降级、Web 失败/零结果、来源域名和 Catalog 映射、引用编号、缓存读写与版本失效。
- 真实 HTTP → Auth → AgentRun → SSE → PostgreSQL 消息/引用 → 来源详情 → Feedback → Admin/Usage。
- 断线后 Worker 继续运行，`Last-Event-ID` 和 `after` 续传不重复、不丢失事件；按 JSON 内容比较持久化重放，不把 JSONB 的字段排序当成变化。
- 检索中取消、重复取消、Provider 超时后重新提问、并发幂等、失败启动释放幂等占用。
- 会话/Run 的账号隔离，公开来源的学校隔离；来源在同校用户间可共享。
- 真实 Catalog SQL 的审核/PII/准入/导入/来源启用门禁，以及中断 Run 的恢复终态。

## 后端容器与现有冒烟脚本

除上述进程内 HTTP 联调外，可以单独启动正式 `cmd/api` 组合根，检查 Docker 构建与现有 API 冒烟链路：

```powershell
docker compose -f infrastructure/docker/docker-compose.eval.yml --profile smoke up -d --build --wait
./scripts/smoke.ps1 -BaseUrl http://localhost:18081
docker compose -f infrastructure/docker/docker-compose.eval.yml --profile smoke down
```

该配置强制使用 Mock LLM/Web 与 disabled Knowledge，连接评测数据库。此冒烟结果独立于 34 项工程报告；不会开启日常环境的真实知识 Provider。
