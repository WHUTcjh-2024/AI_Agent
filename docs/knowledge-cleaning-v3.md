# 本地清洗、准入与附件来源修复

本流程将旧清洗快照和原始文件重建为独立暂存批次。SQLite 仅用于离线审计和批次交付，生产状态仍由 PostgreSQL 管理。原数据库及原件不修改，不自动抓取、不上传 WeKnora、不启用知识 Provider。

## 行为变化

- `content_chars` 只统计实际提取文本统一空白后的字符数；二进制、空白、失败及部分解析分别记录，不能把文件存在算作解析成功。
- 正文使用 UTF-8 字节写入和校验，保留解析器实际产生的换行；Windows 不会额外转换 OCR 的 CRLF，校验值对应真实文件字节。
- 统一 `asku.admission.evaluate` 为统计和导出的准入入口，要求 ACCEPTED、有效分类、官方来源、激活来源、PARSED、正文长度、同一内容哈希上的 PII CLEAR、非重复及完整附件来源。`other`、待审、OCR 未复核、失败附件均不可准入。
- PDF 保留页码及可提取表格；逐页判断 OCR 需要。DOCX 保留段落和表格顺序，Excel 保留工作表/行列，旧 DOC 使用真正的格式解析器，ZIP/RAR 逐成员解析。密码、时间/页数/解压预算及不支持格式均产生明确失败或待审记录，不能报告完整成功。
- OCR 输出固定为 REVIEW；此前 PII 隔离记录不会因一次机器复筛未命中自动放行。旧质量分未重算，不应作为本批次新的质量评分。
- 从全部本地抓取网页的实际附件链接匹配完整 URL，保留查询参数，不按文件名猜父文档。必要时恢复原索引缺少的父网页，新增父网页保持 REVIEW。每条关系均保存证据 URL。
- HTML 优先 TRS 正文容器，正文字符数排除导航。发布日期在清除页面头部前提取，仅采用明确发布提示或元数据，不把“会议时间”等正文活动日期作为发布日期。不能证明的日期设为未知；附件可继承有证据的父通知日期。
- 原地修改旧库的 `--all` / `--fix` 脚本已退役，兼容入口要求提供新批次参数，缺少参数直接失败。

## 安装与运行

在仓库根目录安装 Python 依赖：

```powershell
python -m pip install -e 'asku-knowledge[cleaning]'
```

中文 OCR 需要 Tesseract 的 `chi_sim` 和 `eng` 语言包。RAR 预处理使用 7-Zip；旧 DOC 使用已有文档解析镜像中的 antiword/LibreOffice，须在网络关闭、根文件系统只读、输入挂载只读的独立容器运行。解析程序不会从网络获取文档。

```powershell
python asku-knowledge/scripts/prepare_rar_archives.py `
  --input 'D:/Desktop/datasets/asku-knowledge/data/whut/raw/attachments' `
  --output asku-knowledge/data/archive-cache `
  --sevenzip 'C:/Program Files/7-Zip/7z.exe'
```

将 `scripts/extract_legacy_docs.py` 挂入隔离容器，先对原始附件目录运行，再对 `archive-cache` 运行；输出使用文件 SHA256 命名，存放到 `legacy-text-cache`。容器内调用：

```text
python3 /tools/extract_legacy_docs.py --input /input --output /output
```

创建新批次（输出目录必须尚不存在）：

```powershell
python asku-knowledge/scripts/clean_local_batch.py `
  --source-db asku-knowledge/data/asku_clean_v2.db `
  --crawler-dump tmp/data-audit/crawler-data.sql `
  --school config/schools/whut.yaml `
  --taxonomy asku-knowledge/config/taxonomy.yaml `
  --raw-root 'D:/Desktop/datasets/asku-knowledge/data/whut/raw' `
  --output asku-knowledge/data/clean-batches/new-batch `
  --legacy-dir asku-knowledge/data/legacy-text-cache `
  --archive-dir asku-knowledge/data/archive-cache `
  --tesseract 'C:/Program Files/Tesseract-OCR/tesseract.exe' `
  --workers 6

python asku-knowledge/scripts/verify_clean_batch.py `
  --batch asku-knowledge/data/clean-batches/new-batch `
  --school config/schools/whut.yaml `
  --taxonomy asku-knowledge/config/taxonomy.yaml
```

批次中断后保留现场，使用新的输出目录重跑；解析缓存按原件哈希、来源 URL、文档 ID 和解析配置隔离。修改解析器或启用 OCR 会改变缓存配置。批次为 COMPLETE 且最终校验 PASSED 才可供后续 Canary 使用。

## 交付文件

- `catalog.sqlite`：完整离线治理结果及原记录状态。
- `normalized/`：重新提取的正文，包含需要人工复核的文本。
- `ready_documents.jsonl`、`ready_attachments.jsonl`：统一技术准入结果，仍须 Canary。
- `bundles.jsonl`、`relations.jsonl`：父子关系和来源证据。
- `review_queue.jsonl`：未通过记录的 ID 和原因，不复制敏感正文。
- `summary.json`：格式、解析状态、OCR、关系、准入原因汇总。
- `validation.json`：逐文件哈希、字符数、准入一致性及关系完整性验证结果。

最终校验先复核文件和准入记录，再同步附件侧准入记录并生成导出；正文或准入凭据被修改、路径越出当前批次或关系悬空时直接失败。每次重验先将旧凭据置为 VALIDATING，失败时记录 FAILED，避免继续使用旧 PASSED 结果。上述目录含资料正文，按现有 `.gitignore` 留在本机；GitHub 只提交代码、测试及脱敏汇总。

## 后端与晋升

新增 backend migration `008_cleaning_admission.sql` 与 knowledge migration `005_cleaning_admission.sql`。迁移只新增字段，旧记录默认 BLOCKED/PENDING，不会批量批准旧数据。

后端读取 WeKnora 命中时校验文档与附件的解析、PII、内容哈希及准入记录；旧缓存命中也会再经过目录校验。引用优先使用文档详情 URL。重新解析的离线映射全部设为 `PENDING_REVALIDATION`，不得继承旧 IMPORTED 状态。

答案缓存增加准入版本命名空间，使本次修复前生成的答案不可再命中；无需删除 Redis 数据。后续每次知识晋升仍应更新学校的 `knowledge_version`，让既有答案按原有学校版本机制失效。

本改动不实现生产全量 importer，也不绕过现有 Canary 要求。后续晋升应先完成来源、政策有效性和 PII 人工复核，再验证 10 HTML、10 PDF、5 DOCX、5 Excel、3 Bundle 以及重点主题和历史版本系列。技术 READY 不能代替内容准确率或政策现行性验收。

## 测试

```powershell
cd asku-knowledge
python -m unittest discover -s tests -v
cd ../backend
go test ./internal/knowledge ./internal/store ./internal/integration
```

真实 PostgreSQL 准入回归需要专用 `asku_eval` PostgreSQL/Redis；设置评测环境后运行 `go test ./internal/integration -run TestCleaningReceipt -count=1 -v`。测试包括旧标记伪通过、哈希过期、附件关系未确认、原件不变，以及正文篡改后校验失败。

## 2026-09-04 本地执行结果

批次 `20260904-v3b` 使用原有 9,470 条记录与本地原件，检查 13,168 个来源网页，恢复 1,009 个缺失父网页记录，共处理 10,479 条。新增父网页均保持待审；本次没有执行网络爬取或生产知识导入。

| 指标 | 结果 |
| --- | ---: |
| 完整解析 | 9,316 条 |
| 解析待审 | 1,037 条 |
| 解析失败 / 空文本 | 42 / 84 条 |
| 附件取得非空文本 | 3,483 / 3,609 |
| 附件完整解析 | 3,088 / 3,609 |
| 附件来源关系已恢复 | 3,604 / 3,609 |
| 来源关系 / Bundle | 3,627 / 1,682 |
| 技术 READY | 738 条，其中附件 240 条 |
| OCR 处理页数 | 2,809 页，输出保持待审 |

技术 READY 使用统一准入规则，不沿用旧 `rag_eligible` 统计，也不等于生产可回答数量。待审原因可重叠，不能将各原因计数相加当作文档总数。旧质量分未重算。

在相同的 9,470 条原记录范围内，旧 `rag_eligible` 标记从 3,076 条重算为 738 条；其中“未 ACCEPTED 却标为可用”的 1,139 条已归零。全部 10,479 条原件哈希与解析时记录一致，正文、准入凭据、关系和导出校验均通过。最终 738 条仍需后续 Canary，不自动上线。

最后 5 个未关联附件的链接存在于 4 份本地 HTML 中，但这些 HTML 未出现在当前抓取目录和清洗快照中，页面也没有 canonical URL。应补齐这 4 页的来源 URL 证据，不能按本地文件名猜关系。其余失败或待审资料优先处理 XML 旧版表格、空白表单、复杂压缩包、OCR、分类及 PII 人工复核；没有全量重爬的必要。

脱敏的完整前后对账、校验结果和输入指纹见 [清洗结果 JSON](knowledge-cleaning-v3-results.json)。正文、SQLite、解析缓存和逐条待审队列保存在本地 `asku-knowledge/data/clean-batches/20260904-v3b/`，未提交到 GitHub。
