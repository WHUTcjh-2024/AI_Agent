# AskU Admin

独立的只读运营控制台，对接后端 `GET /v1/admin/overview`。Go 服务嵌入原生 HTML/CSS/JavaScript，运行时无需 Node；Node 22 仅用于前端测试，无需安装 npm 依赖。

## 本地运行

需要 Go 1.26 和已启动的后端。以下命令在 `apps/admin` 执行；环境变量必须显式设置，程序不会自动读取 `.env`。

```powershell
$env:ASKU_ADMIN_ADDR = '127.0.0.1:18090'
$env:ASKU_ADMIN_API_BASE_URL = 'http://127.0.0.1:18080'
# 必须与后端 ASKU_ADMIN_TOKEN 相同；此值仅用于本地开发。
$env:ASKU_ADMIN_API_TOKEN = 'asku-local-admin-do-not-use-in-production'
# 为控制台设置独立口令，不要复用后端 Token。
$env:ASKU_ADMIN_PASSWORD = Read-Host '控制台口令（至少 12 字节）' -MaskInput
$env:ASKU_ADMIN_SECURE_COOKIE = 'false'
go run ./cmd/server
```

浏览器访问 `http://127.0.0.1:18090`，输入控制台口令。环境示例见 `.env.example`。前端文件嵌入二进制，修改页面后需重新启动 `go run` 或重新构建，并刷新浏览器。

```powershell
go test -race ./...
go vet ./...
npm test
npm run check
go build -o asku-admin.exe ./cmd/server
```

## 凭证与部署

| 配置 | 用途 |
| --- | --- |
| `ASKU_ADMIN_ADDR` | 默认 `127.0.0.1:18090` |
| `ASKU_ADMIN_API_BASE_URL` | 后端 HTTP(S) origin；默认 `http://127.0.0.1:18080`，不接受路径、查询参数或用户信息 |
| `ASKU_ADMIN_API_TOKEN` | 必填，仅由服务端向后端发送 |
| `ASKU_ADMIN_PASSWORD` | 必填，独立口令，至少 12 字节 |
| `ASKU_ADMIN_SECURE_COOKIE` | 默认 `true`；仅绑定 loopback 时可设 `false` 进行本地 HTTP 联调 |

浏览器使用 8 小时的 HttpOnly、SameSite=Strict 会话 Cookie；会话 ID 不进入 localStorage。后端 Token 不传给浏览器。登录和退出校验同源 Origin，每个直接连接 IP 每分钟最多 5 次登录尝试。代理仅开放统计读取和 `from`/`to` 参数，不接受浏览器传入的后端凭证或任意目标地址。

部署时在 HTTPS 反向代理后运行，保持 `ASKU_ADMIN_SECURE_COOKIE=true`，保留原始 `Host`，通过环境注入随机长口令和后端 Token。反向代理之后的登录限流以代理 IP 计数，不信任客户端 `X-Forwarded-For`；若多人同时使用，应由受信任入口补充限流策略。公网不可直接暴露原生 HTTP 监听端口。

```powershell
docker build -t asku-admin apps/admin
```

上述构建命令在仓库根目录执行。容器中显式设置 `ASKU_ADMIN_ADDR=:18090`，使用 HTTPS 入口和 Secure Cookie，并将 `ASKU_ADMIN_API_BASE_URL` 指向容器可达的后端地址；容器内的 `127.0.0.1` 不代表宿主机。

当前定位为单实例、单学校内部控制台：最多 128 个有效会话，重启后会话失效，变更口令需重启。暂未提供管理员账号体系、RBAC、跨实例会话和操作审计。

## 联调

在独立 PostgreSQL/Redis 和 Mock Provider 后端上运行，不向日常数据库写入测试问题。完整启动配置见 [Phase 10B](../../docs/phase-10b-admin-console.md)。在仓库根目录执行：

```powershell
$env:ASKU_ADMIN_TEST_PASSWORD = Read-Host '当前控制台口令' -MaskInput
./scripts/admin-smoke.ps1 -ReportPath evals/reports/phase-10b/admin-smoke.json
```

脚本需要 PowerShell 7，默认后端端口 `18081`、控制台端口 `18090`；使用 `-BackendUrl` 和 `-ConsoleUrl` 可调整。脚本只接受 loopback 地址，假定隔离环境没有并发业务请求。默认删除本次测试会话，测试用户仍保留在隔离数据库；`-KeepSession` 可暂留成功会话用于页面检查。报告不含凭证，`expectedProviderConfig` 表示启动环境应满足的约束，不是对 Provider 的远程探测。

详情、指标口径与验收记录见 [Phase 10B](../../docs/phase-10b-admin-console.md)。
