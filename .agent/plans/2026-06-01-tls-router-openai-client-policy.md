# TLS 路由器与 OpenAI OAuth 客户端策略

## Summary
- 新增一类一等资源“TLS 路由器”，放在账号页“更多操作 → 工具”下，紧挨“TLS 指纹模板”。
- 账号继续支持固定 TLS 模板，同时可选择 TLS 路由器：运行时按用户请求 `User-Agent` 匹配 TLS 模板，未命中则回退账号固定模板。
- OpenAI OAuth 的客户端限制改成三态：允许任意客户端、只允许 Codex 客户端、只允许 TLS 路由器匹配到的客户端。

## Key Changes
- 后端新增 `tls_fingerprint_routers` 表和对应 model/repository/service/cache/handler。
- 路由器规则为有序数组，按顺序 first-match，支持 `contains`、`prefix`、`exact`、`regex`。
- 账号 extra 新增 `tls_fingerprint_router_id` 与 `openai_oauth_client_policy`。
- 兼容旧 `codex_cli_only` 字段；缺少新策略时按旧字段映射。
- TLS 启用且选择 router 时先按入站 UA 匹配 router；未命中、router 禁用、不存在、目标 profile 不可用时回退账号固定模板。

## Frontend
- 账号页“更多操作 → 工具”新增“TLS 路由器”，打开 `TLSFingerprintRoutersModal`。
- TLS 路由器弹窗支持列表、创建、编辑、删除、启停、规则排序。
- 创建/编辑/批量编辑账号的 TLS 区域新增“TLS 路由器”下拉框。
- OpenAI OAuth 区域将“仅允许 Codex 官方客户端”改为“客户端访问策略”下拉。
- OpenAI OAuth 导入默认值增加固定模板、TLS 路由器、客户端访问策略字段。

## Test Plan
- 后端覆盖 TLS router CRUD、规则校验、UA 匹配、TLS profile 解析和 OpenAI OAuth 三态访问策略。
- 前端覆盖 AccountsView 入口、TLS 路由器表单、创建/编辑/批量编辑/导入默认值字段读写。
- 验证命令：
  - `go test ./backend/internal/service ./backend/internal/handler/admin ./backend/internal/pkg/openai`
  - `pnpm --dir frontend test -- AccountsView EditAccountModal BulkEditAccountModal SettingsView TLSFingerprintRoutersModal`

## Assumptions
- TLS 路由器只匹配用户请求的 `User-Agent`。
- TLS 路由器是账号可选择的共享资源，不放进系统全局设置，也不会自动影响所有账号。
- 新迁移使用 `151_add_tls_fingerprint_routers.sql`。
- 代码注释用中文；不提交 `SYNC.md`；commit message 使用 conventional commits。
