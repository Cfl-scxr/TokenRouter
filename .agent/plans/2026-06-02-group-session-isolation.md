# 分组会话隔离开关实施计划

## Summary

- 新增分组级开关 `session_isolation_enabled`，在分组管理创建/编辑页显示为“开启会话隔离”。
- 当前目标分组启用开关时，同一用户的同一显式会话如果最早归属于其它分组，本次请求返回 403，不进入账号调度。
- 隔离只作用于显式客户端会话信号，不作用于内容 hash 或摘要 fallback。

## Key Changes

- 后端新增 `groups.session_isolation_enabled BOOLEAN NOT NULL DEFAULT false`，迁移文件为 `152_add_group_session_isolation.sql`。
- 同步字段到 ent schema、service `Group`、创建/更新 input、admin request/response DTO、group repository、API Key 认证缓存。
- 扩展 `GatewayCache` 支持会话归属记录：`sticky_session_owner:{userID}:{source}:{sessionHash}`。
- 新增服务层显式会话归属校验，并接入 OpenAI、Anthropic/Gateway、Gemini native/CLI 入口。
- 前端 `GroupsView.vue` 增加开关、状态徽标、payload/type/i18n 字段。

## Test Plan

- 缓存测试：首次绑定、同组刷新、目标隔离分组遇到其它 owner 冲突、并发首次绑定。
- 后端映射测试：创建/更新/返回/API Key auth cache 保留 `session_isolation_enabled`。
- 网关测试：目标分组开启隔离时跨分组显式会话返回 403；未开启时放行；同分组放行；fallback 不误拦截。
- 前端测试：创建/编辑表单回填、切换、提交 payload 包含字段。
