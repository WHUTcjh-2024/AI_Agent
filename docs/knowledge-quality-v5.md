# 知识数据 V5：WeKnora 生产候选交付

验收日期：2026-09-05。学校：武汉理工大学。完整清洗源批次：`20260905-v4`；生产候选批次：`20260905-v5-production`。

V5 按“无法证明就不进入正式候选”的原则，从 10,479 条记录中物理删除 10,076 条高风险噪声，保留 403 条治理记录。47 份文档通过证据审核，其中 23 份为附件，组成 12 个完整父子知识包；352 条进入复核队列，另有 4 条只用于上下文裁决审计，均不进入 WeKnora 导入目录。

## 删除结果

| 删除原因 | 数量 |
|---|---:|
| 语义、主题或受众范围无法可靠确定 | 6,921 |
| 解析失败、PII 状态不合格或正文长度异常 | 1,500 |
| 新闻、结果公示或非学生事务 | 976 |
| 表单或不支持的压缩归档 | 361 |
| 失去可信父文的附件 | 207 |
| 重复正文 | 59 |
| 2023 年以前且未经审核的年度通知 | 52 |
| 合计 | 10,076 |

`deletion-manifest.jsonl` 逐条记录删除 ID、原因、来源正文哈希和官方 URL。删除发生在新的生产候选批次中；原始清洗批次继续作为不可变审计来源。

## WeKnora 数据门槛

| Canary 项目 | 要求 | 实际 |
|---|---:|---:|
| HTML / Markdown | 10 | 24 |
| PDF | 10 | 10 |
| DOCX | 5 | 5 |
| Excel | 5 | 5 |
| 完整知识包 | 3 | 12 |
| 推免主题 | 3 | 5 |
| 转专业主题 | 3 | 3 |
| 研究生主题 | 3 | 12 |
| 已审核版本系列 | 2 | 2 |

70 条原文证据全部通过，210 条跨学校、跨适用期和“最新政策”反例全部被拒绝。47 条白名单文档逐条通过正确范围和“未经在线验证不得回答现行政策”检查。Excel 只保留不含学号、身份证号、工号、主考教师和监考教师字段的课程目录；考试人员表、项目名单和空白申报表均未放行。

本批次达到 `WEKNORA_CANARY_IMPORT_READY` 数据标准。`production_ready` 仍为 `false`：必须在真实 WeKnora 实例完成上传、稳定 ID 映射、检索和引用评测，并把适用期门禁接入问答链路后，才能启用正式 Provider。

## 交付位置

- 完整生产候选：`asku-knowledge/data/clean-batches/20260905-v5-production/`
- WeKnora 导入目录：`asku-knowledge/exports/weknora-whut-v5/`
- 验收报告：`asku-knowledge/reports/quality-delivery-20260905-v5/`
- 最小交付 ZIP：`asku-knowledge/reports/quality-delivery-20260905-v5/whut-quality-v5.zip`

WeKnora 导入目录中的 `documents/*.md` 是经过哈希复核的 47 份正文，`import-manifest.jsonl` 提供 AskU 稳定文档 ID、附件 ID、来源 URL、主题、教育层次、适用期、回答范围和知识包 ID。上传成功后必须将 WeKnora 返回的知识 ID 写回 `knowledge.weknora_mappings`，状态先保持 Canary，检索验收通过后才能写为 `IMPORTED`。

## 复现命令

```powershell
python asku-knowledge/scripts/prepare_quality_review.py --source asku-knowledge/data/clean-batches/20260904-v3b --evidence-source asku-knowledge/data/clean-batches/20260905-v4 --plan asku-knowledge/config/quality-review-whut-v5.yaml --output asku-knowledge/reports/quality-delivery-20260905-v5
python asku-knowledge/scripts/quality_batch.py release --batch asku-knowledge/data/clean-batches/20260905-v4 --reviews asku-knowledge/reports/quality-delivery-20260905-v5/review-ledger.json --school config/schools/whut.yaml --taxonomy asku-knowledge/config/taxonomy.yaml --raw-root D:/Desktop/datasets/asku-knowledge/data/whut/raw
python asku-knowledge/scripts/prune_production_batch.py --source asku-knowledge/data/clean-batches/20260905-v4 --output asku-knowledge/data/clean-batches/20260905-v5-production
python asku-knowledge/scripts/verify_quality_delivery.py --batch asku-knowledge/data/clean-batches/20260905-v5-production --cases asku-knowledge/reports/quality-delivery-20260905-v5/evidence-cases.json --output asku-knowledge/reports/quality-delivery-20260905-v5
python asku-knowledge/scripts/verify_weknora_candidate.py --batch asku-knowledge/data/clean-batches/20260905-v5-production --pipeline asku-knowledge/config/pipeline.yaml --output asku-knowledge/reports/quality-delivery-20260905-v5
python asku-knowledge/scripts/export_weknora_canary.py --batch asku-knowledge/data/clean-batches/20260905-v5-production --report asku-knowledge/reports/quality-delivery-20260905-v5 --output asku-knowledge/exports/weknora-whut-v5
```

每次重放必须使用不存在的新输出目录。不得把 `review_queue.jsonl`、完整 SQLite 或删除清单中的记录上传 WeKnora。
