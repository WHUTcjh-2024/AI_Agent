# AskU Phase 6：Web Search Gateway

## 完成范围

Phase 6 只建立可替换的学校官网搜索能力，不提前实现完整 Agent Orchestrator：

- 官方域名搜索结果过滤；
- Top-N 页面抓取与正文限制；
- 与问题相关的页面片段提取；
- 搜索结果、页面正文、提取结果三级 Redis 缓存；
- 默认 Mock Provider 与可配置 SearXNG Provider；
- 保持移动端 REST/SSE 事件类型不变。

## 依赖方向

```text
Run Service → websearch.Searcher
                    │
                    ▼
             Web Search Gateway
              ├─ Provider
              ├─ Fetcher
              ├─ Extractor
              ├─ JSONCache
              └─ ScopeResolver
```

`Run Service` 不知道 SearXNG、HTTP 抓取或 Redis 细节；Provider 不负责学校白名单；学校配置也不依赖搜索类型。后续 Agent Orchestrator 只消费 `websearch.Searcher`。

## 安全与可靠性

- URL 只允许 `http/https`，拒绝 UserInfo、伪后缀域名和白名单外域名；
- 每次 HTTP 重定向都重新校验学校域名白名单；
- 页面正文默认最大 2 MiB，只接受 HTML / Text；
- 抽取阶段忽略 script、style、nav、header、footer、form、svg；
- 单页失败只丢弃该页，不生成未经验证的政策答案；
- 未解析到发布时间时绝不拿抓取时间冒充发布时间。

## 缓存

```text
search:{school}:{query_sha256}       默认 10 分钟
page:{url_sha256}                    默认 30 分钟
extract:{query_url_sha256}           默认 30 分钟
```

缓存不可用时搜索链路 fail-open；Provider 或页面抓取错误不会被缓存。答案缓存与限流已在 Phase 8 通过独立端口接入。

## 配置

```text
ASKU_WEB_SEARCH_PROVIDER=mock        # mock | searxng
ASKU_WEB_SEARCH_BASE_URL=
ASKU_WEB_SEARCH_API_KEY=
ASKU_WEB_SEARCH_TIMEOUT=12s
ASKU_WEB_SEARCH_TOP_N=3
ASKU_WEB_SEARCH_SEARCH_TTL=10m
ASKU_WEB_SEARCH_PAGE_TTL=30m
ASKU_WEB_SEARCH_EXTRACT_TTL=30m
```

默认 Mock 模式不访问公网，返回可点击的武汉理工大学官方主页链接，并用内置页面正文验证 Gateway。切换 SearXNG 只改环境变量；API Key 不写入代码、事件或日志。

## 联调入口

在 APP 点击 `四六级什么时候报名？`，或输入 `官网搜索测试`：

1. `route.resolved` 选择 `web_search`；
2. `retrieval.started` 的 engine 为 `web-search`；
3. `sources.updated` 返回官方来源及缓存统计；
4. 原有 `message.delta / completed` 合同继续工作。

此入口只证明搜索、抓取、缓存与来源展示已接通，不代表真实校园政策检索已完成。

## 验证

```powershell
cd backend
go test ./...
go vet ./...
go test -race ./...

cd ..
docker compose -f infrastructure/docker/docker-compose.yml up -d --build
Invoke-RestMethod http://localhost:18080/healthz
```

单元测试覆盖白名单绕过、跨域重定向、正文大小、HTML 去噪、Top-N、三级缓存和 SearXNG 协议映射。
