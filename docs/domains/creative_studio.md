# 创作台

TokenRouter 创作台（Creative Studio）提供面向个人用户的图片生成、编辑与局部重绘异步任务：浏览器上传素材并创建任务，服务端只保存任务元数据，worker 从 Redis 队列消费任务并调用上游图片模型，结果在 Redis 中短暂留存，由前端取回并保存到浏览器本地。

本文覆盖创作台 API、任务生命周期、幂等、隐藏执行 Key、计费、Redis 临时数据、审核无留存、前端本地存储边界、配置和检查清单。它不定义批量图片作业（见[批量图片作业](batch_image_jobs.md)），不承诺生产价格数值，也不描述上游供应商自己的内容政策。

边界声明（用户与运维都必须理解）：

- 服务端不持久化用户素材：原图、mask、生成图、prompt 明文和 provider 原始响应都不进入 PostgreSQL；prompt 只存 sha256，请求幂等指纹也是 sha256。
- 生成期间服务端临时接收并转发：素材与 prompt 明文只存在于 Redis 临时键，TTL 默认 30 分钟，到期即不可恢复。
- 上游供应商可能有自己的留存策略：素材与 prompt 会按上游 API 要求发送给对应平台，供应商侧的数据边界不受 TokenRouter 控制。
- 断线/过期后结果可能丢失且不算成功：客户端未及时取回输出时任务降级为 `result_lost`，服务端绝不明示成功，也不会从服务端恢复素材；上游已成功但结果丢失的任务仍保持计费。
- 浏览器本地存储不保证永久：输出图片只保存在当前浏览器的 IndexedDB 中，清理站点数据、换浏览器或换设备都会丢失素材，且无跨设备同步。

## 章节导航

- [API 路由](#api-路由)：说明路由、multipart 字段和请求限制。
- [生命周期](#生命周期)：说明任务状态机与 `result_lost` 语义。
- [幂等](#幂等)：说明 Idempotency-Key、请求指纹和部分唯一索引。
- [隐藏执行 Key](#隐藏执行-key)：说明托管 Key 的供应、可见性与级联。
- [计费](#计费)：说明预占、捕获、释放和结算幂等。
- [Redis 临时数据](#redis-临时数据)：说明临时键、TTL、ack 即删和队列协调。
- [审核无留存](#审核无留存)：说明创作台送审的无媒体留存模式。
- [提供商说明](#提供商说明)：说明 openai/grok/gemini 三个平台的执行契约。
- [前端本地存储边界](#前端本地存储边界)：说明 IndexedDB、收割流程和丢失边界。
- [配置](#配置)、[运维检查清单](#运维检查清单)和[安全检查清单](#安全检查清单)：说明运行时启用条件与验证要求。

## API 路由

创作台路由挂在用户 JWT 面板前缀下（`backend/internal/server/routes/user.go`），响应统一 envelope `{code, message, data}`；`POST /creative/runs` 额外经过面板 heavy 限流：

```text
GET  /api/v1/creative/models
POST /api/v1/creative/runs
GET  /api/v1/creative/runs
GET  /api/v1/creative/runs/{id}
GET  /api/v1/creative/runs/{id}/outputs/{index}/content
POST /api/v1/creative/runs/{id}/outputs/{index}/ack
POST /api/v1/creative/runs/{id}/cancel
```

`GET /creative/models` 返回当前用户可用分组与图片模型的组合（`data` 为 `{group_id, group_name, model, operations, image_sizes, price_1k}` 数组）：只包含用户可绑定、已启用图片生成、平台支持创作台操作且配置了图片尺寸价格的分组。Gemini（含 Vertex 账号）与 OpenAI 分组支持 `generate`/`edit`/`inpaint`，Grok 分组仅支持 `generate`。

`POST /creative/runs` 接受 `multipart/form-data`，只接受上传文件，不接受远程 URL：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `group_id` | 文本 | 必填，目标分组 ID |
| `model` | 文本 | 必填，须在该分组可选模型集合内 |
| `operation` | 文本 | 必填，`generate`/`edit`/`inpaint`（小写归一） |
| `prompt` | 文本 | 必填，去空白后非空且不超过 8000 字符 |
| `source_images` | 文件，可多个 | 源图；字段名兼容 `source_images`、`source_images[]`、`source_images[i]` |
| `mask` | 文件，单个 | 局部重绘蒙版，仅 `inpaint` 允许携带 |
| `image_size` | 文本 | 可选，`1K`/`2K`/`4K`，默认 `1K` |
| `aspect_ratio` | 文本 | 可选，不超过 16 字符 |
| `output_count` | 文本 | 可选，1-4，默认 1 |
| `response_mime_type` | 文本 | 可选，`image/png`/`image/jpeg`/`image/webp`，默认 `image/png` |

请求限制（默认值来自 `creative.*` 配置）：

- 单文件（源图/mask）不超过 32 MiB，单次任务输入总量（全部源图 + mask）不超过 64 MiB；multipart 单字段读取上限 40 MiB（含头部余量）。
- 上传文件 MIME 只接受 `image/png`、`image/jpeg`、`image/webp`，缺失或非法时按字节魔数嗅探。
- `edit` 必须至少携带一张源图；`inpaint` 必须携带源图和 PNG mask，且 mask 尺寸必须与第一张源图一致；非 `inpaint` 操作携带 mask 直接拒绝。
- 除 `group_id` 外的字段缺失或非法返回 `400` 系列业务错误（如 `CREATIVE_INVALID_PARAMS`、`CREATIVE_MASK_REQUIRED`、`CREATIVE_MASK_SIZE_MISMATCH`）；分组不可用返回 `403`；余额不足以预占返回 `402 CREATIVE_INSUFFICIENT_BALANCE`；审核命中返回 `403 CREATIVE_CONTENT_BLOCKED`。
- `Idempotency-Key` 请求头可选，最长 255 字符，语义见[幂等](#幂等)。

任务 ID 为 `crun_` 前缀加 16 字节随机 hex。任务公共投影包含 `id`、`status`、`model`、`requested_model`、`operation`、`requested_output_count`、`image_size`、`aspect_ratio`、`response_mime_type`、`group_id`、`estimated_cost`、`hold_amount`、`actual_cost`、错误字段、时间戳和 `outputs` 输出元数据数组（`index`、`status`、`mime_type`、`byte_size`、`transient_expires_at`、`acked_at`）；幂等重放响应额外带 `idempotent_replay=true`。

`GET /creative/runs` 按 `created_at` 倒序返回当前用户任务，支持 `status`、`limit` 查询参数，`limit` 默认 20，越界或非正数按 20 处理，返回 `data` 与 `has_more`。

`GET .../outputs/{index}/content` 在临时有效期内返回图片二进制（`Cache-Control: private, no-store`）；输出已 ack、已过期或临时键已丢失时返回 410 语义错误（`CREATIVE_OUTPUT_EXPIRED`/`CREATIVE_RESULT_LOST`），并把仍处 `succeeded` 的任务降级为 `result_lost`，绝不明示成功。

`POST .../outputs/{index}/ack` 用于客户端确认输出已保存到本地：删除对应临时输出键并把输出标记为 `acked`，重复 ack 幂等成功。

`POST /creative/runs/{id}/cancel` 取消 queued/running 任务：置 `cancelled`、释放预占并尽力清理临时键；任务已终态返回 `409 CREATIVE_RUN_NOT_CANCELLABLE`。

## 生命周期

任务状态机：

```text
queued -> running -> succeeded
queued -> running -> failed
queued -> running -> cancelled
queued -> running -> result_lost
succeeded -> result_lost
```

`succeeded`、`failed`、`cancelled`、`result_lost` 均为终态。`result_lost` 的语义必须按丢失处理，不能当作成功：

- 客户端在 TTL 内未取回并 ack 输出、worker 发现输入载荷已过期时，任务进入 `result_lost`；服务端不保留、也不能恢复图片本体。
- worker 加载不到 payload 或输入（TTL 过期，provider 未执行）时标记 `result_lost` 并释放预占；上游已确认成功但结果丢失的路径保持计费（见[计费](#计费)）。
- 输出读取路径发现临时输出过期或缺失时，把 `succeeded` 任务降级为 `result_lost`（错误码 `RESULT_EXPIRED`）并返回 410。

worker 从 Redis 预留任务后先幂等推进 `running` 并回填执行账号；执行前再次检查取消。执行期间被用户取消的任务：provider 若已成功，仍按实际成功输出捕获费用并记录用量，但终态保持 `cancelled`，绝不回写为 `succeeded`。

执行错误的重试边界：网络层错误、429 与 5xx 视为可重试，按 `max_execute_attempts`（默认 3，含首次）递增尝试并重排；其余 4xx 不可重试直接 `failed`。结算失败的按错误重试预算重排（上限为执行次数的两倍），超限后保留当前状态出队。

## 幂等

- 客户端可对 `POST /creative/runs` 携带 `Idempotency-Key` 头；同一用户同一键重放时，若请求指纹一致则直接返回原任务（`idempotent_replay=true`），不重复建单、不重复计费。
- 同一用户同一键但请求体不同（指纹不一致）返回 `409 CREATIVE_IDEMPOTENCY_CONFLICT`。
- 请求指纹是规范化 JSON（分组、模型、操作、prompt sha256、各源图 sha256、mask sha256、尺寸、比例、输出数、MIME）的 sha256；`creative_runs.request_fingerprint` 列带唯一约束，重复提交在数据库层同样被拦截。
- `(user_id, idempotency_key)` 部分唯一索引只约束非空键，幂等范围按用户隔离，键本身最长 255 字符。
- 计费与结算请求 ID 全部经由 `usage_billing_dedup` 幂等表去重，worker 重试、重复回调不会产生重复资金动作。

## 隐藏执行 Key

创作台任务需要一个 API Key 作为计费与用量归属主体，但用户不应看到或操作它，因此服务端自动供应隐藏执行 Key：

- `api_keys.managed_by` 标记托管来源，CHECK 约束只允许 `'creative_studio'` 或 NULL；任务创建时按用户 + 分组幂等供应（名称 `creative-studio:{group_id}`，`billing_mode` 固定 `auto`，停用分组回退关闭）。
- 普通 Key 列表查询在仓储层过滤 `managed_by IS NULL`；按 ID 的 get/update/delete 命中托管 Key 一律按不存在处理（404 语义），不泄露存在性。
- 创作台写 `usage_logs` 时以隐藏 Key 的 ID 满足 `usage_logs.api_key_id` 非空约束；用户删除账号时任务元数据随 `user_id` 级联删除。

## 计费

创作台复用批量图片的 UsageBillingRepository hold/capture/release 路径（`ReserveBatchImageBalance`/`CaptureBatchImageBalance`/`ReleaseBatchImageBalance`），按基础单价乘输出数估价，快照订阅/余额倍率；没有批量折扣与账号倍率。资金动作的请求 ID 前缀固定，全部经 `usage_billing_dedup` 幂等：

```text
creative_hold:{run_id}      创建任务时预占
creative_capture:{run_id}   成功时按成功输出数捕获
creative_release:{run_id}   失败/取消/未执行丢失时释放
creative_settle:{run_id}    写 usage_logs 的结算记录 ID
```

- 创建任务时先估价并冻结（auto 模式先预留订阅额度、只冻结未覆盖部分的钱包余额）；免费分组（估价为 0）跳过资金动作。
- 成功时按实际成功输出数捕获，并写一条 `usage_logs`：`BillingMode` 为 `"image"`，`ImageCount` 为成功输出数，`request_id` 为 `creative_settle:{run_id}`，入站端点记录 `/v1/creative/runs`，上游端点记录 `creative:{operation}`。
- 失败与取消通过幂等路径释放全部未使用预占；指纹冲突按已释放处理，避免毒消息循环。
- provider 已成功但结果丢失（`result_lost` 且已捕获）时保持计费；payload 过期导致 provider 未执行的 `result_lost` 释放预占。
- 执行期间被取消但 provider 已成功：费用按实际成功输出捕获、用量照写，终态保持 `cancelled`。

生产准确价格由分组图片定价配置解析，本文不定义价格数值。

## Redis 临时数据

输入载荷、源图字节、mask 与输出图片本体只保存在 Redis 临时键，TTL 为 `creative.transient_ttl_seconds`（默认 1800 秒）；PostgreSQL 只存任务与输出元数据。Redis 不可用时创建任务 fail-close 拒绝：

| 键 | 内容 | 清理 |
| --- | --- | --- |
| `creative:payload:{run_id}` | 任务执行载荷 JSON（含 prompt 明文、模型、操作、计数、指纹；图片字节不内联） | TTL 或 `DeleteRunTransient` |
| `creative:input:{run_id}:{idx}` | 单张源图字节 | TTL 或 `DeleteRunTransient` |
| `creative:mask:{run_id}` | mask 字节 | TTL 或 `DeleteRunTransient` |
| `creative:output:{run_id}:{index}` | 单张生成图字节 | TTL、ack 即删或 `DeleteRunTransient` |

输出保存时同时把 `transient_expires_at` 写入输出元数据，客户端据此知道取回截止时间；ack 立即删除对应输出键。

队列协调（`creative:queue:*`）与批量图片同构：ready 列表、delayed 有序集合、active 有序集合、单任务 inflight 键（默认 TTL 7 天）、单任务锁键（默认 TTL 300 秒）；入队与预留用 Lua 脚本原子执行，重排/确认用事务管道。`creative.queue_enabled` 默认开启，应用启动时运行 worker、delayed mover 和 stale active recovery 三个循环；worker 从 Redis 预留任务并持锁执行，处理中按心跳续期锁并刷新 active 时间戳，超时未心跳的 active 任务由恢复循环重投。

## 审核无留存

创建任务时，prompt 与全部上传图（含 mask）会构造 OpenAI Images 协议报文送内容审核，且必须开启 `ContentModerationCheckInput.NoMediaRetention`：

- 无媒体留存模式下审核日志只保留输入 hash、分类、分数和决策等元数据；不做命中媒体快照，不落 `input_excerpt` 与正文输入项，因此审核记录与 Ops 日志不会保存 base64 图片或 prompt 明文。
- 审核服务自身失败不阻断创作台（fail-open 记日志）；命中阻断时返回 `403 CREATIVE_CONTENT_BLOCKED`。

审核模式、规则优先级与失败语义的通用约定见[内容审核与风险处置](content_moderation.md)。

## 提供商说明

执行器按分组平台直接构造上游 HTTP 请求，不经过本地 HTTP 回环；执行超时为 `creative.execute_timeout_seconds`（默认 300 秒）。单张输出不超过 32 MiB，同一任务内按 sha256 去重重复输出：

- `openai`：`generate` 走 `/v1/images/generations`（JSON）；`edit`/`inpaint` 走 `/v1/images/edits`（multipart，多源图 + mask）。
- `grok`：仅 `generate`（xAI images generations，OpenAI 协议兼容）；`edit`/`inpaint` 直接拒绝。
- `gemini`：统一使用原生 `generateContent`，prompt 与源图/mask 以 inlineData 放入 parts；`inpaint` 时 mask 作为额外 inline 图片附加。凭据按账号类型选择：API Key 账号用 `x-goog-api-key`，Vertex 服务账号与 OAuth 用 Bearer token。

模型候选：Gemini 复用批量图片的账号模型映射展开（含 Vertex）；OpenAI 候选为 `gpt-image-1`/`gpt-image-2`；Grok 候选为 `grok-imagine` 系列。

## 前端本地存储边界

前端 `/creative` 页面要求登录，simple 模式隐藏入口，侧栏可见性取决于 `GET /creative/models` 是否返回非空（`getCreativeModels`）：

- 输出收割：任务轮询前 10 秒每 1 秒、之后每 3 秒；终态为 `succeeded` 时逐个取回未 ack 的输出，先写入 IndexedDB 再调用 ack；单个输出取回失败（410/`result_lost`）只标记该输出缺失，不中断其它输出。
- 本地存储：IndexedDB 库名 `tokenrouter-creative-studio`（版本 1），对象仓库为 `assets`（源图/mask/输出 blob）、`scenes`（画布 JSON 快照）和 `settings`（参数选择恢复）；图片绝不以 base64 进入 localStorage。
- 丢失边界：服务端标记成功但本地无对应 blob 的输出显示“素材缺失”，前端不会向服务端重新拉取恢复；本地配额不足时提示用户下载备份；页面顶部常驻隐私提示条；清理浏览器站点数据会清空全部本地素材，且没有任何跨设备同步。
- 幂等重试：创建任务失败重试复用同一 Idempotency-Key，成功后重置。

## 配置

以下配置键定义在 `backend/internal/config/config.go`（默认值与 `deploy/config.example.yaml` 一致）：

```yaml
creative:
  enabled: true                       # 创作台 HTTP API 开关
  queue_enabled: true                 # 创作台队列 worker 开关
  transient_ttl_seconds: 1800         # 输入载荷与临时输出保留时间（秒）
  max_asset_bytes: 33554432           # 单文件上传上限（32 MiB）
  max_total_input_bytes: 67108864     # 单次任务输入总量上限（64 MiB）
  max_output_count: 4                 # 单次任务最大输出数量（1-4）
  max_prompt_chars: 8000              # prompt 最大字符数
  default_response_mime_type: "image/png"
  default_image_size: "1K"
  queue_ready_key: "creative:queue:ready"
  queue_delayed_key: "creative:queue:delayed"
  queue_active_key: "creative:queue:active"
  inflight_key_prefix: "creative:queue:inflight:"
  lock_key_prefix: "creative:queue:lock:"
  inflight_ttl_seconds: 604800
  job_lock_ttl_seconds: 300
  default_requeue_delay_seconds: 30
  error_retry_delay_seconds: 60
  lock_conflict_delay_seconds: 5
  stale_active_after_seconds: 600
  delayed_mover_interval_seconds: 5
  recovery_interval_seconds: 300
  delayed_move_limit: 100
  recover_limit: 100
  execute_timeout_seconds: 300        # 单次上游执行调用超时
  max_execute_attempts: 3             # provider 瞬时错误最大执行次数（含首次）
```

校验约束：`max_total_input_bytes` 不得小于 `max_asset_bytes`；`max_output_count` 必须在 1-4；启用队列时所有队列键非空。与批量图片不同，创作台的 `enabled` 与 `queue_enabled` 默认开启，但缺少 Redis 时任务创建会失败。

## 运维检查清单

- 确认 Redis 可用（临时存储与队列都依赖 Redis）。
- 确认 `creative.enabled` 与 `creative.queue_enabled`。
- 确认目标分组启用图片生成、配置图片尺寸价格，且账号模型映射包含图片模型。
- 确认上游账号凭据有效（Gemini apikey/Vertex/OAuth、OpenAI、xAI）。
- 确认分组图片定价与倍率，验证估价的 hold/capture/release 行为。
- 明白临时输出默认 30 分钟过期：通知用户及时取回，或按需调大 `transient_ttl_seconds`。
- 排查 `result_lost` 时先检查客户端是否在 TTL 内完成取回与 ack，再检查 worker 日志。

## 安全检查清单

- PostgreSQL 不保存图片字节、mask、prompt 明文或 provider 原始响应；prompt 只存 sha256，幂等指纹同样不可逆。
- 日志与审核记录不含 base64 图片或 prompt 明文（审核走无媒体留存模式）。
- 备份不包含创作台素材本体（素材不在 PostgreSQL 中）；恢复数据库不会恢复 Redis 临时输出。
- 全部路由按用户隔离资源归属；隐藏执行 Key 不暴露存在性。
- 取消与失败路径释放预占并清理临时键；ack 即删输出。
- 不向客户端泄露上游凭据、代理或 provider 原始响应；错误消息截断到 500 字符并脱敏。

相关 Project Doc：[批量图片作业](batch_image_jobs.md)、[内容审核与风险处置](content_moderation.md)、[路由与结算](routing_and_billing.md)、[HTTP 接口边界](../interfaces/http_api.md)、[配置边界](../interfaces/configuration.md)、[系统架构](../architecture/system_architecture.md)、[可观测性与数据生命周期](../operations/observability_and_data_lifecycle.md)和[领域目录](index.md)。
