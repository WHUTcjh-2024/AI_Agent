# AskU Phase 2 实施与联调说明

## 目标

本阶段按《AskU 模块化技术开发方案》先完成可验证的产品控制层：移动端不改页面，通过 Adapter 接入 Go Backend；后端负责认证、会话、消息、AgentRun、可重连 SSE、来源与反馈；PostgreSQL 保存长期数据，Redis 承担限流与幂等。

## 已落地架构

```text
React Native UI
    ↓ ChatService / AuthService
ApiChatService / ApiAuthService
    ↓ REST + replayable SSE
Go Backend
    ├── PostgreSQL：用户、Token Hash、会话、消息、运行、事件、来源、反馈
    ├── Redis：提问限流、幂等键
    ├── SchoolContext：武汉理工配置
    └── Agent Engine Adapter：当前为明确标注的 MockEngine
```

## 联调边界

- 当前回答引擎是 Mock Agent，只用于验证端到端工程链路，不冒充真实校园知识。
- 真实微信登录已保留 `/v1/auth/wechat`，但缺少开放平台凭证时明确返回未配置；测试环境自动使用开发登录。
- WeKnora、Crawler、实时搜索和 LLM 尚未接入。后续在 Agent Engine Adapter 内替换，不改移动端页面和 SSE 消费模型。
- 联调使用 HTTP 明文仅面向本机/局域网开发；生产必须切换 HTTPS 并关闭 Android cleartext 配置。

## 验收命令

```powershell
docker compose -f infrastructure/docker/docker-compose.yml ps
Invoke-RestMethod http://localhost:18080/healthz

cd backend
go test ./...
go vet ./...

cd ../apps/mobile
npm run typecheck
npm run doctor
npm run export:android
npm run export:ios
```

## 测试场景

- `转专业有什么要求？`：SSE 分段回答、官方来源卡、来源详情。
- `宿舍可以养宠物吗？`：无可靠来源，明确拒绝猜测。
- `offline`：模拟 Agent 上游失败与重试状态。
- 生成中点击 Stop：取消 AgentRun 并写入终止事件。
- 历史页：恢复会话、删除、清空和空状态。
- 回答操作：有帮助、没帮助、复制、分享。
