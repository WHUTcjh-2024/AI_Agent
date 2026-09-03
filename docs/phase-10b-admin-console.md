# Phase 10B — 独立内部管理控制台

日期：2026-09-03。承接 Phase 10A 聚合 API 和 Phase 13A 工程评测，落实原开发方案的内部运营查看能力。知识数据仍在准备，本阶段不宣称真实知识质量或校园试点验收完成。

## 交付

`apps/admin` 提供独立 Go 服务及嵌入页面。浏览器以独立控制台口令登录，服务端用 `ASKU_ADMIN_API_TOKEN` 调用现有后端；没有修改移动端的登录方式，也不把 Admin Token 编入前端资源。

- 概览：问题量、期间活跃用户、运行完成率、估算成本。
- 趋势：逐日问题量与活跃用户、可展开明细、回答路径分布。
- 质量与成本：失败、取消、无引用、负向反馈、缓存、TTFT/耗时、Token、费用。
- 用户与留存：累计注册、人均提问、每会话提问、D1/D7 可计算样本。
- 热门问题、错误码、快捷日期和自定义日期筛选。
- 加载、空库、接口错误、日期错误、会话过期、重新登录与退出状态。

服务端只代理指定 API，不跟随重定向；后端错误不直接展示原始内容。代理请求超时 15 秒、响应上限 2 MiB，浏览器验证统计字段完整性，避免把接口漂移解释成全零指标。返回页面与接口均禁止缓存，问题文本通过 DOM textContent 渲染。

## 统计口径

| 页面内容 | 解释 |
| --- | --- |
| 期间活跃用户 | 当前整个时间窗口内的活跃用户；不是单日 DAU |
| 完成率 | `completedRuns / runs`，取消及未终结运行仍在分母中 |
| 无引用回答 | 已完成、非 controlled 路由、无 Citation 的回答数量；不等同于事实错误数量 |
| 费用 | API 的 Micro RMB 除以 1,000,000；按服务端模型单价估算，不是供应商账单 |
| 缓存命中率 | `cacheHits / runs`；分母为零显示 `—` |
| D1 / D7 | 同时展示 retained/eligible；eligible 为零显示“暂无可计算样本” |
| 耗时零值 | 当前 API 无独立耗时样本数，零值显示 `—`，不推断零延迟 |
| 默认窗口 | 沿用后端最近 7×24 小时，可跨 8 个自然日；页脚展示实际起止时刻 |
| 快捷 / 自定义 | 依后端统计时区，结束日期包含当天；最多 90 个自然日 |

自定义日期最终由后端验证。当前学校时区是 Asia/Shanghai；存在夏令时的学校配置仍受后端最多 90×24 小时限制，页面会显示接口校验结果。留存随观察窗口积累，不以联调样本作为产品留存结论。

## 隔离联调环境

使用 PowerShell 7，在仓库根目录打开三个终端。避免与 `scripts/eval.ps1` 同时启停评测依赖。

终端一：

```powershell
docker compose -f infrastructure/docker/docker-compose.eval.yml up -d --wait
$env:ASKU_HTTP_ADDR = '127.0.0.1:18081'
$env:ASKU_DATABASE_URL = 'postgres://asku_eval:asku_eval_local@127.0.0.1:15433/asku_eval?sslmode=disable'
$env:ASKU_REDIS_ADDR = '127.0.0.1:16380'
$env:ASKU_REDIS_PASSWORD = ''
$env:ASKU_SCHOOL_CONFIG = (Resolve-Path config/schools/whut.yaml).Path
$env:ASKU_DEV_AUTH_ENABLED = 'true'
$env:ASKU_AGENT_MODE = 'policy'
$env:ASKU_LLM_PROVIDER = 'mock'
$env:ASKU_LLM_MODEL = 'asku-mock'
$env:ASKU_WEB_SEARCH_PROVIDER = 'mock'
$env:ASKU_KNOWLEDGE_PROVIDER = 'disabled'
$env:ASKU_ADMIN_TOKEN = 'asku-local-admin-do-not-use-in-production'
Set-Location backend
go run ./cmd/api
```

终端二：按 [Admin README](../apps/admin/README.md) 启动控制台，将 `ASKU_ADMIN_API_BASE_URL` 改为 `http://127.0.0.1:18081`，设置自己的控制台口令。终端三：

```powershell
$env:ASKU_ADMIN_TEST_PASSWORD = Read-Host '当前控制台口令' -MaskInput
./scripts/admin-smoke.ps1 -ReportPath evals/reports/phase-10b/admin-smoke.json
```

测试完成后，在两个服务终端按 Ctrl+C，再执行：

```powershell
docker compose -f infrastructure/docker/docker-compose.eval.yml down
```

只操作 `asku-eval` 项目；测试账号和统计存在隔离数据库中。该 PostgreSQL 使用 tmpfs，停止容器即丢弃测试数据；报告保存在宿主机的 `evals/reports` 下。不要把此数据库用于正式运营。

## 验证与回归门禁

- Go 竞态测试和 vet：会话签发/轮换/到期/退出、跨站请求、代理凭证、登录限流、配置、异常响应与重定向。
- Node 测试：缺字段拒绝、零分母、费用单位、包含结束日的 90 天边界、时区/夏令时、重复逐日数据；另检查全部前端模块语法。
- `scripts/admin-smoke.ps1`：真实 HTTP → Session → AgentRun → SSE → PostgreSQL 聚合 → Console 代理，26 项检查。断言单次提问和完成运行增量、10 组 API 数据一致、普通用户无法访问、静态文件不含实际凭证、退出失效。
- 手动浏览器检查：空库、错误口令、登录/退出、刷新恢复会话、提问后刷新、完整自然日筛选、无效日期、逐日明细和留存样本提示；停用后端后显示错误，恢复服务后可重试加载。
- GitHub Actions 的 `admin` 作业启动独立 PostgreSQL/Redis，构建两个服务与 Admin 镜像，执行同一联调脚本，上传 `admin-integration` 报告及日志。

工程联调使用 Mock LLM/Web Search、关闭 Knowledge。真实供应商效果、真实微信身份、知识准确性、真机网络和多人生产部署需要后续独立验收。

## 后续顺序

下一阶段补齐原方案 Phase 2：微信 Provider、移动端安全凭证存储和账号生命周期，以及用户日额度/试点预算。外部微信配置未到位时先完成契约与测试；数据侧继续按批次审计、Canary、正式导入、Phase 13B 真实评测推进。
