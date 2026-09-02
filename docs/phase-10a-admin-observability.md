# Phase 10A — Admin & Observability Backend

## 范围

Phase 10A 只完成可供内部 Admin 使用的只读数据面，不建设复杂 CRM，也不在移动端暴露运营指标。Phase 10B 可基于稳定契约建设独立 Admin UI。

```text
users/messages/feedback/usage_records
                 + immutable run_events
                           ↓
              PostgreSQL read-only snapshot
                           ↓
            GET /v1/admin/overview
```

## 已实现

- 当前学校、最多 90 天的半开时间窗 `[from,to)`；日期参数按 `ASKU_REPORTING_TIMEZONE` 解释。
- 用户：累计注册、新增、活跃、D1/D7 留存及可计算样本数。
- 使用：问题数、活跃会话、Questions/Active User、平均会话提问轮数、Top Questions。
- 质量：完成/失败/取消、成功率、检索型回答无 Citation、负向反馈、错误码分布。
- 性能：总耗时平均/P50/P95、TTFT 平均/P95。
- 成本：Input/Output Token、估算人民币微元、单问题成本、Answer Cache 命中率。
- 路由与逐日趋势：controlled / knowledge / web_search / cache 及每日核心指标。

路由、缓存命中和延迟直接从 `run_events` 聚合，不再引入第二套可漂移的埋点状态。查询使用 PostgreSQL `REPEATABLE READ + READ ONLY` 事务，保证一个响应内口径一致。

## 安全边界

- Admin API 使用独立 `ASKU_ADMIN_TOKEN`，不接受普通用户 Access Token。
- 未配置 Token 时接口返回 404；错误 Token 返回 401。
- 当前只允许查询 `SchoolRegistry.Current()`，拒绝跨校读取。
- Top Questions 可能包含用户输入，只能用于受控内部运营环境。
- API 只返回聚合数据，不返回用户 ID、昵称、会话 ID 或内部知识文件路径。

## 联调

Compose 开发环境默认提供本地专用 Token，生产环境必须覆盖：

```powershell
$env:ASKU_ADMIN_TOKEN = '<high-entropy-secret>'
docker compose -f infrastructure/docker/docker-compose.yml up -d --build

$headers = @{ Authorization = 'Bearer <high-entropy-secret>' }
Invoke-RestMethod -Headers $headers `
  'http://localhost:18080/v1/admin/overview?from=2026-09-01&to=2026-09-02'
```

不传 `from/to` 时默认返回最近 7×24 小时。`to=2026-09-02` 表示包含 Asia/Shanghai 的 9 月 2 日，响应会返回实际 UTC 半开边界。

## 验收

- 无 Token / 错 Token / 普通用户 Token 无法读取管理数据。
- 空库返回完整零值对象和连续日期序列，不返回 `null` 数组。
- 创建用户并完成一次提问后，问题、活跃用户、Run、路由、延迟和 Token 指标可追踪。
- 缓存命中后 `routes.cache` 与 `cost.cacheHitRate` 增长。
- 跨校或超过 90 天时间窗返回 400。
- PostgreSQL、Redis、Go 单测、竞态检查和真实 HTTP 联调通过。
