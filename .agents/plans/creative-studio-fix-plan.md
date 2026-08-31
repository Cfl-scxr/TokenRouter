# 创作台确认问题修复计划

## Summary

本计划只处理你标记为“确认问题”的 19 项：

`TR-CS-REV-001` 至 `TR-CS-REV-013`、`TR-CS-REV-016`、`TR-CS-REV-020`、`TR-CS-REV-023`、`TR-CS-REV-024`、`TR-CS-REV-026`、`TR-CS-REV-027`。

你标记为“不处理”的项目 `TR-CS-REV-014`、`015`、`017`、`018`、`019`、`021`、`022`、`025` 不纳入本轮修复。

已确定的架构取舍：

- 使用完整 durable 状态机、outbox 和 reconciler。
- 图片仍只保留在 Redis TTL 内，不引入对象存储或数据库图片持久化。
- 接受 provider 已返回但进程在写入 Redis 前崩溃时的极窄重复调用风险，并记录告警。
- managed hidden key 继续计入现有 `APIKeyLimit`，不新增用户侧数量展示，也不暴露托管 key。

## Backend Changes

### 1. 引入可恢复的 run 状态机和 outbox

新增下一可用迁移，当前预计为 `257`：

- 扩展 run 状态：
  - `provider_succeeded`
  - `settlement_pending`
  - `release_pending`
- 增加 provisioning/结算所需的持久字段：
  - provisioning phase
  - provider result recorded time
  - settlement/release attempt count
  - next reconcile time
  - last reconcile error
- 新增 `creative_run_outbox`，至少包含：
  - `run_id`
  - `operation`：`provision`、`settle`、`release`
  - `status`：`pending`、`leased`、`done`、`cancelled`
  - `available_at`
  - lease token/expiry
  - attempt count
  - last error
  - created/updated timestamps
  - `(run_id, operation)` 唯一约束

创建流程改为：

1. DB 事务创建 run、输出元数据和 `provision` outbox。
2. 使用现有幂等 billing request ID 完成 hold。
3. 保存 payload、源图、mask 到 Redis。
4. 成功入队后标记 provisioning 完成并关闭 outbox。
5. 进程崩溃时由 reconciler 按 phase 继续执行。
6. 明确处理的 Redis/入队失败进入 failed + release_pending，不再留下假 queued 任务。

已有 queued/running/terminal 任务需要兼容迁移，不改变历史任务的图片留存边界。

### 2. 修复 Redis 队列 lease fencing

扩展 `CreativeRunQueue` 接口，使领取结果带 lease token/epoch，并让以下操作都携带并校验该 token：

- `Ack`
- `RequeueAfter`
- `Heartbeat`
- lock refresh/release
- stale active recovery

Redis 实现要求：

- active/inflight 记录保存 token。
- stale recovery 原子地检查旧 token、转移任务并生成新 token。
- refresh 返回明确的 ownership 结果，返回失去所有权时立即取消 worker 的执行 context。
- 旧 worker 失去租约后不得再写 run、输出、计费或队列状态。

新增测试覆盖 worker A 失去 Redis 连接、worker B 接管、A 恢复后无法 ACK/重排队的场景。

### 3. 分离 provider 执行和结算重试

worker 流程调整为：

1. provider 调用成功后先写 Redis 输出和 output metadata。
2. run 进入 `provider_succeeded`。
3. 创建 `settle` outbox，不再重新调用 provider。
4. settlement worker 执行 capture、usage log 和最终状态提交。
5. 成功后进入 `succeeded`，取消竞态则保留 `cancelled`，但仍完成 capture 和 usage log。
6. Redis 输出 TTL 到期时，根据已持久化的成功输出 metadata 完成计费，再进入 `result_lost`。

输出读取和 ACK 规则调整为：

- 只有结算完成且 run 已进入可交付终态后才允许读取。
- ACK 先写数据库，再删除 Redis。
- Redis 删除失败由清理 reconciler 补偿。
- 不允许在 capture 或终态提交失败时把任务从队列 ACK 掉。

### 4. 区分临时存储错误类型

为 transient store 增加明确错误分类：

- not found/expired：按既有 result lost 语义处理。
- Redis timeout、连接断开、命令失败：可重试基础设施错误，不得终结任务，不得 ACK。
- payload/output 损坏：记录明确错误并进入可补偿终态。
- `MarkResultLost`、capture、release 失败时保留 outbox 或队列记录，不能只写日志。

### 5. 可靠处理余额 hold 和 allowance

- 预占成功后同步更新 run 的 `AllowanceReserved` 持久状态。
- 回滚和 release 根据数据库事实决定是否释放 allowance，不依赖过期内存对象。
- 失败、取消、未执行丢失和 provider 成功结果丢失分别使用幂等 release/capture。
- release 失败进入 `release_pending`，由后台 reconciler 重试。
- 增加 hold、capture、release、quota rollback 的故障注入测试。

### 6. 修复 managed hidden key 竞态

- 增加当前最新迁移中的部分唯一索引，约束用户、分组和 `managed_by=creative_studio` 的组合。
- 改为 `INSERT ... ON CONFLICT` 或冲突后重新查询。
- 保持 managed key 不出现在普通用户列表。
- 保持 managed key 继续计入 `APIKeyLimit`。
- 不新增用户侧配额汇总、托管数量字段或托管 key 展示。
- 增加并发首次创建、软删除后重建和达到 APIKeyLimit 的回归测试。

## Provider and API Changes

### 1. OpenAI/Gemini 请求契约

- GPT Image 模型请求移除 `response_format`，仅在确实支持的 DALL-E 路径发送。
- 增加生成和编辑请求的严格 JSON/multipart wire contract tests。
- inpaint mask 改为不透明底图，用户涂抹区域使用透明清除；前端导出像素满足 alpha 语义。
- 后端校验 mask 为 PNG、尺寸与源图一致且不超过上游 4 MiB 限制。
- Gemini 自定义 base URL 校验失败时直接返回错误，不回退到官方 Google 地址。
- Gemini inline 图片请求在本地按“base64 后请求总大小”估算并限制在官方 20 MB 以内；本轮不引入 File API。
- 不修改 GPT Image 1 的 2K/比例行为，因为该项已标记为不处理。

### 2. 统一模型映射后的最终能力校验

抽取统一的 creative model capability resolver：

1. 解析 requested model。
2. 应用账号模型映射。
3. 对 mapped/final model 再执行图片白名单、操作能力和 provider capability 校验。
4. 目录、创建校验、定价、预留和执行器全部使用同一结果。

增加正向映射、反向映射、文本模型映射和不支持模型的测试。

### 3. 上传、prompt 和 CORS 契约

- multipart part 使用配置中的 `MaxAssetBytes`，通过 `limit+1` 读取检测是否超限，禁止静默截断。
- 前后端共享允许的 MIME 类型和魔数校验规则。
- 增加创作台 capability/limits 接口，返回：
  - `max_prompt_chars`
  - `max_asset_bytes`
  - `max_total_input_bytes`
  - `max_mask_bytes`
  - 允许的 MIME 类型
- 前端从服务端读取限制，按 Unicode code point 校验 prompt。
- 文件选择器只接受后端真正支持的 PNG/JPEG/WebP。
- CORS `Access-Control-Allow-Headers` 增加 `Idempotency-Key`，并补充 OPTIONS 测试。

## Frontend Changes

### 1. 修复 active run 轮询和历史列表

新增 active runs 查询能力：

```text
GET /api/v1/creative/runs/active?limit=100&cursor=<opaque>
```

返回：

```json
{
  "items": [],
  "next_cursor": "...",
  "has_more": false
}
```

前端行为：

- active endpoint 负责所有 queued/running/provider_succeeded/settlement_pending 任务。
- 不再只轮询最近 20 条历史记录。
- 已知 active run 即使暂时不在 history 页面中，也继续轮询和收割。
- terminal run 从 `pollStates` 中清理，避免无限增长。
- history 列表只加载元数据，输出状态改为批量查询。

### 2. 降低历史列表和输出下载压力

- 后端增加批量输出 metadata 查询，消除逐 run 的 N+1 查询。
- 前端历史刷新继续保留代次校验，但不重复读取完整 blob。
- 输出下载使用独立配置的超时，默认提高到 300 秒。
- 下载失败增加有限次数的指数退避重试。
- 大输出继续使用 Redis TTL，不增加服务端持久化图片。

## Test Plan

### 后端单元和集成测试

- Redis lease fencing：失联 worker、接管 worker、旧 token ACK/重排队失败。
- provider 成功后 capture/数据库失败：provider 不重复调用，settlement outbox 可恢复。
- output 先 ACK 后 Redis 删除失败：数据库状态保持一致，清理任务可补偿。
- Redis timeout、key expired、payload 损坏三类错误分别验证。
- hold、capture、release、allowance rollback 的幂等和失败恢复。
- create provisioning 在每个阶段崩溃后的恢复。
- managed hidden key 并发创建和唯一索引迁移。
- OpenAI GPT Image generate/edit wire contract。
- OpenAI mask alpha、PNG 大小和尺寸校验。
- Gemini base URL fail-closed 和 20 MB inline 限制。
- 模型映射后的最终白名单校验。
- multipart 配置边界、MIME/magic 校验和 CORS preflight。
- active runs 游标、has_more、旧任务继续轮询。
- 大量 history run 的批量 metadata 查询和下载重试。

### 前端测试

- mask 导出像素 alpha。
- 服务端 capability 驱动 prompt、文件类型和大小校验。
- active runs 多页轮询和 terminal 清理。
- settlement_pending/provider_succeeded 的状态展示和收割。
- 大 blob 下载超时、重试和失败恢复。

### 验证命令

```text
go test ./internal/service ./internal/handler ./internal/repository ./internal/server
go test -tags unit ./internal/service ./internal/handler ./internal/repository ./internal/server
go vet ./internal/service ./internal/handler ./internal/repository ./internal/server
go vet -tags unit ./internal/service ./internal/handler ./internal/repository ./internal/server
pnpm test:run
pnpm build
pnpm lint
git diff --check
```

## 文档和发布

同步更新：

- `docs/domains/creative_studio.md`
- `docs/interfaces/http_api.md`
- `docs/interfaces/openai_upstream.md`
- `docs/interfaces/gemini_upstream.md`
- 创作台相关配置和运维章节

文档需要反映新的状态机、settlement/release pending 语义、queue fail-closed 行为、mask alpha 规则、provider 限制和 active runs 接口。

发布顺序：

1. 先执行数据库迁移。
2. 部署同时兼容旧状态和新状态的应用版本。
3. 启动 outbox/reconciler，并观察 pending 数量、lease lost、重复 provider 防护、release pending、result lost 和 outbox lag。
4. 确认历史 queued/running 任务完成迁移后，再启用新的恢复告警阈值。

## 明确假设和排除项

- 图片不写入 PostgreSQL 或对象存储，仍只在 Redis TTL 内保留。
- provider 已返回但进程在写 Redis 前崩溃的窗口接受极窄重复调用风险；该情况只增加告警和恢复记录，不引入新的持久化图片。
- hidden key 继续计入 APIKeyLimit，但用户侧保持现有显示方式，不显示托管数量或托管 key。
- 不处理 `TR-CS-REV-014`、`015`、`017`、`018`、`019`、`021`、`022`、`025`。
- 当前最新迁移为 256；实施时使用下一可用迁移 ID。
- 开始实施前，按仓库规则将本计划原样保存到 `.agents/plans/creative-studio-fix-plan.md`；新增代码注释统一使用中文。

## 执行进度（2026-08-31）

- 已完成迁移 257、`creative_run_outbox`、provisioning/settlement/release 状态机与两个后台 reconciler。
- 已完成 Redis queue lease token fencing、失租约 context 取消、旧 worker ACK/重排队拒绝及回归测试。
- 已完成 GPT Image response_format 修正、mask alpha/大小/尺寸校验、Gemini base URL fail-closed 与 inline 20 MiB 限制、最终模型映射校验、multipart limit+1 与 CORS header。
- 已完成 active runs 游标接口、输出元数据批量查询、前端能力接口、Unicode prompt 校验、MIME 选择器、长超时与下载退避重试。
- 已同步创作台、HTTP、OpenAI、Gemini、架构、部署和数据生命周期文档。
- 验证：后端带/不带 unit 的目标包测试、目标包 vet、迁移/队列/outbox 定向测试、前端全量测试（307 文件/2145 用例）、`pnpm lint`、`pnpm build`、`git diff --check` 均通过；build 仅保留既有大 chunk warning。
