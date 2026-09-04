# AskU 课表：iwut Feature Parity Audit 与定向补强

审计日期：2026-09-04。以下矩阵在修改生产代码前完成。

- AskU：已 fetch 并核对本地 main 与 origin/main，基线 `b8cf6b0f2d2260b685e0fbf4694cecac9c1049ba`。
- iwut：只读研究最新 main `b4681b0c514eaf044e1d544beed872c7ff3d2db5`，源码放在仓库外的临时目录。
- 依据为上游源码行为，**不是学校当前真实账号的响应样本或成功率证明**。
- 独立实现，仅参考协议、字段与业务事实；不引入 iwut 源码、UI、样式或资产。

## 1. 修改前 Feature Parity Matrix

| 能力 | iwut | AskU main | 当前差异 | V1 是否需要 | 操作 |
| --- | --- | --- | --- | --- | --- |
| CAS 官方登录 | 支持本科 CAS WebView | 同一入口，用户手动登录 | iwut 可绑定账号自动填充；AskU 不接触凭据 | 是 | 保留手动官方认证 |
| 验证码 / 二次认证 | 绑定登录另有短信处理 | 在学校原页面完成 | 不建设 AskU 密码或验证码表单 | 是 | 保留，真机待验 |
| CAS 返回后自动读取 | homeapp 加载后延迟 1.5 秒注入 | 相同行为 | 无已证实的协议差异 | 是 | 保留 |
| 切换本科角色 | POST changeAppRole，固定 appRole | 地址、角色、方法相同 | 无 | 是 | 保留 |
| 当前用户 | currentUser.datas.userId | 同一字段，仅在 WebView 内使用 | iwut 有调试桥输出；AskU 只返回规范化课程 | 是 | 保留隐私边界 |
| 当前学期 | welcomeInfo.xnxqdm | 相同，校验 YYYY-YYYY-[1-3] | 未发现本科导入需要学期列表 | 是 | 保留 |
| 本科课表 API | POST cxxskcb，XH/XNXQDM | 同 Method / Header / Body | 无 | 是 | 保留 |
| 校历 API | POST cxxljc，XN/XQ，首行 XQKSRQ | 同请求，额外严格日期校验 | AskU 缺校历则拒绝整批导入 | 是 | 保留，避免错误日期 |
| 学期 / 校历一起更新 | 分别更新课程及非空 termStart | 单个快照保存学期、日期和课程 | AskU 不会混用旧校历和新课表 | 是 | 保留 |
| 课程字段 | KCM/SKJS/JASMC/SKXQ/KSJC/JSJC/SKZC | 映射为统一 Course | 原始字段不进入 UI | 是 | 保留 |
| 数字字符串 | Number 转换后要求整数 | 仅无空白十进制字符串 | AskU 会拒绝带边缘空白的数字 | 是 | 仅放宽空白，仍拒绝小数/指数/布尔 |
| 长文本 | trim，不静默截断 | trim 后截断为 200 字符 | 不同课程可能被截成相同记录 | 是 | 超限行明确跳过，避免错误合并 |
| 周次位图 | 连续 1 拆成多个范围 | 精确 weeks[] | AskU 保留整条安排，支持到 64 周；iwut 上限 20 | 是 | 保留 |
| 单 / 双 / 不连续周 | 通过范围表达 | 精确集合表达 | 数据语义一致 | 是 | 保留并补显式场景测试 |
| 周末 / 缺教师 / 缺地点 | 支持 | 支持；详情地点措辞与网格不同 | 统一缺地点提示即可 | 是 | 小改文案 |
| 完全重复安排 | dedupe 主要针对独立实验课覆盖普通导入；本科重复并未普遍去重 | 全字段签名去重 | AskU 已满足要求 | 是 | 保留，补不同教师/地点/周次测试 |
| 部分坏行 / 全坏行 | 过滤，UI 拒绝全空结果 | 跳过计数，全坏拒绝 | AskU 有明确反馈 | 是 | 保留 |
| 合法空 rows | 视为无本学期数据，拒绝 | 可保存为空课表 | 用户明确要求合法空课表 | 是 | 保留 AskU 行为 |
| 冲突课 | 周视图及详情有冲突处理 | 独立分栏、点击详情 | 无需替换现有布局 | 是 | 保留，补 3–5 / 4–6 测试 |
| 调课 | 重导入反映 API 当前安排；未发现本科专门调课接口 | 重导入整批替换 | 未提供校方节假日调课覆盖表 | 是 | 保留，不猜测调休日 |
| 独立实验课 | 独立导入来源，覆盖普通课重叠周 | 仅本科 API 返回的课程 | 独立实验系统不属于本轮四接口链路 | 否 | 不新增，报告覆盖限制 |
| HTTP / 网络异常 | 通用导入失败 | NETWORK/SYSTEM/AUTH/TIMEOUT/FORMAT | AskU 对最终登录地址识别不完整 | 是 | 补最终 URL、明确错误信号与故障测试 |
| 重定向登录失效 | 通用错误 / 自动登录流程 | 仅 redirected 且路径含 /tpass/ | 应识别配置的 CAS 登录页及学校已知登录路径 | 是 | 定向修正，不扫描密码表单 |
| 本地持久化 | Zustand + MMKV | Context + AsyncStorage | 不必换库；AskU save 未检查 load 的 2M 上限 | 是 | 写入前同限校验 |
| 重启恢复 | 持久化 store 恢复 | repository hydration | 已有基本测试，缺导入全过程连接验证 | 是 | 补生命周期测试 |
| 失败保留旧课表 | 正常取消/请求失败不更新 | 校验且落盘后再发布 | 需补全七种失败类型与延迟响应测试 | 是 | 保留架构，补测试 |
| 登录恢复 / Cookie | 绑定用户可复用；手动导入退出清 Cookie | 每次临时 WebView 登录，无账号绑定 | Android incognito 不等于退出已清全部 OS Cookie | 是 | 保留无凭据设计，真机隐私验收列为门槛 |
| 周导航 / 回本周 | 周导航、回当前周等 | 手势、上/下周、点周标题回本周 | AskU 本周范围默认 20 周，误称已结束 | 是 | 改为超出浏览范围提示 |
| 日期 / 今天 | UTC+8 校历 | 学校时区日期，按周一对齐 | 非周一开学锚点仍须真实校历确认 | 是 | 保留日期模型，列入验收 |
| 当前学期 / 更新时间 | 提供课表管理相关信息 | 学期在底部，顶部有更新时间 | 学期可移至顶部更清楚 | 是 | 小改信息层级 |
| 当前时间线 / 节次钟点 | 内置 SECTION_TIMES | 只显示节次 | 上游静态表并非本轮获得的官方时间配置 | 否 | 不猜钟点、不加时间线 |
| 历史学期管理 | 本科导入同样取当前学期 | 当前学期 | 未找到需要历史学期列表的证据 | 否 | 不扩展 |
| 手工课 / 研究生 / Widget / 成绩等 | 上游有更广功能 | 不纳入本模块 | 超出武汉理工本科 V1 | 否 | 不做 |

## 2. 协议事实与证据

上游固定提交链接：

- [本科导入](https://github.com/TokenTeam/iwut/blob/b4681b0c514eaf044e1d544beed872c7ff3d2db5/services/course-import/bachelor-import.ts)
- [导入页生命周期](https://github.com/TokenTeam/iwut/blob/b4681b0c514eaf044e1d544beed872c7ff3d2db5/app/browser/course.tsx)
- [Normalizer](https://github.com/TokenTeam/iwut/blob/b4681b0c514eaf044e1d544beed872c7ff3d2db5/services/course-import/normalize.ts)
- [持久化 store](https://github.com/TokenTeam/iwut/blob/b4681b0c514eaf044e1d544beed872c7ff3d2db5/store/course.ts)、[Storage](https://github.com/TokenTeam/iwut/blob/b4681b0c514eaf044e1d544beed872c7ff3d2db5/lib/storage.ts)
- [去重行为](https://github.com/TokenTeam/iwut/blob/b4681b0c514eaf044e1d544beed872c7ff3d2db5/lib/course-dedupe.ts)、[周次](https://github.com/TokenTeam/iwut/blob/b4681b0c514eaf044e1d544beed872c7ff3d2db5/lib/course-weeks.ts)、[日期](https://github.com/TokenTeam/iwut/blob/b4681b0c514eaf044e1d544beed872c7ff3d2db5/lib/date.ts)

学校 origin 为 `https://jwxt.whut.edu.cn`。请求在学校主 frame 内依次执行，均 `credentials: include`，不由 AskU 后端代理。

| 顺序 | 路径 | 方法 / Header / Body | 取值 |
| --- | --- | --- | --- |
| 入口 | `https://zhlgd.whut.edu.cn/tpass/login` | service 指向 `https://jwxt.whut.edu.cn/jwapp/sys/homeapp/index.do?forceCas=1` | 用户本人在官方页认证 |
| 1 | `/jwapp/sys/homeapp/api/home/changeAppRole.do?appRole=ef212c48c8f84be79acbd9d81b090f51` | POST；Content-Type: application/x-www-form-urlencoded；无 Body | 本科角色 |
| 2 | `/jwapp/sys/homeapp/api/home/currentUser.do` | GET；Fetch-Api: true | datas.userId；datas.welcomeInfo.xnxqdm |
| 3 | `/jwapp/sys/kcbcxby/modules/xskcb/cxxskcb.do` | POST；Content-Type: application/x-www-form-urlencoded; charset=UTF-8；X-Requested-With: XMLHttpRequest；Accept: application/json, text/javascript, */*; q=0.01；Body XH、XNXQDM，URL 编码 | datas.cxxskcb.rows |
| 4 | `/jwapp/sys/kcbcxby/modules/xskcb/cxxljc.do` | POST；同 form Content-Type 和 X-Requested-With；Body XN=学年、XQ=学期，URL 编码 | datas.cxxljc.rows[0].XQKSRQ |

没有发现可证实的替代学期字段、另一种本科响应包装或新增调课 API，因此不添加猜测性字段别名。上游的实验课去重是多来源的业务规则，不应套用到 AskU 同名不同教师/教室的本科安排。上游拆位图并不代表每个片段是一门新课，AskU 门数继续按课程名统计。

## 3. 本轮计划与范围

仅补响应失败分类、Parser 边界、缓存大小一致性、必要 UI 文案与验收测试。继续沿用 CourseProvider → JWAPP Adapter → Course Schema → 本地快照结构。

## 4. 本轮实际修改（逐文件）

以下路径相对于仓库根目录。

| 文件 | 原因 | 行为变化 |
| --- | --- | --- |
| `apps/mobile/src/features/timetable/providers/jwapp/jwapp-browser-entry.ts` | 登录重定向判断依赖 redirected 标志；角色返回体完全忽略 | 检查最终 URL 的 origin / 登录路径；数据端点严格 JSON；角色端点兼容空体及普通确认文本，检查 JSON 中明确错误并拒绝 HTML；不回传原始响应 |
| `apps/mobile/src/features/timetable/providers/jwapp/jwapp-parser.ts` | 空白数字过严、文本截断可能错误合并、显式失败与 rows 共存 | 数字先 trim，仍只接受十进制整数；超过 200 字符的课程/教师/地点使该行跳过，计入 skippedRows；全坏仍拒绝；顶层 code=401/403 映射 AUTH，success=false 映射 SYSTEM |
| `apps/mobile/src/features/timetable/providers/jwapp/jwapp-browser.generated.ts` | 入口/Normalizer 已变化 | 使用 AskU esbuild 脚本重建，自包含浏览器产物与源码一致 |
| `apps/mobile/src/features/timetable/components/CourseImportBrowser.tsx` | 主页面 401/403 误报 SYSTEM | 显示登录失效；子资源失败仍不终止导入 |
| `apps/mobile/src/features/timetable/store/timetable-repository.ts` | 可写入超过读取上限的快照 | 序列化后、setItem 前校验同一 2,000,000 字符上限；超限不会替换旧值 |
| `apps/mobile/src/features/timetable/components/WeekNavigator.tsx` | 导航上限不等于官方学期结束 | 超出范围时显示当前周数及浏览范围提示，不再断言学期结束 |
| `apps/mobile/src/features/timetable/components/CourseDetailSheet.tsx` | 空地点提示不一致 | 详情与课程块统一为「地点待定」 |
| `apps/mobile/src/features/timetable/screens/TimetableScreen.tsx` | 学期在底部不够醒目 | 学期移到顶部来源/更新时间之间，底部保留时区与滑动提示 |
| `apps/mobile/tests/timetable-bridge.test.ts` | 四接口细节、认证重定向与失败体测试不足 | 执行真实生成 bundle；核验四个完整路径、方法、Headers、Body；逐接口 CAS/显式失败短路；空 rows、坏 rows、缺校历、HTML、空/文本角色确认 |
| `apps/mobile/tests/timetable.test.ts` | 真实排列与缓存边界需明确覆盖 | 1–16 周、单双周、不连续 1/2/5/8/11、不同教师/地点/周次不合并、数字陷阱、长文本、3–5 与 4–6 冲突、超限写入前拒绝 |
| `apps/mobile/tests/timetable-lifecycle.test.ts` | 原测试未挂载真实页面/Store 生命周期 | 挂载生产 Screen、WebView 回调、Provider、Schema、Context Store、Repository；首次导入/重挂载/重导入、七种失败保留、迟到与旧 nonce、落盘前不发布、合法空表、无效 success payload、主资源认证与子资源故障、学期文案 |
| `apps/mobile/package.json` | 生命周期测试需要 React 挂载能力 | 仅新增锁定版本的开发依赖 react-test-renderer 与其类型；不新增生产依赖 |
| `apps/mobile/package-lock.json` | 可重复安装 | 锁定上述测试依赖 |
| `docs/features/timetable.md` | 原文是历史报告，不能当成本轮验收结果 | 增加日期说明与本报告入口，并指出当前 jwapp 路径 |
| `docs/features/timetable-iwut-parity-audit.md` | 需要完整可审查交付 | 修改前矩阵、固定提交协议依据、逐文件记录、验证层级、真机清单和剩余债务 |

`success=false` / `code=401/403`、带空白数字、超限文本等测试是**防御性合成案例**，不宣称它们是本轮捕获的真实校方返回。没有基于未经证实的消息文本推测账号状态。若校方在原地址返回无明确错误码的 HTML，仍安全归类 FORMAT；真实学校响应格式必须实测。

## 5. 已有实现为何保留

- 四个核心 API 和角色参数与最新 iwut 一致，不作无依据的协议修改。
- 精确 weeks[] 已完整表达连续、单双、不连续周；无需改成上游多个周次范围记录。
- Course Schema、Provider/School Adapter、Native nonce/来源校验和敏感字段剔除边界保留，UI 没有增加 JWAPP 字段。
- 完全相同记录去重、不同地点/教师/周次保留、冲突分栏和课程详情保持原实现。
- Context + AsyncStorage 的串行队列已经做到持久化成功才发布；用真实挂载测试补证据，不重写状态管理。
- 合法空课表按用户要求允许保存，全部坏行拒绝；不追随 iwut 的空表拒绝行为。
- 不增加绑定密码、自动填密码、学校 Session 恢复、历史学期、研究生、独立实验系统、成绩、Widget 或服务器存储。

## 6. 从 iwut 确认的业务 Case 与 UI 结论

1. 本科角色切换是用户/课表查询前置步骤；四接口顺序需要固定保留。
2. 学生身份和当前学期共同来自 currentUser；学期拆出 XN/XQ 才能查询对应校历。
3. homeapp 返回后仍留有 SPA 初始化间隔，上游使用 1.5 秒；AskU 保留，并设请求/页面超时。
4. 周次位图的连续 1 区间表达真实上课周，不连续或单双周不应填满范围中的缺口。
5. 多来源实验课的去重是覆盖指定周的业务逻辑，不能推广成「同名课合并」规则。
6. iwut 重导入保留手动/实验来源；AskU 当前只有一个本科活动快照，不引入该复杂性。
7. iwut 的节次钟点来自静态配置，本轮未取得校方权威时间配置，不采用其钟点或当前时间线。
8. 绑定自动登录与手动临时会话具有不同清理策略；AskU 只保留手动临时登录目标，并明确 Android 原生会话清理仍有缺口。

功能参考 iwut，但视觉仍为 AskU 自研。只调整学期信息位置、地点缺省措辞和超范围提示；继续白底、少量蓝色、稳定低饱和课程色、无新增阴影/渐变。不复制组件树、配色、资产或布局实现。

## 7. 自动与工程验证

本轮使用 Node `22.23.1`；系统默认 `20.18.0` 不满足仓库 engines。所有命令在 `apps/mobile` 执行，原生工程在独立英文目录生成。测试仅使用合成数据。

| 检查 | 本轮结果 |
| --- | --- |
| `npm run timetable:bundle` | 通过；生成产物已更新 |
| `npm run typecheck` | 通过 |
| `npm run lint` | 通过，0 error / warning；这是仓库规定的课表及接入文件 lint 范围 |
| `npm test` | 78/78 通过，原有 48 项保留；包含产物 freshness 检查 |
| `npm run export:android` | 最终代码 Hermes 导出通过，`index-8c01e1eb9fe6ac9e5cf6ed9d95791e86.hbc` |
| `npm run export:ios` | Hermes 导出通过；不等于 iOS 原生构建 |
| Android `app:assembleDebug` | 通过，8m 50s，253 tasks；JDK 17 / SDK 36 / x86_64；已安装模拟器运行 |

最终 iOS Hermes 产物为 `index-322ca721a89b7b1b0c4adf894fff752e.hbc`。Android 原生构建目录 `D:/Desktop/asku-parity-20260904` 是从当前 mobile 代码复制的独立英文目录，排除环境秘密文件、旧 native 工程和缓存后 `npm ci` → `expo prebuild --platform android --no-install` → `gradlew app:assembleDebug -PreactNativeArchitectures=x86_64`。没有改动主仓库的生成 native 工程。依赖的 deprecated / 跨磁盘 hard-link 回退警告未阻止构建。

调试 APK：`apps/mobile/artifacts/asku-parity-debug-x86_64.apk`。这是模拟器构建，运行依赖 Metro 8081，不是 ARM 真机发行包。工程、测试和最终导出日志在 `apps/mobile/artifacts/parity-*.log`，均为忽略的本地 QA 产物。没有复制参考项目到 AskU。验收后已停止本轮启动的 Metro 与无窗口模拟器；再次运行调试包需重启模拟器及 `npx expo start --port 8081 --lan`。

生命周期测试运行真实业务代码，仅替换平台宿主视图、Native Storage、Navigation focus、计时器及 WebView 外部事件；网络协议另由隔离 VM 执行真实生成 bundle 测试。**这属于代码验证，不是原生模拟器或学校真实账号 E2E。**

测试运行时 react-test-renderer 有官方 deprecation 提示，版本已与 React 19.2.3 对齐；未屏蔽该提示。安装依赖报告 17 个 moderate 漏洞，未执行会改动无关依赖的强制修复；不能宣称全依赖安全审计通过。

## 8. 真实账号验收状态与真机清单

当前没有用户提供并由本人操作的真实本科登录，因此真实账号 E2E **未验收**。不要求用户把密码、Cookie、学号发到聊天或终端。

| 验证层级 | 判定方式 |
| --- | --- |
| 代码验证 | 上述自动测试与导出，只使用合成响应 |
| 模拟器验证 | 本轮 API 36 模拟器、新生成 APK、当前源码 Metro：首次空态 → Mock 导入 → 周视图 → 滑动第 2 周 → 回本周 → 两门冲突课各自详情 → 重新导入取消 → 强制停止并重启恢复 → 断网重导入报 NETWORK 且旧表/更新时间保留，均通过 |
| 真实学校登录页验证 | 本轮在原生 WebView 打开武汉理工统一认证页；账号/密码留空。仅此步通过，不等于 CAS 返回、身份获取或课表导入成功 |
| 真实账号 E2E | Android 真机，由有权使用该账号的学生在学校页面登录，逐项核对数据后方可签收；当前未进行 |

本轮截图（仅 Mock 或空白登录页）：`apps/mobile/artifacts/parity-android-week.png`、`parity-android-detail.png`、`parity-android-login.png`、`parity-android-restored.png`、`parity-android-offline.png`。界面顶部学期、更新时间、冲突课程和详情可见。断网测试结束已恢复模拟器原先开启的 Wi-Fi / mobile data。最初旧 APK / 开发服务端口不匹配导致无法加载脚本；使用本轮 APK 和 8081 Metro 后完成上述验证，没有把这次环境错误归为学校认证失败。

下面清单全部属于**待真实账号验收**，不能因为代码测试通过勾选：

- [ ] Android 真机型号 / Android 版本 / WebView 版本 / 构建版本已记录；使用真实武汉理工本科账号。
- [ ] 首次打开无课表 → 点击导入 → 显示官方统一认证域名；不经过 AskU 自建凭据输入框。
- [ ] 学生本人登录；验证码 / 二次认证若触发可正常完成。
- [ ] CAS 返回教务 homeapp，无必要页面被误拦截；自动进入读取流程。
- [ ] changeAppRole 成功；currentUser 识别的是当前本人，学号仅在学校 WebView 临时使用。
- [ ] 当前学期与学校课表一致，不需手填参数或切换历史学期。
- [ ] cxxskcb 成功；cxxljc 成功，开学日期与官方校历一致；若 XQKSRQ 非周一，确认第 1 周锚点语义。
- [ ] 课程门数（按名称）与安排条数分别核对；抽查课程名、教师、地点、星期、开始/结束节次。
- [ ] 连续周、单双周、不连续周、周六、周日都与教务一致；同名不同教师/地点/周次未错误合并。
- [ ] 缺教师仍显示，缺地点显示「地点待定」；部分坏行提示与实际跳过数量一致。
- [ ] 当前日期、今天标识、当前周与学校时区一致；手机更换时区仍一致。
- [ ] 上/下周、左右滑、回本周正常；3–5 和 4–6 冲突课分别可见、可点详情。
- [ ] 成功后关闭 WebView；学期和最后更新时间可见，数据已保存。
- [ ] 正常关闭及强制停止 App 后重启能恢复；使用含 JS bundle 的真机包离线启动可看课表。
- [ ] 重新导入成功后课程、学期、校历一起更新。
- [ ] 取消 / 返回键 / 断网 / 超时 / 登录失效后，旧课表、校历和更新时间均保持；再启动仍恢复旧快照。
- [ ] 取消后迟到回调、连续点击、进后台再恢复不导致重复导入或错误覆盖。
- [ ] 合法空 rows 可以保存；全坏格式、缺校历、异常响应不能替换旧课表。
- [ ] 清除确认 / 取消清除 / 清除后重启符合预期，学校数据不受影响。
- [ ] 使用脱敏检查确认 Native Bridge / AsyncStorage / 日志不含学号、密码、Cookie、验证码或原始学校响应；AskU 后端未收到课表。
- [ ] 检查 Android 学校 WebView 会话在成功 / 取消 / 强杀后的持久化与清理行为；没有解决之前，不签收「学校 Cookie 不落盘」。

## 9. 剩余技术债与试点门槛

| 优先级 | 项目 | 边界 / 下一步 |
| --- | --- | --- |
| P0 | 真实本科账号端到端未验收 | 当前 CAS/验证码、实际响应、课程数/日期/单双周准确性必须真机核对，无法通过合成测试证明真实导入成功率 |
| P0 | Android 学校会话隐私 | react-native-webview 13.16.1 的 incognito 在进入时 removeAllCookies，destroy 不提供对应退出 Cookie 清理；AskU 不主动读取/保存凭据，但不能保证操作系统 WebView Cookie 从不落盘。需要独立原生临时会话/生命周期清理方案与真机退出/强杀审查；本轮不伪造通过 |
| P1 | 实际响应与非周一校历锚点 | 校方错误包装、SSO 域名变更、XQKSRQ 日期语义需脱敏样本确认；同 URL 的 HTML 错误当前为 FORMAT，不靠猜测文本判认证 |
| P1 | 极端设备存储故障 | 已验证写入前校验、setItem 失败旧快照保留、串行发布。真实 AsyncStorage/SQLite 在系统杀进程和磁盘耗尽时的原子性仍需设备级验证；不是文件系统故障保证 |
| P1 | 原生平台覆盖 | Windows 无 Xcode；iOS 原生/真机未验收。Android x86_64 调试构建也不能代替 ARM 真机包与离线冷启动验收 |
| P2 | 导航范围与日期配置 | 默认至少浏览 20 周，按课程扩展到最多 64；没有官方学期结束日，已去掉错误结束断言。不增加猜测钟点 |
| P2 | 测试挂载工具维护 | react-test-renderer 已 deprecated，未来 React 升级时评估替代并保持平台测试覆盖 |
| P2 | 依赖与旧文档 | npm 安装报告的 17 moderate 需单独兼容性治理；旧 V1 报告只作历史，已增加新报告入口 |

本轮交付是审计、定向修补和代码层验证；P0 真机验收及 Android 会话隐私门槛未关闭之前，不能宣布「真实课表功能已完成验收」。
