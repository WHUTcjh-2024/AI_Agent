# Gold V2 导入验收报告

更新时间：2026-09-06。

## 结果

Gold V2 原始包包含 36 条来源记录和 100 条评测题。清洗与准入后，13 篇新增官方文档进入 WeKnora；23 条来源因已存在、正文不足、证据未完成复核、PII 或版面质量等原因未进入知识库。评测题与知识文档分离，问答记录没有被当作文档导入。

| 检查项 | 结果 |
| --- | ---: |
| 新增 WeKnora 文档 | 13/13 |
| 新增文档解析并启用 | 13/13 |
| AskU 来源记录 | 13/13 |
| AskU 文档记录 | 13/13 |
| AskU-WeKnora 映射 | 13/13 |
| 目标知识库总文档 | 52/52 已解析并启用 |
| 目标知识库总映射 | 52/52 `IMPORTED` |
| 新增文档 PII | 0 |
| 阻断质量标记进入知识库 | 0 |

知识版本已提升为 `v20260906-gold-v2`。重复运行导入脚本会按 `external_id + clean_content_sha256` 复用已有文档；同一 ID 内容变化时会失败退出，避免静默覆盖。

## 检索验收

- 首轮逐文档 Canary 在 Top 10 中命中 11/13。
- 针对性复核后，招生章程文档可被精确检索并引用，文档级命中为 12/13。
- `SRC034` 校园 VPN 文档已解析、启用并参与检索，但旧库中语义近重复的校外数据库访问文档排名更高。对应用户问题仍可得到有官方引用的正确答案，意图级覆盖为 13/13。后续应做重复文档分组或重排优化，不应为提高命中率删除已审核的新文档。
- AskU 稳定知识问答端到端通过；引用包含 AskU 文档 ID 和学校官方 URL。
- AskU 混合问答端到端通过：WeKnora 与 SearXNG 同时工作。测试问题缺少下半年正式通知时，系统明确说明信息不足，没有推断或编造日期。

## 当前完成度

以“小范围真实校园试点可发布”为目标，当前整体完成度评估为 **78%**。

| 领域 | 权重 | 已完成 | 判断 |
| --- | ---: | ---: | --- |
| 核心架构、后端与 Agent | 25% | 24% | 会话、SSE、持久化、路由、缓存、引用、Provider 和成本记录已实现，39 项工程门禁通过 |
| 移动端与 Admin | 15% | 12% | 主流程和运营统计已实现，仍缺真机完整流程与安全凭证迁移 |
| 知识数据与检索 | 25% | 19% | 52 篇正式文档可用；旧高频题基线 83/103，Gold V2 仍有 88 条评测题待证据复核 |
| 评测与可靠性 | 15% | 12% | 自动工程评测、实时知识和混合链路均通过；Phase 13B 人工答案质量基线未完成 |
| 发布、安全与运营 | 20% | 11% | 本地环境可运行；微信真实登录、系统安全存储、HTTPS 签名包、预算硬限制和试点监控仍待完成 |

本地可人工测试的 MVP 完成度约 **90%**；真正进入受控校园试点前仍需完成以下工作：

1. 接入微信真实授权并关闭生产环境开发登录。
2. 将移动端 Access/Refresh Token 从 AsyncStorage 迁入系统安全存储。
3. 完成 Android 签名包、HTTPS 地址、真机弱网和跨设备历史恢复验收。
4. 完成 Phase 13B：复核 Gold V2 剩余 88 条评测题，测量答案准确性、引用支持度、时效性、延迟和成本。
5. 增加用户日额度与试点总预算硬限制，覆盖并发、重试、取消和模型自动切换。
6. 优化近重复知识的分组或重排，再开展小范围试点监控。

## 可复查产物

- 清洗报告：`asku-knowledge/reports/gold-v2-20260906/data-cleaning-report.md`
- WeKnora 导入明细：`asku-knowledge/reports/gold-v2-20260906/weknora-import.json`
- AskU Catalog 同步：`asku-knowledge/reports/gold-v2-20260906/asku-catalog-sync.json`
- 逐文档 Canary：`asku-knowledge/reports/gold-v2-20260906/retrieval-canary.json`
- 低排名复核：`asku-knowledge/reports/gold-v2-20260906/retrieval-recheck.json`
- 综合检索结论：`asku-knowledge/reports/gold-v2-20260906/retrieval-acceptance.json`
- 稳定知识问答：`asku-knowledge/reports/gold-v2-20260906/asku-e2e.json`
- 混合问答：`asku-knowledge/reports/gold-v2-20260906/asku-hybrid-e2e.json`
- 工程评测：`evals/reports/20260906-gold-v2-final/report.md`
