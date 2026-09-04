# 知识数据 V4：准确性优先的离线交付

验收日期：2026-09-05。学校：武汉理工大学。源批次：`20260904-v3b`；交付批次：`20260905-v4`。

本次完成全量规则清洗、证据白名单和独立验收。交付等级是 **限定适用范围的离线证据集**，尚未达到全库生产上线标准。审核类型为 `AI_ASSISTED_EVIDENCE_REVIEW`，不冒充人工专家签字；官网现行性和真实 RAG 问答准确率尚未验证。

## 实测结果

| 项目 | 结果 | 含义 |
|---|---:|---|
| 全量记录 | 10,479 | 原始批次保留，逐条检查清洗文件与哈希 |
| 标题更新 | 5,103 | 从页面元信息、正文标题或附件首段恢复 |
| 分类建议变化 | 5,537 | 自动建议，不等于全部分类已审核正确 |
| 受众建议变化 | 7,632 | 自动建议，不自动获得准入资格 |
| 文本变化 | 6,125 | 标题和空白规范化，不擅改政策数字 |
| 提取版本标签 | 295 | 不据此推断“现行有效” |
| 识别为非学生业务、新闻或结果公示 | 1,035 | 保持隔离，不作为办事规则自动入库 |
| 严格证据白名单 | 17 | 13 个网页、4 个附件，覆盖 10 个主题 |
| 未放行记录 | 10,462 | 保留在复核队列，禁止自动导入正式回答链路 |
| 原文证据用例 | 40 / 40 | 核对原文片段、事实词、来源哈希和适用范围 |
| 范围反例 | 120 / 120 | 阻止跨学校、跨期及未经验证的“最新政策”请求 |
| 具体错误样例隔离 | 5 / 5 | 教师事项误分类、医保年份冲突等 |
| 白名单重复正文 / 未审核父文附件 | 0 / 0 | 附件、父文和交付包成员保持一致 |
| Python 单元测试 | 37 / 37 | 包括篡改检测、准入、时效和失败凭证失效 |

原批次的 738 条 `READY` 仅代表技术门禁结果；V4 的 17 条采用更严格的证据与适用范围条件，两者不能当作同口径通过率比较，也不能将 40 条核对通过宣称为“全库准确率 100%”。同一审核计划产生的证据用例不是独立盲测。

PII 重扫结果为 CLEAR 9,988、HIT 228、NOT_SCANNED 263；扫描命中较原批次增加 13 条。HIT 和未扫描均不放行。仅一份政策解释附件的历史风险标记经全文证据审核及 CLEAR 重扫后解除，原批次和单条解除记录均保留；没有批量清除风险标记。

## 修正和隔离的关键问题

1. **推免目录按年级适用，不能按上传时间取最新。** 官方解释附件写明：“2023级学生可参照2022版推免期刊目录或2026版推免期刊目录；从2024级开始，以2026版推免期刊目录为准。”两版目录已关联同一版本系列并记录适用规则。目录正文仍待完整表格结构审核，未放行，不能据此回答具体期刊是否入选。
2. **保留来源冲突，不猜测修正。** 医保通知所列 2027 年待遇与 2027 年 9—12 月缴费期存在需要确认的时间关系；论文规则写“小于 20%”及“大于 20%”，未明确恰好 20%。两项均记录证据并隔离。
3. **纠正业务对象。** 教师项目申报、优秀导师推荐、领导慰问信等，不再因“推荐”“学生”等词直接进入学生政策分类。
4. **附件不能绕过父文审核。** 4 个放行附件全部绑定已审核父通知，交付包只含明确审核的关系与成员。`attachment_count` 表示交付白名单内的附件数，可能少于原页面附件总数。
5. **历史材料严格限期。** 旧学年综测、年度转专业和考试通知均携带适用期。所有记录的 `is_current` 保持未知，日期最新不等于政策现行。

## 文件与使用约定

本地完整批次：`asku-knowledge/data/clean-batches/20260905-v4/`。

- `catalog.sqlite`、`normalized/`：完整记录及清洗正文，包含隔离数据，仅供治理复核。
- `reviewed_evidence.jsonl`：40 条逐字证据单元，带来源、学校、主题、适用期与范围说明；优先用于受控离线试验。
- `ready_documents.jsonl`：17 份审核文档；`ready_attachments.jsonl` 是其中 4 份附件的关系元数据，不能再当作 4 份独立正文重复导入。
- `ready_relations.jsonl`、`ready_bundles.jsonl`：仅包含已审核父子关系和成员。
- `context_decisions.jsonl`：4 条版本适用或冲突处置，不能当作正文准入凭证。
- `review_queue.jsonl`：10,462 条未放行记录及原因。不得批量改为 `ACCEPTED`。
- `review-ledger.json`、`validation.json`、`delivery-contract.json`：审核依据、全量校验与范围合同。只有 `CANARY_READY` 合同和 `PASSED` 验收同时有效时，才能使用离线白名单。

独立验收目录：`asku-knowledge/reports/quality-delivery-20260905/`，包括 `acceptance.json`、`evidence-cases.json` 和审核账本。

交付 ZIP 只包含白名单正文、证据与验收材料，不包含隔离正文。`documents.jsonl` 使用包内相对路径；`package-manifest.json` 记录所有文件 SHA-256。原始批次和原始附件均未覆盖。数据与报告按仓库忽略规则保留本地，GitHub 同步清洗代码、审核计划与说明。

## 复现

在仓库根目录执行。`$rawRoot` 指向原始采集文件位置；源批次和原始文件必须存在，审核计划绑定原始批次哈希。重放必须使用新的输出目录。

```powershell
$rawRoot = 'D:/Desktop/datasets/asku-knowledge/data/whut/raw'
$sourceBatch = 'asku-knowledge/data/clean-batches/20260904-v3b'
$targetBatch = 'asku-knowledge/data/clean-batches/20260905-v4-replay'
$reportDir = 'asku-knowledge/reports/quality-delivery-20260905-replay'
python asku-knowledge/scripts/quality_batch.py build --source $sourceBatch --output $targetBatch --school config/schools/whut.yaml --taxonomy asku-knowledge/config/taxonomy.yaml --raw-root $rawRoot
python asku-knowledge/scripts/prepare_quality_review.py --source $sourceBatch --plan asku-knowledge/config/quality-review-whut-v4.yaml --output $reportDir
python asku-knowledge/scripts/quality_batch.py release --batch $targetBatch --reviews "$reportDir/review-ledger.json" --school config/schools/whut.yaml --taxonomy asku-knowledge/config/taxonomy.yaml --raw-root $rawRoot
python asku-knowledge/scripts/verify_quality_delivery.py --batch $targetBatch --cases "$reportDir/evidence-cases.json" --output $reportDir
python asku-knowledge/scripts/package_quality_delivery.py --batch $targetBatch --report $reportDir --output "$reportDir/whut-quality-v4.zip"
$env:PYTHONPATH = (Resolve-Path asku-knowledge).Path
python -m unittest discover -s asku-knowledge/tests -v
```

每一步须成功才执行下一步。`release` 会撤销旧交付状态；验收异常或失败会写入 `FAILED`，防止旧成功凭证继续被误用。

## 正式上线仍需通过的门槛

`asku/scoped_evidence.py` 是离线范围门禁，尚未接入 Go 问答链路。生产适配必须校验学校、主题、适用期、正文哈希和准入状态，缺少范围默认拒答；不得直接读取全量 SQLite 或把所有 `PARSED` 文档导入。

上线前仍需完成：官网有效性与撤销/替代关系核验；高频问题覆盖验收；真实检索、引用和生成回答的独立评测；剩余附件表格/OCR 结构审核；人工确认来源本身的政策冲突。当前未开启生产知识提供方，也未导入 WeKnora。

后续扩充应按高频主题逐批审核并补充原文证据、反例和适用范围，保持“无法证实就不放行”。规则建议数量和白名单数量都不应作为准确率指标。
