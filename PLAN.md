# 数据共享功能实施计划

## Summary

新增“数据共享”子系统：管理员可把分组标记为数据共享分组并配置须知内容；用户切换 API Key 到该分组时弹出“数据共享须知”并强制等待 10 秒确认；数据共享分组产生的对话以完整 session 格式保存，供用户查看下载，供管理员管理、删除、统计图表和按附件 JSONL 格式导出。

已锁定：

- 采集粒度：只保存聚合后的 session，不保存单次 API 快照。
- 管理员改组：管理员可直接把 API Key 切到数据共享分组。
- 数据保留：不自动清理，只提供手动删除。
- 管理指标：管理端记录并展示“单 session 平均 token 量”。
- 暂不新增额外统计指标。

## Key Changes

- 数据模型与迁移：
  - `groups` 增加 `data_sharing_enabled` 字段。
  - 新增 `data_share_sessions` 表，保存完整 session JSON、用户/API Key/分组/模型/provider、状态、质量标记、`storage_bytes`、`input_tokens`、`output_tokens`、`total_tokens`、创建/更新时间。
  - 新增或复用 settings 保存须知内容与版本；用户确认记录落库到 API Key 维度，记录目标分组、须知版本、确认时间。
  - Ent schema、DTO、repository、service 同步新增字段；新增代码注释使用中文。

- 同意与分组切换：
  - 用户端 `PUT /api/v1/keys/:id` 在目标分组为数据共享分组时，要求携带有效确认信息。
  - 新增用户接口获取须知、提交确认、查询目标分组是否需要确认。
  - Keys 页切换到数据共享分组前展示“数据共享须知”弹窗，内容来自管理端配置，确认按钮倒计时 10 秒，倒计时结束且用户点击确认后才提交改组。
  - 管理端 API Key 改组接口可直接切换，并记录管理员操作来源。

- Session 采集与导出：
  - 在 Claude/OpenAI/Gemini 网关请求完成后，仅当 API Key 所属分组 `data_sharing_enabled=true` 时采集。
  - 采集器按现有会话信号聚合：`session_id`、`conversation_id`、`prompt_cache_key`、metadata session，缺失时使用现有 session hash。
  - 同一会话多次请求持续合并 messages、tools、usage、request_id，并更新 `total_tokens` 与 `storage_bytes`。
  - 导出 JSONL 对齐附件字段：`trajectory_id`、`session_id`、`dataset`、`provider`、`model`、`created_at`、`ended_at`、`status`、`is_final_snapshot`、`source_request_count`、`system_prompt`、`tools`、`messages`、`usage`、`meta`。
  - 默认只导出符合附件硬要求的记录；不合格 session 保留在管理页，可查看原因、删除，但不进入默认正式导出。

- 页面与统计：
  - 用户端新增侧栏“数据共享”入口和 `/data-sharing` 页面：查看自己的 session、筛选、查看详情、下载单条或批量 JSONL。
  - 管理端新增侧栏“数据共享”入口和 `/admin/data-sharing` 页面：配置须知、管理数据共享分组、查看所有 session、筛选、批量删除、导出 JSONL。
  - 管理端统计接口返回：session 总数、合格/不合格数量、总占用空间、按时间增长的空间、总 token、单 session 平均 token 量。
  - `avg_tokens_per_session = sum(total_tokens) / session_count`，按当前筛选条件实时计算；无 session 时返回 0。
  - 图表使用现有 Chart.js，重点展示空间占用，并增加平均 token 量指标卡和趋势/分组对比。

## API / Type Additions

- 用户接口：
  - `GET /api/v1/data-sharing/notice?group_id=...`
  - `POST /api/v1/data-sharing/confirm`
  - `GET /api/v1/data-sharing/sessions`
  - `GET /api/v1/data-sharing/sessions/:id`
  - `GET /api/v1/data-sharing/export`

- 管理接口：
  - `GET/PUT /api/v1/admin/data-sharing/notice`
  - `GET /api/v1/admin/data-sharing/sessions`
  - `GET /api/v1/admin/data-sharing/sessions/:id`
  - `DELETE /api/v1/admin/data-sharing/sessions/:id`
  - `POST /api/v1/admin/data-sharing/sessions/batch-delete`
  - `GET /api/v1/admin/data-sharing/export`
  - `GET /api/v1/admin/data-sharing/stats`

- 类型新增：
  - `Group.data_sharing_enabled`
  - `DataShareSession`
  - `DataShareNotice`
  - `DataShareStats.avg_tokens_per_session`
  - API Key 更新请求增加数据共享确认字段。

## Test Plan

- 后端测试：
  - 分组 `data_sharing_enabled` 创建、更新、列表、详情正确。
  - 用户切换到数据共享分组时，无确认被拒绝，有确认成功；管理员改组直接成功。
  - session 聚合器能合并多次请求、累计 usage、计算 `total_tokens` 和 `storage_bytes`。
  - 导出过滤无工具、纯报错、普通问答、模型不合规、工具未配对等记录。
  - 管理统计正确计算总空间、总 token、单 session 平均 token 量。

- 前端测试：
  - Keys 页弹窗标题、倒计时、确认后改组流程正确。
  - 用户/管理端侧栏入口和路由权限正确。
  - 用户页可列表、详情、下载。
  - 管理页可配置须知、筛选、删除、导出，并展示空间占用和平均 token 指标。

## Assumptions

- 用户下载仅限自己的数据；管理员可查看、删除、导出全部数据。
- 不做自动过期清理。
- 须知内容默认提供中文模板，管理员可覆盖；每次修改生成新版本。
- 正式导出严格遵守附件质量规则，不合格记录保留但默认不导出。

## 参考文件

- /Users/daodaoneko/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/wxid_xg380fm90d1c22_3f04/msg/file/2026-05/Agent数据交付格式说明.docx