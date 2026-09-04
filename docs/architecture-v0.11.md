# Architecture V0.11：Router Robustness + School Portability

实现日期：2026-09-04。当前仍是单 Active School 部署，未引入 LLM Router、工作流框架或多租户 UI。

## 结果

确定性 Question Analyzer 已替换介绍词子串路由。原始 Message 和 Generation 问题保持原文；Knowledge/Web 只检索 EffectiveQuestion。Hybrid 的并行检索、Web 时效权威、Knowledge 失败降级、Backend Citation、Run/SSE 和数据库职责保持原边界。

学校配置统一到根目录；School Registry、Knowledge Gateway、Web Gateway、Answer Cache 已用同一组测试代码加载 `testu`。同一个编译好的 API 二进制也分别以 WHUT 和 testu 配置启动并运行成功。testu 未接受 mock Provider 返回的 WHUT 来源。

## Router Before / After

固定当前年份为 2026；Before 根据修改前 PolicyRouter/needsFreshWebSearch 实现核对。

| 问题 | Before | After | route reason |
|---|---|---|---|
| 你好 | Controlled（介绍词子串） | Controlled | pure_social |
| 你好，请问四六级什么时候报名 | Controlled（错误） | Hybrid | freshness_schedule_question |
| 您好，奖学金怎么评 | Knowledge | Knowledge | stable_campus_knowledge |
| 你好像知道今年什么时候报名 | Controlled（错误） | Hybrid，原文保留 | freshness_current_marker |
| 2024 年转专业政策 | Knowledge，无历史区分 | Historical / Knowledge | historical_explicit_year |
| 2024 年政策现在还有效吗 | Knowledge（错误） | Hybrid | freshness_status_question |
| hello，图书馆几点关门 | Controlled（错误） | Hybrid | freshness_schedule_question |
| 2026-2027 学年校历 | Hybrid | Hybrid，明确年份判断 | freshness_current_year |

分析顺序为：空白规范化 → 明确边界的寒暄/礼貌前缀剥离 → 完整 Social/Intro 匹配 → 当前状态/相对时间 → 明确年份 → 时间安排 → 动态信息 → 稳定知识。

`你好像`、`你好不好用` 不被剥离。`你是谁不重要，…` 不满足完整产品介绍匹配。`四六级一般怎么报名` 作为一般流程背景走 Knowledge；本轮报名、当前状态、时间、截止等走 Hybrid。历史年份只在没有现状信号时允许 Knowledge。

可观察字段保留在既有 Run Events/结构化日志中：route、reason/routeReason、retrievalMode、degradedCapabilities、cacheHit；没有新建观测表。缓存命中仍维持兼容的 cache route，metadata/日志保留原知识路由原因。

## 换校操作

1. 新增 `config/schools/{school_id}.yaml`：学校 ID/名称、allowed_domains、可选 discovery/禁止域与路径、seeds、独立 Knowledge Base ID、非空 knowledge_version。
2. 新增 `asku-knowledge/config/sources/{school_id}.yaml`：同一 school_id 的官方 Source Registry；active source/base_url/seeds 必须在允许范围内且不触发禁止规则。
3. 导入该校知识数据到学校自己的 WeKnora KB，并配置既有 WeKnora 连接参数。启用 Backend `ASKU_KNOWLEDGE_PROVIDER=weknora` 或 Pipeline `weknora.enabled: true` 时，官方 KB ID 必须存在。
4. 两端设置 `ASKU_SCHOOL_CONFIG` 为该 YAML 的绝对路径并重启。容器中设为 `/config/schools/{school_id}.yaml`，沿用根学校目录只读挂载。
5. 发布该校 Mobile 时，同样设置此环境变量并运行 `npm run timetable:bundle` 后构建。未填写 `mobile_timetable` 时默认禁用教务导入。生成物只包含公开的 Mobile 字段，不携带 KB ID 或服务端凭证。

不需要修改 Go Agent、Knowledge/WeKnora Gateway、WebSearch Gateway、Python Fetcher/Normalizer 或复制 Backend/Crawler。课表导入属于另一种外部系统适配：同协议可以换配置；不同教务协议仍需新增对应 Provider，不影响问答服务移植。

例：从仓库根目录执行 `$env:ASKU_SCHOOL_CONFIG=(Resolve-Path config/schools/newschool.yaml).Path`，之后运行 Backend/Pipeline；不要同时指定另一个 school_id。Python Loader 对冲突直接报错，避免默默继承另一学校。

## 修改文件

以下只描述本任务的修改；仓库原有未提交课表、导航、数据维护脚本等工作保留，没有创建提交或覆盖回退。

| 文件 | 修改原因 |
|---|---|
| `backend/internal/agent/question_analysis.go` | 新增独立的输入分析、前缀边界处理、完整介绍匹配、分层 Freshness 和可注入时钟。 |
| `backend/internal/agent/policy_router.go` | 只把 QuestionProfile 映射为 Capability Plan；两个检索请求使用 EffectiveQuestion。 |
| `backend/internal/agent/agent.go` | MockRouter 的时效判断复用 Analyzer，避免维护第二份规则；联调 fixture 保留。 |
| `backend/internal/agent/orchestrator.go` | 补齐结构化 route_reason/cache_hit/degraded_capabilities，缓存命中时保留原路由原因。 |
| `backend/internal/agent/policy_router_test.go` | 同步稳定的 integration_probe reason；保留旧工程评测入口。 |
| `backend/internal/agent/question_analysis_test.go` | 执行 84 条表驱动评测，并验证前缀语义、空输入和取消。 |
| `backend/internal/agent/retrieval_query_test.go` | 验证原文/检索 query 分离、SchoolID 传递、动态问题绕过答案缓存及注入时钟。 |
| `backend/internal/config/config.go` | 删除核心代码中默认 whut 配置路径，使用显式 ASKU_SCHOOL_CONFIG。 |
| `backend/internal/school/registry.go` | 缺失 active school 路径时给出明确启动错误。 |
| `backend/internal/school/portability_test.go` | 实际 Loader、两个 Gateway 和共享缓存验证 whut/testu 两个单校部署。 |
| `backend/internal/knowledge/gateway.go` | 显式提供但越校的 URL 全被拒绝时，整条 Evidence 丢弃。 |
| `backend/internal/knowledge/gateway_test.go` | 断言越校 Evidence 整条拒绝，替代只清空 URL 的旧预期。 |
| `backend/internal/websearch/gateway.go` | 页面与摘录缓存加入 SchoolID；缓存页面和抓取返回 URL 重新校验范围。 |
| `backend/internal/websearch/cache_scope_test.go` | 搜索、页面、摘录三层缓存均不能跨学校命中。 |
| `backend/internal/evaluation/runner.go` | 把 routing.yaml 纳入评测输入哈希。 |
| `backend/internal/evaluation/runner_test.go` | 补齐评测器自身测试所需 routing fixture。 |
| `config/schools/whut.yaml` | 统一 Backend/Crawler 学校字段；纳入公开 Mobile 课表适配参数。 |
| `asku-knowledge/config/schools/whut.yaml（删除）` | 删除重复 School Config，阻止两端漂移。 |
| `asku-knowledge/config/sources.yaml（迁移）` | 通用来源表改为按学校命名。 |
| `asku-knowledge/config/sources/whut.yaml` | 保留原有 WHUT 来源内容，按 school_id 加载。 |
| `asku-knowledge/config/pipeline.yaml` | 移除学校专属知识库名称；明确 WeKnora enabled 开关。 |
| `asku-knowledge/asku/config.py` | 加载统一 School Config 和指定 Registry；在运行前校验 ID、域名、seeds、禁用域、版本与 KB。 |
| `asku-knowledge/asku/db.py` | 领取批次和统计 API 不再默认 whut，调用方必须给 school_id。 |
| `asku-knowledge/asku/url_utils.py` | 更新官方域名准入注释，与统一 allowlist 语义一致。 |
| `asku-knowledge/tests/test_contract.py` | 真实配置契约、跨学校来源拒绝、缺少 KB/版本、来源文件 ID 错配等测试。 |
| `asku-knowledge/tests/test_url_utils.py` | 显式选择统一根配置，取消隐式试点学校依赖。 |
| `asku-knowledge/tests/fixtures/sources/testu.yaml` | synthetic school 的测试来源注册表。 |
| `evals/fixtures/testu.yaml` | Go/Python 共用 synthetic school，具有独立域名、KB 和知识版本。 |
| `evals/routing.yaml` | 固定时间 2026-09-04；84 条 expected_route/expected_retrieval/expected_reason。 |
| `evals/engineering.yaml` | 加入路由回归、学校移植、原文/检索分离、缓存绕过及 Web 缓存范围门禁。 |
| `infrastructure/docker/docker-compose.yml` | 允许通过环境覆盖容器内学校配置路径。 |
| `apps/mobile/scripts/build-school-config.mjs` | 从统一 YAML 生成白名单内的公开适配字段，支持 --check 检测漂移。 |
| `apps/mobile/scripts/build-timetable-browser.mjs` | 先生成/校验学校参数，再构建 JWAPP 浏览器脚本。 |
| `apps/mobile/src/config/school-adapter.ts` | 为生成的配置定义稳定类型，使未启用课表的第二学校也可编译。 |
| `apps/mobile/src/config/school.generated.ts` | 自动生成的公开适配配置，不是第二份手工配置，不包含知识库 ID。 |
| `apps/mobile/src/screens/Home/HomeScreen.tsx` | 移除首页学校专属副标题；保留已有课表入口修改。 |
| `apps/mobile/src/screens/Profile/ProfileScreen.tsx` | 学校名称和介绍从当前 User 获取。 |
| `apps/mobile/src/features/timetable/screens/TimetableScreen.tsx` | 学校名称和课表可用性来自 Adapter Config；未配置学校不能发起导入。 |
| `apps/mobile/src/features/timetable/components/CourseImportBrowser.tsx` | 登录提示使用通用学校文案。 |
| `apps/mobile/src/features/timetable/providers/registry.ts` | 使用配置标签及 JWAPP Provider，不按学校 ID 写分支。 |
| `apps/mobile/src/features/timetable/store/timetable-repository.ts` | 本地课表缓存 key 加入学校。 |
| `apps/mobile/package.json` | 在已有用户依赖修改上加入构建配置解析所需 yaml。 |
| `apps/mobile/package-lock.json` | 同步 yaml 依赖锁定；原有用户依赖修改保留。 |
| `apps/mobile/tests/timetable.test.ts` | 同步协议模块重命名，保留已有断言。 |
| `apps/mobile/tests/timetable-bridge.test.ts` | 同步 JWAPP 导入名称，继续验证真实生成脚本和 Native 边界。 |
| `README.md` | 更新 V0.11 总览与报告入口。 |
| `backend/README.md` | 说明显式学校配置和确定性评测。 |
| `asku-knowledge/README.md` | 说明统一配置、Registry 选择和启动契约。 |
| `apps/mobile/README.md` | 说明构建时换校、课表协议限制和缓存迁移影响。 |
| `docs/architecture-v0.11.md` | 本次实施、验证、移植和审计报告。 |
| `apps/mobile/src/features/timetable/providers/jwapp/jwapp-auth.ts` | 登录 URL、origin 和导航允许域使用 School Adapter。 从原 whut 目录改名，协议以 JWAPP 命名。 |
| `apps/mobile/src/features/timetable/providers/jwapp/jwapp-parser.ts` | 课程 source/学校 ID/时区从配置获取，保留既有协议解析。 从原 whut 目录改名，协议以 JWAPP 命名。 |
| `apps/mobile/src/features/timetable/providers/jwapp/jwapp-course-provider.ts` | 校验配置的 school/provider/timezone；未启用时拒绝导入。 从原 whut 目录改名，协议以 JWAPP 命名。 |
| `apps/mobile/src/features/timetable/providers/jwapp/jwapp-browser-entry.ts` | 请求路径与学校 origin 来自公开配置。 从原 whut 目录改名，协议以 JWAPP 命名。 |
| `apps/mobile/src/features/timetable/providers/jwapp/jwapp-script.ts` | 同步浏览器 bundle 入口名称。 从原 whut 目录改名，协议以 JWAPP 命名。 |
| `apps/mobile/src/features/timetable/providers/jwapp/jwapp-browser.generated.ts` | 从共享配置和现有协议代码重新生成，不单独维护学校值。 从原 whut 目录改名，协议以 JWAPP 命名。 |

## 验证结果

| 验证 | 结果 |
|---|---|
| `go test -race ./...` | 通过；依赖外部服务的集成组另由下述 all eval 执行 |
| `go vet ./...` | 通过 |
| `python -m unittest discover -s tests -v` | 18 项通过，包含新增配置 Contract Tests |
| Router YAML | 84 条固定时钟案例通过，比较路由、检索能力和 reason |
| `npm run typecheck` | 通过；切换到 testu 生成配置后也通过，已恢复 WHUT 构建配置 |
| `npm test` | 48 项通过，包含实际浏览器 bundle/Native 校验 |
| `npm run lint` | 通过 |
| `npm run doctor` | 21/21 通过，使用本机已有 Node 22.23.1 |
| `npm run export:android` | 通过，输出 `apps/mobile/dist/android-bundle` |
| `scripts/eval.ps1 -Suite offline` | 工程检查通过 |
| `scripts/eval.ps1`（all） | 最终 39 passed / 0 failed / 0 skipped；9 项集成检查实际执行 |
| `scripts/smoke.ps1` | 通过：Hybrid、9 类 SSE 事件、2 条消息、3 条引用、Source Detail、Admin Hybrid 统计 |
| 同 API 二进制 testu 运行 | 通过：返回 Test University；混合请求拒绝 WHUT 来源，0 条越校引用；用户原始问题逐字持久化验证通过 |
| `docker compose ... config --quiet` | 通过；PostgreSQL/Redis 容器实际用于 all eval 与 smoke |
| Backend Docker image build | 未完成：Docker Hub 获取 `alpine:3.23` token/镜像元数据网络超时；没有更改或降级 Dockerfile 基础镜像。使用本机最新编译 API + 容器数据库完成等价功能验证 |

环境处理：系统默认 Node 20.18.0 低于项目要求，验证使用已安装的 Node 22.23.1；Python 补齐已声明的 psycopg binary 依赖；Docker Desktop 初次启动异常后恢复。未重置 Docker 数据。

本地证据（评测目录按项目惯例忽略，不纳入源码）：

- `evals/reports/v0.11-final/report.md`、`report.json`、`go-test.jsonl`：最终 all eval。
- `evals/reports/v0.11-smoke/smoke.txt`：真实 HTTP/SSE smoke。
- `evals/reports/v0.11-smoke/testu.txt`、`testu-sse.txt`：换配置运行与越校来源拒绝。
- `evals/reports/v0.11-smoke/original-message.txt`：原始问题持久化断言。
- `evals/reports/v0.11-smoke/hardcode-audit.txt`：全仓学校硬编码检索结果。

## Hardcode Audit

执行全仓 `rg -n "武汉理工|WHUT|whut|whut\.edu\.cn|jwc\.whut"` 并审查生产核心命中。

- 合法配置：根学校 YAML、按校 Source Registry；Mobile 的两个生成物只由这些配置生成，并由 --check 防漂移。
- 合法非生产样本：Go MockRouter 的联调学校来源、Web Mock Provider、Mobile mocks、tests/fixtures、evals 和文档。
- 修复的生产行为：Go 默认学校路径、Python 默认 school_id/来源文件、Mobile 页面学校名、课表登录/请求/来源元数据参数和本地缓存。
- Mobile 教务协议目录改为 `providers/jwapp`，不存在按 `schoolId === 'whut'` 分支的运行逻辑。Go Agent 中余下 WHUT 字面量全部位于显式 MockRouter 的联调 fixture。
- 既有/并行编辑的数据维护脚本（如 `asku-knowledge/scripts/repair_data_quality.py`）仍可能针对试点数据。这些一次性维护工具没有改写，不属于本次可移植的核心 Pipeline；换校时不能照搬这些维护命令。

## 真实剩余限制

1. Docker 后端镜像最终构建需要网络恢复后重跑。功能与集成链路已通过，不把它表述成镜像构建通过。
2. 真实校园知识/真实模型答案质量仍有 3 项 `blocked_data`，需要已准入数据和人工引用核验；工程 fixture 不能证明校园答案正确率。
3. 课表目前实现 JWAPP 协议。另一学校使用不同教务系统时需要新增 Provider，默认关闭导入；问答核心不受影响。
4. 旧版未分学校的本地课表缓存不会被自动沿用，需要重新导入。答案缓存结构兼容；Web 页面/摘录 key 变更只导致一次冷缓存。
5. Router 是确定性规则分类；试点后应依据既有 route reason 与反馈调整回归集。没有为分类增加 LLM 调用。
