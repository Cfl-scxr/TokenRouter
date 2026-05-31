# 数据共享采集耗时诊断面板

## Summary
- 将“最近耗时”卡片改为可点击入口，弹窗展示采集链路各阶段耗时和柱状分布。
- 耗时统计使用进程内“最近 N 个样本”滚动窗口，不落库、不新增迁移；服务重启后样本清空。
- 窗口大小做成全局运行配置，管理员保存后后端 recorder 立即调整。

## API / Types
- `DataShareCaptureRuntimeSettings` 新增 `duration_window_size`，默认 `512`，最小 `32`，最大 `10000`；旧 settings JSON 缺字段时使用默认值。
- `/admin/data-sharing/runtime-settings` 的 GET/PUT 支持 `duration_window_size`，和现有 worker、队列、超时、压缩、缓冲参数一起保存。
- `/admin/data-sharing/stats` 的 `DataShareStats` 新增 `capture_durations`：
  - `window_size`, `sample_count`, `parts`
  - 每个 part 包含 `key`, `label`, `category`, `last_millis`, `avg_millis`, `p50_millis`, `p95_millis`, `max_millis`, `sample_count`, `buckets`
  - 桶固定为 `<10ms`, `10-50ms`, `50-100ms`, `100-250ms`, `250-500ms`, `0.5-1s`, `1-2s`, `2-5s`, `5-10s`, `10-30s`, `>=30s`
- 保留 `capture_buffer.last_flush_duration_millis` 兼容字段；前端卡片优先展示 `flush_total.last_millis`，无样本时回退旧字段。

## Implementation Changes
- 后端新增线程安全 duration recorder，按阶段维护环形样本；窗口大小变更时保留最新样本并裁剪/扩容。
- 记录阶段：
  - `capture_queue_wait`, `capture_build`
  - `buffer_hydrate`, `buffer_merge`, `buffer_submit_total`
  - `flush_queue_wait`, `flush_finalize`, `payload_encode`, `storage_limit_check`, `db_lookup`, `db_write`, `flush_total`
- repository 的保存流程增加可选 timing hook，用于记录 payload 编码、容量检查、查重、DB 写入耗时；不改变持久化语义。
- 前端在“采集缓冲池状态”的运行配置区增加“耗时窗口样本数”输入框。
- “最近耗时”卡片改成按钮式卡片，点击打开 `BaseDialog`；弹窗按阶段显示统计摘要和柱状图，自动刷新时同步更新。

## Test Plan
- 后端：
  - duration recorder 的窗口缩放、桶边界、P50/P95、空样本测试。
  - worker queue wait、flush queue wait、buffer flush total、repository 分段记录测试。
  - runtime settings 旧 JSON 缺 `duration_window_size` 的默认值兼容测试。
- 前端：
  - `pnpm --dir frontend typecheck`
  - 浏览器验证 `http://localhost:3000/admin/data-sharing`：窗口大小保存、卡片可点击、弹窗图表/空状态/暗色模式正常。
- 回归：
  - 不新增 migration。
  - 不提交 `SYNC.md`。
  - 所有新增代码注释使用中文。
