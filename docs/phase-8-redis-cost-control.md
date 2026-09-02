# AskU Phase 8：Redis Cost Control

## 完成范围

Phase 8 用 Redis 降低重复调用成本并保护运行时，不引入复杂 Semantic RAG Cache：

```text
Request
  → per-user Rate Limit
  → Idempotency reservation
  → Versioned Answer Cache
  → Policy Router
      ├─ Knowledge Query Cache → WeKnora
      └─ Search / Page / Extract Cache → Official Web Search
  → grounded generation
  → cache eligible Knowledge answer
```

## 解耦边界

- `cache.Redis`：只负责 Redis I/O、限流和幂等原语，不引用 Agent、Knowledge 或 Web Search。
- `agent.VersionedAnswerCache`：拥有可靠答案的读写、Key 和版本失效规则，仅依赖 JSON Cache 端口。
- `knowledge.Gateway`：拥有 Query Cache 策略，只缓存经过字段清洗和官方域名校验的 Evidence，并在每次读取后再次校验。
- `websearch.Gateway`：继续拥有 Search、Page、Extract 三级缓存。
- `api.Server`：只消费限流和幂等端口，阈值来自配置。
- `cmd/api/main.go`：唯一组装 Redis 与各业务缓存端口的位置。

## Key 与失效规则

```text
rate:user:{user_id}:{minute}
idem:{user_id}:{request_key_hash}
query:{school_id}:{knowledge_version+provider+kb+query+top_n hash}
search:{school_id}:{query_hash}
page:{url_hash}
extract:{query+url hash}
answer:{school_id}:{knowledge_version}:{query_hash}
```

外部 Idempotency-Key 只保存摘要，不直接进入 Redis Key。Query Cache 包含知识版本、Provider、知识库 ID 和 Top-N，避免数据更新、供应商切换或不同召回深度互相污染。

每所学校必须在 SchoolContext 中配置 `knowledge_version`。校园资料更新后提升版本，旧 Answer Cache 会自然不可达并按 TTL 清理，无需扫描 Redis。

## 答案缓存准入

只缓存同时满足以下条件的结果：

1. 路由为 `knowledge`；
2. 答案非空且大小受限；
3. 至少有一个来源；
4. 所有来源均标记为官方且具有稳定 Source ID。

实时网页搜索结果、产品说明、网络错误和“暂时没有找到可靠信息”均不进入 Answer Cache，避免把时效信息或失败状态固化。

## 故障策略与观测

- Query/Answer Cache 读写错误均 fail-open，继续调用原能力。
- Provider、数据库或 Run 创建失败时，不缓存结果。
- Run 创建失败会释放幂等占位，允许用户安全重试。
- `retrieval.completed` 暴露 Query Cache 命中；Answer Cache 命中通过 `route=cache` 和 `cacheHit=true` 暴露。

## 配置

```text
ASKU_KNOWLEDGE_QUERY_CACHE_TTL=10m
ASKU_ANSWER_CACHE_TTL=30m
ASKU_QUESTION_RATE_LIMIT_PER_MINUTE=30
```

试点阶段通过 TTL 和学校知识版本完成失效控制，不建设 Redis Streams、分布式锁、复杂预算系统或语义相似问题缓存。
