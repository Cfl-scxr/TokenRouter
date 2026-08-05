# 异步图片任务

异步图片任务允许客户端提交耗时较长的 OpenAI 兼容图片请求，而不必一直保持 HTTP 连接。OpenAI 和 Grok 的图片生成或编辑请求可以提交到 `/v1/images/generations/async` 或 `/v1/images/edits/async`，再通过 `/v1/images/tasks/{task_id}` 轮询。这样可以避免 Cloudflare 524 等代理或 CDN 响应超时，同时保留现有图片路由、计费、内容审核、并发和故障转移行为。

本文拥有异步提交、轮询、Redis 状态、对象存储卸载、超时和所有权契约。提供商选择和计费仍由同步图片网关拥有。该功能只是进程内执行包装器，不是持久任务队列：已接受任务会保存在 Redis，但进程重启不会恢复它的 goroutine。

## 章节导航

- [端点](#端点)：修改路由注册或支持平台时读取。
- [启用功能](#启用功能)：修改运行时设置或 S3 兼容存储时读取。
- [任务生命周期](#任务生命周期)：修改提交、执行、状态、所有权、TTL 或计费行为时读取。

## 端点

已认证网关同时公开 `/v1` 路径和现有的无前缀别名：

```text
POST /v1/images/generations/async
POST /v1/images/edits/async
GET  /v1/images/tasks/{task_id}
```

对应别名是 `/images/generations/async`、`/images/edits/async` 和 `/images/tasks/{task_id}`。

只支持 OpenAI 和 Grok 分组。请求使用与对应同步端点相同的 JSON 或 multipart 载荷。轮询任务只返回一个最终 JSON 结果，因此拒绝流式图片请求。

## 启用功能

异步图片任务默认关闭，并受对象存储配置约束。开关关闭或 S3 凭据不完整时，异步端点返回 `404`，不会创建任务或写入 Redis。这是有意的安全边界：如果不把结果卸载到对象存储，`gpt-image-1` 等模型每张可能达到数 MB 的 `b64_json` 会积压在 Redis 并耗尽内存。

### 从管理界面配置（推荐）

进入“管理后台 -> 备份 -> 异步图片对象存储”。保存后立即生效；对象存储客户端会在下一次请求时重建，不需要重启容器。

表单可以复用备份 S3 配置：复用端点、区域和凭据，同时保留图片专用存储桶和前缀。图片存储桶留空时也会复用备份存储桶。关闭复用后可以使用独立的 S3 兼容账号。

启用强认证门禁时，保存操作需要二次 TOTP 验证，原因与备份 S3 表单相同：改变目标位置会把生成内容重定向到另一个账号。

关闭开关只会停止新提交，已经接受的任务仍可轮询，不会让执行中的任务失去查询入口。

### 从配置文件配置

管理设置优先。数据库中从未保存过该设置时，使用 `config.yaml` 的 `image_storage` 配置块，从而保持旧部署的既有行为。

在 `config.yaml` 中配置 S3 兼容对象存储，例如 AWS S3、Cloudflare R2、阿里云 OSS 或 MinIO。所有键也接受对应的 `IMAGE_STORAGE_*` 环境变量覆盖：

```yaml
image_storage:
  enabled: true
  endpoint: "https://<account_id>.r2.cloudflarestorage.com"  # AWS 官方服务可留空
  region: "auto"
  bucket: "my-images"
  access_key_id: "..."
  secret_access_key: "..."
  prefix: "images/"
  force_path_style: false          # MinIO 或路径风格存储桶设为 true
  public_base_url: ""              # 非空时返回 public_base_url/key 直链，否则返回预签名 URL
  presign_expiry_hours: 24         # public_base_url 为空时的预签名链接有效期
  max_download_bytes: 33554432     # 转存上游图片 URL 时的大小上限，默认 32 MB
```

任务完成时，每张图片都会上传到存储桶，结果会重写为紧凑形式：`data[].url` 指向存储对象，可以是永久的 `public_base_url/key` 链接或限时预签名 URL；`b64_json` 会被删除。Redis 只保存这份小型 JSON。上传失败时任务标记为 `failed`，不会把原始 Base64 保存到 Redis。

需要支持 S3 兼容客户端之外的供应商时，应实现 `service.ImageStorage` 接口，即 `Save(ctx, key, contentType, data) (url, error)`，并替换 S3 实现。

### 启用后端点仍返回 404

`404 async image tasks are not enabled` 表示 `image_storage` 没有解析成完整配置，因此功能仍处于关闭状态。路由始终存在，404 来自处理器而不是路由未注册，容易被误判为构建缺失。

检查启动日志：

```text
WARN image_storage.enabled is true but object storage is not fully configured; async image tasks are disabled  missing_keys=[...]
```

`missing_keys` 会准确列出加载配置时为空的凭据。

每个 `image_storage` 键都注册了默认值，并可通过对应的 `IMAGE_STORAGE_*` 环境变量访问。新增字段时必须同步维护配置环境变量可达性测试，否则 Viper 可能静默忽略没有已知配置键的环境变量。

另有两类与存储无关的 404：API Key 所属分组必须是 OpenAI 或 Grok 平台，其他平台或没有分组的 Key 会返回 `Images API is not supported for this platform`；任务只能由提交它的同一 API Key 轮询，同一用户的其他 Key 也会按设计返回 `image task not found`。

<a id="task_lifecycle"></a>
## 任务生命周期

已接受请求会从 `processing` 转为 `completed` 或 `failed`。提交时先创建新的 `imgtask_...` 标识和 Redis 记录，再启动进程内 goroutine。该接口不使用 `Idempotency-Key` 去重；重复提交会创建另一个任务，可能再次生成图片并计费。

goroutine 使用与同步图片端点相同的规范化请求、API Key 上下文、模型映射、账号调度、故障转移、用量记录和结算路径。它不受客户端断开影响，但受 30 分钟执行超时约束。轮询只读取任务状态，不会再次请求上游或结算。

### 提交任务

```bash
curl -i https://api.example.com/v1/images/generations/async \
  -H 'Authorization: Bearer sk-...' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "gpt-image-1",
    "prompt": "冬季风暴中的灯塔",
    "size": "1536x1024"
  }'
```

服务端把初始任务保存到 Redis，并返回 `202 Accepted`：

```json
{
  "id": "imgtask_0123456789abcdef",
  "task_id": "imgtask_0123456789abcdef",
  "object": "image.generation.task",
  "status": "processing",
  "created_at": 1784092800,
  "expires_at": 1784179200,
  "poll_url": "/v1/images/tasks/imgtask_0123456789abcdef"
}
```

`Location` 包含轮询路径，`Retry-After: 3` 给出建议轮询间隔。

### 轮询任务

使用提交任务时的同一 API Key：

```bash
curl https://api.example.com/v1/images/tasks/imgtask_0123456789abcdef \
  -H 'Authorization: Bearer sk-...'
```

任务执行期间返回：

```json
{
  "id": "imgtask_0123456789abcdef",
  "task_id": "imgtask_0123456789abcdef",
  "object": "image.generation.task",
  "status": "processing",
  "created_at": 1784092800,
  "expires_at": 1784179200
}
```

成功后，`result` 与同步图片 API 正文一致，但每张图片都已卸载到对象存储：`data[].url` 指向存储对象，`b64_json` 被删除，因此上游 URL 和 Base64 两种格式最终都会变成紧凑存储链接。

```json
{
  "id": "imgtask_0123456789abcdef",
  "task_id": "imgtask_0123456789abcdef",
  "object": "image.generation.task",
  "status": "completed",
  "http_status": 200,
  "image_url": "https://...",
  "result": {
    "created": 1784092923,
    "data": [{"url": "https://..."}]
  },
  "created_at": 1784092800,
  "completed_at": 1784092923,
  "expires_at": 1784179323
}
```

对于 URL 结果，`image_url` 会复制第一个 `data[].url`，便于简单客户端读取。失败时任务进入 `failed`，并在可用时公开原始 OpenAI 兼容错误对象：

```json
{
  "id": "imgtask_0123456789abcdef",
  "task_id": "imgtask_0123456789abcdef",
  "object": "image.generation.task",
  "status": "failed",
  "http_status": 502,
  "error": {
    "type": "api_error",
    "message": "Upstream request failed"
  },
  "created_at": 1784092800,
  "completed_at": 1784092923,
  "expires_at": 1784179323
}
```

所有提交和轮询响应都包含 `Cache-Control: no-store`，防止 CDN 缓存 `processing` 状态。任务及结果在最近一次状态更新 24 小时后过期，单个任务最多执行 30 分钟。Redis 中的结果元数据上限为 256 KiB，错误信息上限为 64 KiB；过大或无效结果会转为安全的 `failed` 状态。

任务所有权同时受用户和 API Key 约束。未知任务 ID 与属于其他 Key 的任务都返回 `404`，避免泄露任务是否存在。即使已完成的生成耗尽了该 Key 的剩余余额，轮询仍然可用；普通认证、禁用 Key、用户、IP 和分组检查仍会执行。

相关文档：[网关请求生命周期](../architecture/gateway_request_lifecycle.md)、[路由与结算](../domains/routing_and_billing.md)、[配置边界](configuration.md)和[接口目录](index.md)。
