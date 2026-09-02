# AskU Frontend Architecture V0.9

## Citation Source of Truth

`message.completed.message.citations` 是引用唯一事实源。`ChatMessage` 只把结构化 Citation 交给 `CitationPills`，点击后按 `sourceId` 导航到 Source Detail；禁止从 Markdown 正文正则提取 `[1]`。`sources.updated` 仅用于生成过程和来源预览。

Source Detail 支持官方网页、官方附件和 parent page。内部文件路径不属于前端 Domain，也不允许通过 Mock/API Adapter 传入。

## Dependency flow

```text
Screen / Component
        ↓
Controller Hook (screen state and use-case flow)
        ↓
ChatService / AuthService (product contracts)
        ↓
ApiChatService / ApiAuthService (wire adapters)
        ↓
ApiClient (authenticated retry)
   ┌────┴──────────────┐
ApiSessionManager   ApiTransport
        ↓
TokenStore
```

`ServiceProvider` is the mobile composition root. Screens do not read wire JSON, own tokens, or call `fetch`. Mock and API implementations satisfy the same product contracts.

## Responsibilities

- `screens/`: layout, interaction wiring and navigation only.
- `hooks/`: page lifecycle, async orchestration and transient UI state.
- `components/`: reusable visual units; no network or persistence access.
- `services/chat` and `services/auth`: consumer-owned contracts.
- `services/api`: HTTP/SSE wire protocol adapters.
- `ApiSessionManager`: restore, refresh, login bootstrap and one shared session-mutation single-flight. Only an explicit invalid-refresh response clears credentials; transient network/provider failures preserve the session.
- `TokenStore`: replaceable secure-storage boundary; AsyncStorage is the current demo adapter.
- `config/runtime.ts`: validated runtime modes, API URL and app version.

## SSE boundary

`ApiChatService` creates a run and consumes `/v1/runs/{id}/events`. It owns SSE framing, event-id deduplication, reconnect and wire-to-domain mapping. UI only receives `ChatEvent` product states; raw provider payloads and model reasoning never reach screens.

Unexpected EOF before a terminal event is treated as a reconnectable failure. A lagging backend subscriber is disconnected rather than losing events silently, then resumes from persisted event id.

## Persistence ownership

- Backend PostgreSQL: users, sessions, messages, sources, feedback, runs and SSE event history.
- Device TokenStore: current session credentials only.
- Mock adapters: isolated UI regression data only.
- Controller state: input, streaming buffer, agent state and local feedback response.

## Replace adapters

- Real backend: keep `ChatService`; replace or extend `ApiChatService` mapping.
- Real WeChat: implement the login entry, set `EXPO_PUBLIC_ASKU_AUTH_MODE=wechat`, keep `ApiSessionManager` token lifecycle.
- Secure credentials: add a `TokenStore` implementation backed by secure storage and change only the composition root.
- Offline UI regression: set `EXPO_PUBLIC_ASKU_SERVICE_MODE=mock`.

## Compatibility rules

- Main layout uses Flexbox, Safe Area and keyboard resize; platform differences stay under `src/platform`.
- Chat uses `FlatList` and semantic chunks, not per-character updates.
- Touch targets and primary controls include basic accessibility labels.
- Production API must use HTTPS; cleartext is enabled only for explicitly built local test APKs.

## Current boundary

V0.9 completes the citation trust chain and end-to-end integration. Knowledge retrieval is disabled by default, while Search and LLM providers default to explicitly labelled Mock adapters; production must configure real providers before answers can be treated as official policy results. Native iOS archive requires macOS and Xcode.
