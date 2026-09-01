# AskU APP Architecture V0.5

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

本仓库生成的 V0.5 内部测试包：

```text
artifacts/AskU-Architecture-v0.5.0-release.apk
```

该文件使用 Android Debug 证书签名，仅用于 Demo 与内部测试，不可直接用于应用商店发布。

当前测试包 SHA-256：`5B638B9C231C19A0916706C92E1878312F5BDA10FD44E451DA73D4581CC1692D`

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
npm run doctor
npm run export:android
npm run export:ios
```
