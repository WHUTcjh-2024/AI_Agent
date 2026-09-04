# AskU APP Architecture V0.9

React Native / Expo / TypeScript 移动端。默认通过 `ApiChatService` 连接 AskU Go Backend，真实持久化会话并消费可重连 SSE；`MockChatService` 仅保留为离线 UI 回归模式。

## How to Run

环境要求：

- Node.js 22.13+（本项目依赖 React Native 0.86）
- npm 10+
- Android：Android Studio / SDK、JDK 17
- iOS：macOS、Xcode 和 CocoaPods

```bash
cd apps/mobile
npm install
npm start
```

先按仓库根目录说明启动 PostgreSQL、Redis 和 Backend。Android 模拟器默认访问 `http://10.0.2.2:18080`；iOS 模拟器默认访问 `http://127.0.0.1:18080`。

真机需复制 `.env.example` 为 `.env.local`，把 `EXPO_PUBLIC_ASKU_API_BASE_URL` 改为开发电脑局域网地址后重新构建。临时回退本地 Mock：

```text
EXPO_PUBLIC_ASKU_SERVICE_MODE=mock
```

在 Expo Dev Server 中扫码，或按 `a` / `i` 打开 Android / iOS 模拟器。

## Android

```bash
npm run android
```

如果需要生成本地原生工程并在已连接设备运行：

```bash
npx expo run:android
```

项目已配置 `softwareKeyboardLayoutMode: resize`，Chat 输入区同时使用 Safe Area 和 Keyboard Avoiding。

## iOS

需在 macOS 执行：

```bash
npm run ios
```

业务代码没有 Android 专属文件路径或 Android-only API；必要的平台差异集中在 `src/platform`。

## Build APK

`artifacts/` 只存放本地构建产物且不会提交到 Git；发布包必须由 CI/EAS 或受控签名环境生成，不使用开发证书分发。

推荐用 EAS Preview 构建可直接安装的 APK：

```bash
npm install -g eas-cli
eas login
eas build --platform android --profile preview
```

`eas.json` 的 `preview` 配置已设置 `android.buildType: apk`。构建完成后，EAS 会提供安装链接。

也可在 Android 工具链完整时进行本地 Release 构建：

```bash
npx expo prebuild --platform android
cd android
./gradlew assembleRelease
```

Windows 使用 `gradlew.bat assembleRelease`，APK 通常位于 `android/app/build/outputs/apk/release/`。本地 HTTP 联调包需额外传入 `-PaskuUsesCleartextTraffic=true`；正式包默认禁止明文 HTTP。正式分发前需要配置并妥善保管签名密钥。

Windows 本地构建时，部分 Gradle/CMake 工具链仍可能无法正确处理包含中文的项目绝对路径；如遇 autolinking 或路径解析失败，请用 EAS 构建，或将项目临时复制到短英文路径完成本地打包。此限制不影响源码运行与最终 APK。

## Architecture

```text
src/
├── app/            # Navigation 与 Providers
├── screens/        # Home / Chat / Source / History / Profile
├── components/     # Chat、来源、反馈与通用组件
├── hooks/          # Screen Controller 与异步用例编排
├── services/       # Product Port、API Adapter、Token 生命周期
├── config/         # 校验后的运行配置
├── mocks/          # Normal、Multi-source、No-source、Long、Error 数据
├── store/          # 轻量跨页面状态
├── theme/          # Colors / Spacing / Typography / Radius / Shadows / Motion
├── types/          # User、Session、Message、Source、ChatEvent 等契约
├── platform/       # 极少量跨平台差异
└── utils/
```

导航采用“首页 / 历史 / 我的”三 Tab；Chat 和 Source Detail 使用 Stack 推入，不占用底部 Tab。

## Service Adapter

UI 只依赖 `ChatService` 与标准 `ChatEvent`。当前实现：

- `ApiChatService`：REST、SSE 解析、event id 去重、断线续传、取消。
- `ApiSessionManager`：Token 恢复、自动轮换、并发 401 单飞恢复。
- `ApiAuthService`：用户身份读取边界。
- `MockChatService`：设置 `EXPO_PUBLIC_ASKU_SERVICE_MODE=mock` 后启用。

后续接入真实 Agent 时只替换后端能力 Adapter，不需要重写 Screen。详细边界见 `../../docs/frontend-architecture.md`。

## Demo Scenarios

- `四六级什么时候报名` / `官网搜索测试`：Web Search Gateway + Top-N 来源 + Redis 缓存
- `转专业`：受控回答 + 联调来源
- `宿舍可以养宠物吗`：无可靠来源
- `offline`：失败与重试
- History 中“清空”后检查 Empty State，并可添加后端联调示例

## Quality Commands

```bash
npm run typecheck
npm run lint
npm test
npm run doctor
npm run export:android
npm run export:ios
```

## 课表

首页「课表」进入独立页面，不增加底部 Tab。支持武汉理工本科官方 WebView 登录导入、周次/日期浏览、冲突课程详情、重新导入与本地离线缓存。空状态可选择「先体验演示课表」，演示数据会明确标记，可用于完整离线 UI 验证。

首次加入 `react-native-webview` / `expo-crypto` 后，需要重新构建原生 App；仅重启 Metro 不会更新旧安装包的原生模块。Web 版可验证课表 UI，但不提供学校 WebView 登录。

修改 `src/features/timetable/providers/whut/whut-browser-entry.ts` 或 `whut-parser.ts` 后运行 `npm run timetable:bundle`。生成的独立浏览器脚本一并提交，`npm test` 会检测过期脚本，避免运行时函数序列化在 Hermes/压缩构建中失效。

`npm run lint` 检查本次课表功能、接入文件、测试和生成脚本；`npm run lint:all` 额外扫描历史代码。新引入的 Expo ESLint 配置在旧聊天/历史/来源组件中发现已有问题，未在课表任务内重构它们。全项目 TypeScript 检查仍由 `npm run typecheck` 执行。

完整架构、安全边界、自动测试和真机验收步骤见 [课表开发报告](../../docs/features/timetable.md)；协议依据见 [iwut 研究记录](../../docs/research/iwut-course-reference.md)。

### 单校移植（V0.11）

设置 `ASKU_SCHOOL_CONFIG` 为根目录学校 YAML 的绝对路径，运行
`npm run timetable:bundle`，然后类型检查、测试与导出。
`scripts/build-school-config.mjs` 只生成公开 Mobile Adapter 字段，不包含知识库 ID。
`mobile_timetable` 未启用时不开放教务导入；已有解析器适用于当前教务接口协议。
不同教务协议仍需专门的 Provider，这是课表功能限制，不影响校园问答移植。
课表缓存按构建学校隔离；旧版未分学校的本地课表需重新导入。
