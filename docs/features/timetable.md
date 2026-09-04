# AskU 课表 V1 开发报告

实现日期：2026-09-03。范围：武汉理工本科课表导入与 AskU 自有课表界面。未修改 AI 对话业务、知识库、RAG 或后端数据库。

## 功能

- 首页「课表」快捷入口，独立 Stack 页面，保留现有三个底部 Tab。
- 白底、蓝色轻强调、稳定的低饱和课程颜色；周一到周日全显示。
- 上/下周按钮、横向触摸滑动、点击周次回到当前周；默认按 Asia/Shanghai 显示日期。
- 精确 `weeks[]`，支持连续、单双和不连续周；无课周正常展示，不算导入错误。
- 同时段冲突课程分栏，不相互覆盖；点击任意课程查看教师、地点、节次、周次、来源。
- 本地缓存、最后更新时间、重新导入、带确认的本地清除；失败保留旧缓存。
- 未导入空状态以及明确标注的 Mock Provider 演示课表。演示不依赖 AskU 后端或教务在线；只有空状态提供演示入口，不能误覆盖已导入的真实课表。
- 网络失败、HTTP 错误、登录失效、超时、格式变化、存储失败、取消及异常外部跳转处理。

未实现可选的当前时间线；不猜测节次对应钟点。没有增加成绩、选课、分享、日历、Widget 等超范围功能。

## Architecture

```text
WHUT 官方页面（已登录的 WebView）
  → WHUTCourseProvider / 浏览器内 Normalizer
  → 统一 CourseImportResult
  → Native Zod 二次校验
  → TimetableStore（React Context）
  → TimetableRepository（AsyncStorage）
  → TimetableScreen / 统一课程组件

MockCourseProvider ───────────────→ 同一个 Store 与 UI
未来其他学校 CourseProvider ────→ 同一个 Store 与 UI
```

`CourseProvider.importCourses(signal)` 是可取消的导入端口；`CourseBrowser` 是 Provider 与 WebView UI 之间的交互端口。注册和来源名称在 `providers/registry.ts`，学校 URL、请求、原始字段只在 `providers/whut/`。UI 不知道 `KCM`、`SKZC`、`XNXQDM` 等字段。

保留项目现有 Context / AsyncStorage，不额外引入 Zustand、MMKV 或密码绑定体系。`TimetableRepository` 校验本地读取和写入，单 JSON key 保存完整快照；store 串行处理读写，持久化成功后才发布新状态。

浏览器入口单独由 esbuild 构建成自包含字符串，避免 `Function.toString()` 被 Metro/Babel/Hermes 压缩或函数捕获变量破坏。修改入口/解析器后运行 `npm run timetable:bundle`；生成代码来自 AskU 自身，须一并提交。`npm test` 会检查产物是否过期，并在隔离 VM 中实际执行这个生成产物。

## Authentication / API

1. 用户主动点击导入，生成随机 requestId，打开官方 `zhlgd.whut.edu.cn/tpass/login`。
2. 用户本人完成学校密码/验证码/短信步骤；AskU 不读取、填写或接管这些内容。
3. CAS 回到 `jwxt.whut.edu.cn/jwapp/sys/homeapp/`，页面加载后留 1.5 秒初始化时间，再注入独立脚本。
4. POST `changeAppRole.do` 切换本科角色；GET `currentUser.do`，取当前学生和当前学期。
5. POST `cxxskcb.do` 查询该学生当前学期，POST `cxxljc.do` 取开学日期。
6. 浏览器上下文使用 `credentials: include`，在那里规范化，再通过带 channel/version/type/requestId 的消息回传。
7. Native 校验成功后关闭 WebView，持久化数据，显示导入门数和被跳过的无效行数量。

完整请求地址、方法、Content-Type、Header、参数、响应路径及固定上游提交见 [协议研究笔记](../research/iwut-course-reference.md)。本轮只核实源码协议；真实学校账号的成功导入仍必须实测。

## Privacy / Security Review

- 仅缓存课程、学期码、开学日期、更新时间、学校/provider、时区与跳过行数；学号只在学校 WebView 请求中短暂使用，不回传、不落盘。
- 不创建任何密码输入组件或密码 Storage，不保存密码，不上传学校 Session/Cookie/课表到 AskU 服务。
- 使用 incognito、关闭页面缓存、关闭共享 Native Cookie、关闭 WebView 调试和表单保存；不主动读取 Cookie。iOS 的非持久化数据存储与 Android 的 incognito 行为不同：已核对 WebView 13.16.1 Android 实现会在进入时清除旧 Cookie、缓存和表单，不应宣称 Android 退出时绝对不存在任何操作系统 WebView 数据。下一次导入仍从新的手动登录流程开始。
- 仅允许 HTTPS 的 `zhlgd.whut.edu.cn` / `jwxt.whut.edu.cn` 导航，拒绝相似后缀域、userinfo、非默认端口、文件和外部协议。阻止新窗口。
- `originWhitelist=['*']` **不是信任所有站点**：这是为使所有导航进入 `onShouldStartLoadWithRequest` 严格检查，防止 WebView 的默认外部 Linking 行为。官方网页自身的子资源仍由系统 WebView 加载，导航控制不等同于网络代理级过滤。
- 脚本只在目标教务页主 frame 执行；Native 只在活动导入状态接受来自目标页、匹配 requestId 的消息；重复/过期/无关消息忽略，错误 schema 拒绝。
- 消息上限 2,000,000 字符；最多 2,000 安排、64 位周次、16 节。坏行提示数量；全坏行拒绝替换；合法空 rows 可保存。
- 只有开发模式记录固定 `[timetable]` 和枚举错误码，不记录原始错误、URL query、账号、完整响应、Cookie、密码或 analytics。没有后端转发请求。
- 缓存是设备私有的普通 AsyncStorage，不是加密保险箱；系统备份/已解锁设备访问是剩余风险。V1 每台设备只维护一个活动课表，换人使用设备前可清除；未来多账号须增加账号级隔离。

## 日期及数据约定

课程 weekday 为 1–7；开始/结束节次包含端点；weeks 为排序、无重复的精确周数集合。课程同名稳定映射颜色；完全重复安排去重，不合并不同地点/教师或真正冲突。

按 IANA timezone 取得学校日历日期，再用日期序号计算周数；不会根据手机时区误移日期。非周一开学日期归属其所在周的周一。实际周数不强制截断，UI 可提示未开学/学期结束；可浏览至少 20 周，有更后课程时扩展到相应周次（最多 64）。20 周是导航缺省范围，不是声称已经获取了学校的学期结束时间。

## iwut Reference / License

事实参考为 [TokenTeam/iwut](https://github.com/TokenTeam/iwut/tree/14e0ba022f42b6222f4f5a16504ceaf5c918712b) main 提交 `14e0ba022f42b6222f4f5a16504ceaf5c918712b`。

核查了 `bachelor-import.ts`、`normalize.ts`、`app/browser/course.tsx`、`use-zhlgd-autologin.tsx`、`store/course.ts`、`store/user-bind.ts`、`lib/storage.ts`、`lib/date.ts`、`lib/course-weeks.ts`。定位了原课表/导入组件和 Tab 的入口关系，没有沿用其 UI。

**未复制 iwut 的源码、组件树、样式、配色或资产；未将上游仓库加入 AskU。** 仅依据 API、字段、流程事实独立实现。逐字符位图解析、Course schema、冲突排布、UI、store、桥协议、测试均为 AskU 新实现。上游是 AGPL-3.0-or-later，研究记录不构成商业合规法律保证。

## 文件清单

新增：

- `apps/mobile/src/features/timetable/domain/{course,date,timetable}.ts`
- `apps/mobile/src/features/timetable/providers/course-provider.ts`
- `apps/mobile/src/features/timetable/providers/{registry,mock-course-provider}.ts`
- `apps/mobile/src/features/timetable/providers/whut/{whut-auth,whut-parser,whut-browser-entry,whut-browser.generated,whut-script,whut-course-provider}.ts`
- `apps/mobile/src/features/timetable/store/timetable-repository.ts`、`timetable-store.tsx`
- `apps/mobile/src/features/timetable/components/{CourseImportBrowser,WeekNavigator,WeekHeader,TimetableGrid,CourseBlock,CourseDetailSheet}.tsx`
- `apps/mobile/src/features/timetable/screens/TimetableScreen.tsx`
- `apps/mobile/tests/{timetable,timetable-bridge}.test.ts`
- `apps/mobile/scripts/build-timetable-browser.mjs`、`apps/mobile/eslint.config.js`
- 本报告与 `docs/research/iwut-course-reference.md`

修改：首页入口、RootNavigator、RootStack 类型、AppProviders、package.json/lock、tsconfig、移动端 README、`.gitignore`（忽略 UI QA 产物）。未修改后端。已有 `.workbuddy/` 和 `asku_whut_data.zip` 未动。

依赖：Expo 兼容 WebView 13.16.1、expo-crypto（请求 nonce）、Zod；react-dom/react-native-web 用于已有 web 启动命令及跨尺寸 UI QA；开发依赖 ESLint/Expo 配置、tsx、esbuild、Node 类型。

## Test

执行环境：Node 22.23.1（系统默认 Node 20 不满足已有项目要求）。

| 检查 | 结果 |
| --- | --- |
| `npm run typecheck` | 全项目通过，包括测试 |
| `npm run lint` | 课表功能及本轮接入文件、测试、脚本通过，0 warning |
| `npm test` | 48/48 通过，包含生成脚本的新鲜度检查 |
| `npx expo install --check` | 依赖匹配当前 Expo SDK |
| `npm run export:android` | Android JS / Hermes 导出通过 |
| `npm run export:ios` | iOS JS / Hermes 导出通过；不等同于 Xcode 原生编译 |
| Web UI QA | 320、390、768 宽度，0 console error |
| Android 原生构建 | 短英文临时目录执行 `assembleDebug` 成功，253 tasks，x86_64 调试 APK |
| Android 模拟器 | API 36：首页入口、Mock 导入、水平滑动、回到本周、冲突详情、学校官方登录网页打开、取消保留、强制停止后重启恢复课表均通过 |
| `npm run lint:all` | 未通过：历史代码 11 error、7 warning，详见下文 |
| `npm audit` | 17 moderate，0 high/critical，详见下文 |

自动测试覆盖：连续/单/双/不连续/空/非法/超长位图，月份/年份/闰日/上海零点边界，美国/英国手机时区与 DST，周六/周日，缺教师/地点，冲突/重复安排，schema 越界和多余敏感字段剔除，持久化/损坏缓存/失败保留，域名伪装/消息来源/nonce/type/version，四接口请求链、重复注入、非主 frame、网络/HTTP/超时/格式/认证失败。

浏览器实际交互验证：空状态 → Mock 导入 → 冲突课程详情 → 单/双周切换 → 回到本周 → 第17周空课表 → 页面重载恢复 → 重新导入取消保留 → 周日缺省详情 → 取消清除 → 确认清除 → 重载后仍为空 → 再导入。触摸事件验证水平滑动切周；普通桌面鼠标拖拽不是移动端触摸手势。截图位于被忽略的 `apps/mobile/artifacts/timetable-*.png`。

Android 实测使用已有 `AskU_Phase2_API36` 模拟器，并保留已安装 App 的数据执行更新安装。官方 WebView 已显示真实武汉理工登录页，账号和密码均保持空白；没有进行任何账号尝试。`timetable-android-week.png` / `timetable-android-login.png` 为原生截图。

本机原项目中文路径触发 Gradle 的 Windows non-ASCII 检查。没有搬动或改写原项目，而是在短英文临时目录复制源码（排除密码环境文件、node_modules、旧 native 产物），`npm ci` → `expo prebuild --platform android --no-install` → `gradlew app:assembleDebug` 完成原生验证。Metro 仅监听 IPv6 的 localhost 时，模拟器默认 IPv4 地址无法连接；改为 LAN 监听并冷启动后正常。发布建议仍使用 EAS 或标准英文路径构建环境。

`artifacts/asku-timetable-debug-x86_64.apk` 是本轮模拟器调试产物，依赖 Metro 8083，不是 ARM 真机发行包，也不使用它做生产分发。

本轮原生临时构建副本仍保留在 `D:/Desktop/asku-qa-e929a308`：清理请求被执行工具策略拒绝，没有绕过该限制。该目录不是主项目；APK 和截图已另存到主项目的 `apps/mobile/artifacts/`。关闭临时开发服务后，调试 APK 再次冷启动需要运行 `npx expo start --port 8083 --lan`；正式离线启动验收应使用打包了 JS 的真机发行构建。

全量 lint 是新添加的额外历史检查，原项目没有 lint script。遗留位置：`AgentStatus.tsx` / `StreamingCursor.tsx` 的 Animated ref 编译器规则，`SourceDetailScreen.tsx` 的 effect setState 编译器规则，历史页 Hook 依赖，以及三个旧组件未使用 import。没有关闭这些规则，也没有为本任务重构无关模块；因此不能声称全仓库 lint 已干净。

依赖审计剩余风险来自 Expo/xcode/uuid 与 React Navigation/query-string/decode-uri-component 链。npm 给出的部分修复会把 Expo 降到 46 或没有兼容修复，未执行 `audit fix --force`。发布前需跟踪上游兼容版本。

## Manual Verification（真实账号必须做）

前提：使用自己有权访问的武汉理工**本科**账号，在 Android 和 iOS 真机各验收。新增原生依赖需先重建安装包（`npm run android` / macOS 的 `npm run ios` 或已有 EAS 流程），只刷新 Metro 不够。不要把账号密码发到终端、聊天、调试日志。

1. 打开 AskU 首页，确认 AI 提问和原导航正常；点「课表」。
2. 可先用演示验证界面，再点「重新导入」进入真实学校官方页面。
3. 本人在学校页面输入登录信息，正常完成验证码/短信；错误密码应由学校自己提示。
4. 检查 CAS 到教务的实际跳转，仅必要域名被允许；若学校新增域名，先核实再精确扩展白名单，不能改成任意域。
5. 等待读取和导入完成；核对学期、开学日期、课程名称、星期、节次、单/双/不连续周与学校课表一致。
6. 点击两门冲突课程分别查看；核对缺教师/地点、周末与无课周显示。
7. 左右触摸滑动/按钮切周，点击周标题回到本周；上下滚动应保持顺畅。
8. 退出并重启 App，断网进入课表仍应可看；不能自动请求教务。
9. 把手机时区改为伦敦/纽约，在上海日期跨日/周边界核对本周与今天；恢复原手机时区。
10. 点击重新导入，测试取消/断网/学校超时，旧课表保持不变；再次成功导入应完整替换旧快照。
11. 测试连续点击、返回键、进后台再回来、取消后晚到消息；无重复导入或取消后覆盖。
12. 用经过授权的网络调试确认课表接口只由学校页面调用，AskU 后端没有收到密码、学号、Cookie、Session 或课表；勿保存敏感抓包到仓库。
13. 在校方系统无课的真实学期验证合法空 `rows` 行为。测试清除确认及重启后数据确实消失。

真实登录、Cookie/CAS 兼容性、学校实际 Response、iOS 原生构建/真机和真实个人课表准确性，在没有账号实测前均不标记通过。
