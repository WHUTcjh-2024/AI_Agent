# WorkBuddy 数据交付与准入

## 当前数据库快照（2026-09-02）

- `knowledge.documents`: 1,048
- `rag_eligible=true`: 223
- `knowledge.weknora_mappings`: 197，均标记为 `IMPORTED`
- `knowledge.attachments`: 0
- `knowledge.staging_documents`: 0
- `crawler.urls`: 0
- 197 条已导入记录中：98 条 `ACCEPTED`，99 条仍为 `REVIEW`

当前批次不得用于正式知识问答。Backend 已采用 deny-by-default 门禁，仅允许 `ACCEPTED + rag_eligible + no PII + active source + IMPORTED mapping`。

## 必须修正的契约漂移

### Audience

| 当前值 | 标准值 |
| --- | --- |
| `武汉理工大学学生` | `ALL` |
| `研究生` | `GRADUATE` |
| `本科生` | `UNDERGRADUATE` |

### Secondary topic

| 当前值 | 标准值 |
| --- | --- |
| `innovation_entrepreneurship` | `innovation_project` |
| `career` | `career_procedure` |
| `party_youth_league` | `party_youthleague` |
| `health_insurance` | `medical` |
| `financial` | `financial_aid` |
| `accommodation` | `dorm` |
| `teacher_education` | `other` |

`postgraduate_admission`、`volunteer_service` 暂无已批准标准分类，不得自行新增枚举；先标记 `other + UNCERTAIN + rag_eligible=false`，待产品确认后再扩展 Taxonomy。

## Canary 前置条件

1. 所有枚举值通过 `config/taxonomy.yaml` 校验。
2. `REVIEW`、`UNCERTAIN` 文档不得写入正式 `IMPORTED` 映射。
3. PII 检测实际执行并留下审计结果，不能只使用默认 `false`。
4. 重复内容组完成主文档选择与版本关系标注。
5. 网页与附件形成 Knowledge Bundle；附件数量不能为 0。
6. 数据先进入 `knowledge.staging_documents`，Canary 通过后再晋升。
7. 导入后写入 WeKnora ID 到 AskU Document/Attachment 的稳定映射。
8. 全量导入完成后提升 `knowledge_version`，再开启 Backend 的 WeKnora Provider。
