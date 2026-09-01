# AskU Architecture V0.5

## Goal

V0.5 turns the existing phase modules into replaceable layers. The HTTP contract and product state remain stable while search, model, knowledge base, authentication and storage adapters can evolve independently.

## System boundary

```text
Mobile UI
  Screen → Controller Hook → Product Service → API/SSE Adapter
                                         ↓
Go API
  Handler → Run Coordinator → Agent Executor
                              ├→ Router
                              ├→ Searcher → Web Search Provider / future WeKnora
                              └→ Generator → LLM Provider

Infrastructure adapters
  PostgreSQL implements consumer repository ports
  Redis implements cache ports
  School registry supplies school-specific policy and allowed domains
```

Only composition roots know concrete implementations:

- Backend: `backend/cmd/api/main.go`
- Mobile: `apps/mobile/src/services/ServiceProvider.tsx`

## Backend modules

| Module | Owns | Must not own |
|---|---|---|
| `api` | HTTP validation, auth middleware, response/SSE wire format | SQL, provider logic, answer generation |
| `run` | run lifecycle, event persistence/publication, cancellation | routing, search, LLM selection |
| `agent` | capability composition and product progress | HTTP, database schema, SSE transport |
| `llm` | normalized generation port, provider adapters, usage accounting | sessions or UI events |
| `websearch` | official-domain search/fetch/extract/cache pipeline | run lifecycle or message persistence |
| `store` | PostgreSQL implementation and transaction boundaries | product routing |
| `cache` | Redis implementation | business decisions |
| `domain` | stable entities and shared domain errors | infrastructure imports |

Consumer modules define the ports they need. Infrastructure implements those ports structurally; it does not push repository types upward.

## Consistency and failure model

- User message and AgentRun creation share one transaction.
- Assistant message and its source links share one transaction.
- Refresh Token consumption and replacement token creation share one transaction.
- Run terminal status and its terminal SSE event share one transaction.
- On startup, orphaned non-terminal runs are closed with a persisted `server_restarted` event.
- Source detail is scoped to the authenticated user's school.
- Run events are persisted before publication and carry increasing ids.
- A slow live subscriber is disconnected, not silently skipped; reconnect replays persisted events.
- External provider errors are translated into stable public codes and safe user messages.
- Unknown/trailing JSON is rejected before rate-limit and idempotency resources are consumed.
- Real model deltas are consumed through the streaming provider path and coalesced into short SSE chunks.

## Extension recipes

### Add a real knowledge base

Implement an `agent` retrieval capability or a `Searcher` adapter around WeKnora, register it in the backend composition root, and keep `run.Service`, handlers and mobile unchanged.

### Add an LLM provider

Implement `llm.Provider`, add provider-specific config validation, and select it in `main.go`. Usage accounting remains in `llm.Gateway`.

### Add a web search provider

Implement `websearch.Provider`. Official-domain filtering, page fetching, extraction and cache policy remain in `websearch.Gateway`. Source identity includes the school id so shared URLs do not collide across schools.

### Add another school

Add a school context file and registry selection strategy. Do not add school names or official domains to handlers, screens or providers.

### Add real WeChat authentication

Implement server-side credential validation behind the auth boundary and mobile login acquisition behind `AuthService`. Token refresh and authenticated requests remain unchanged.

## Automated architecture checks

`backend/internal/architecture/dependencies_test.go` rejects reverse dependencies such as `api → store`, `run → websearch`, or `domain → infrastructure`. Regular gates:

```powershell
cd backend
go test ./...
go vet ./...
go test -race ./...

cd ../apps/mobile
npm run typecheck
npm run doctor
npm run export:android
npm run export:ios
```

## Deliberate limits

- No Kubernetes, message broker, distributed workflow engine or microservice split.
- No real WeKnora, crawler or WeChat credentials in this repository yet.
- AsyncStorage is acceptable for internal demo tokens; production should use a secure TokenStore adapter.
- Provider selection remains startup configuration, which is appropriate for the current single-school validation stage.
- Development login is disabled by default and must be explicitly enabled by a local test environment.
