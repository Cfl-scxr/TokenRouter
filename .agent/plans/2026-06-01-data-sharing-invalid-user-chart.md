# 数据共享无效会话用户排行图表

## Summary
在管理端数据共享页新增“无效会话用户排行”图表，用来按无效会话数量找出贡献异常多无效数据的用户。图表固定统计 `quality_status = invalid`，但仍跟随日期、模型、路径、User-Agent、分组、Key、搜索等其他筛选；用户当前选择“完整/部分完整”质量筛选时，这个图表也继续展示无效贡献者。

实施前先把本计划保存到 `.agent/plans/2026-06-01-data-sharing-invalid-user-chart.md`，不提交 `SYNC.md`。

## Key Changes
- 后端扩展现有 `/admin/data-sharing/stats` 返回值，新增 `invalid_user_breakdown`，不新增独立接口。
- 新增公开统计类型：
  - `user_id`
  - `user_name`
  - `user_email`
  - `session_count`: 同一筛选条件下该用户全部 session 数，忽略当前质量筛选
  - `invalid_count`: 无效 session 数
  - `invalid_ratio`: `invalid_count / session_count`
  - `storage_bytes`: 该用户无效 session 占用空间
  - `total_tokens`: 该用户无效 session token 总量
- 后端聚合逻辑：
  - 基于现有 stats 筛选构造查询，但清空 `QualityStatus` 后统计用户总量。
  - 用 `COUNT(*) FILTER (WHERE quality_status = 'invalid')` 统计无效数。
  - 仅返回 `invalid_count > 0` 的用户。
  - 排序固定为 `invalid_count DESC, storage_bytes DESC, user_id ASC`，限制 Top 20。
  - 关联 `users` 表补充用户名和邮箱。
- 前端类型 `DataShareStats` 增加 `invalid_user_breakdown`，新增横向 Bar 图：
  - 标题为“无效会话用户排行”。
  - X 轴为无效 session 数，Y 轴为用户显示名。
  - Tooltip 展示无效数、总数、无效率、空间、token。
  - 空数据时展示现有风格的空状态。
- 图表交互：
  - 点击某个用户条形后，将下方列表筛到该用户并把质量筛选切到“无效”。
  - 前端新增内部 `filters.user_id`，并在筛选区显示可清除的用户筛选 badge，避免隐藏筛选造成困惑。

## Test Plan
- 后端：
  - 更新 `backend/internal/repository/data_share_session_repo_test.go` 中 stats 查询测试，覆盖 `invalid_user_breakdown` 返回和排序。
  - 增加/调整测试确认该 breakdown 忽略传入的 `QualityStatus`，但保留其他筛选条件。
  - 跑 `go test ./internal/repository ./internal/service ./internal/handler/admin`。
- 前端：
  - 跑 `pnpm --dir frontend typecheck`。
  - 用当前 `http://localhost:3000/admin/data-sharing` 验证图表渲染、loading/empty 状态、tooltip、点击下钻和清除用户筛选。
  - 检查桌面和窄屏下图表标签不溢出、不遮挡现有卡片。

## Assumptions
- “大量贡献”按无效会话绝对数量排序，不用无效率或综合风险排序。
- 图表固定看无效会话，不跟随质量筛选项。
- v1 不新增数据库迁移；复用现有 `quality_status`、`user_id`、`created_at` 等索引，后续如果真实数据量下 stats 慢，再单独加复合索引。
