# Grok / xAI 上游

TokenRouter 支持 Grok OAuth 订阅账号和标准 xAI API Key 账号，并通过 OpenAI 兼容的 Responses、Chat Completions、Messages 和 WebSocket 入口转发请求。Grok 分组还支持图片生成/编辑、视频生成/编辑/扩展以及视频状态查询。

本文覆盖账号凭据、聊天/媒体转发、媒体资格、异步视频归属、模型目录和运行时变量，不定义 xAI 套餐价格，也不把上游当前返回的所有动态模型固化为兼容承诺。

## 章节导航

- [基本信息](#基本信息)：修改路由或默认上游时读取。
- [账号配置](#账号配置)：修改 OAuth/API Key 导入与刷新时读取。
- [客户端协议](#客户端协议)：修改 Responses、Chat 或 Messages 准入时读取。
- [媒体请求格式](#媒体请求格式)：修改图片/视频 body 转换时读取。
- [媒体账号资格](#媒体账号资格)：修改付费探测和调度隔离时读取。
- [任务归属与结算](#任务归属与结算)：修改视频查询、下载或用量记录时读取。
- [客户端配置](#客户端配置)：核对生成给客户端的 base URL。
- [默认模型目录](#默认模型目录)：修改内置模型与别名时读取。
- [环境变量](#环境变量)：修改 OAuth 或上游运行参数时读取。

## 基本信息

- 平台名：`grok`
- 账号类型：OAuth 订阅账号、API Key 账号
- 主要网关入口：`/v1/responses`、`/responses`、Chat Completions、Messages 和 Responses WebSocket
- API Key 账号默认上游地址：`https://api.x.ai/v1`

## 客户端协议

Grok 分组支持 Anthropic Messages、OpenAI Responses 和 Chat Completions。Responses 与 Chat 是不可关闭的基础协议和新建默认值；迁移前已有分组启用三项。文本协议禁用时会在账号选择、计费、重试和 fallback 前返回协议原生 `403`。

Responses WebSocket 是 Grok/OpenAI 的原生传输能力，不由兼容 Responses 开关扩展到其它平台。图片和视频继续使用独立媒体资格与分组策略，不受文本协议集合直接控制。

<a id="grok_account_contract"></a>
## 账号配置

管理员可在控制台选择 OAuth 或 API Key 创建账号。OAuth 账号可通过控制台创建或重新授权；创建 Grok 分组并绑定账号后，用户即可生成分组 API Key。

其它通用账号类型即使可由兼容导入层保存，也没有 Grok 正式凭据和转发契约；`cosy` 明确只属于 Qoder。完整分类见[上游账号能力矩阵](upstream_account_matrix.md)。

## 媒体请求格式

JSON 图片编辑和视频生成请求可在 `image`、`images`、`reference_images` 与 `mask` 对象中提供参考图片。与 xAI 直接兼容的请求应使用 `url` 字段；历史 `image_url` 字段仍可使用，TokenRouter 会在转发前把它规范化为 `url`。如果两者同时存在，则保留非空的 `url`；空白 `url` 会回退使用 `image_url`。multipart 图片编辑中的上传文件也会转换为 `url` 形式的 data URL。

## 媒体账号资格

新的 Grok 图片或视频生成请求会执行媒体专用的账号资格检查。API Key 账号保持可用；OAuth 账号必须由 xAI 计费探测提供明确的付费资格证据。Free、禁止访问、缺少观测、观测格式错误或结论不明确的 OAuth 账号都不会承接新的媒体生成请求。尚无观测的 OAuth 账号会在第一次转发媒体请求前执行探测，导入账号时也会主动先执行计费探测。聊天请求和已有视频任务的状态查询不受这项隔离影响。当分组中没有合格账号时，媒体端点返回 HTTP `503`，错误类型为 `grok_media_no_eligible_account`。

管理员可通过账号创建或更新 API 的 `extra.grok_media_eligible` 覆盖自动判定：`false` 表示排除，`true` 表示强制允许；更新时传 `null` 可删除覆盖并恢复基于探测结果的自动判定，省略该字段则保留现有覆盖。仅出现每周用量周期不能作为付费层级证据。图片接口返回成功时必须包含至少一个实际图片输出；空的 HTTP `200` 响应会触发账号 failover，不会作为成功生成结果计数或返回。

## 任务归属与结算

新视频请求成功后按 `request_id + user_id + api_key_id` 保存所选分组和账号绑定。后续状态与 content 下载必须回到创建任务的账号，不能重新随机调度；复合 Key 的映射后来被删除时，服务仍可从持久/缓存绑定构造只用于旧任务查询的最小 Grok 分组视图。查询已有任务不要求账号仍具备“新媒体生成”资格，但仍校验 Key、用户和任务归属。

视频 content 先确认任务状态，再使用服务端上游凭据代理下载，并安全透传 Range/内容头；上游 URL 和 bearer token 不返回客户端。生成/编辑类成功结果进入标准用量记录和结算，查询/下载不重复计费。模型重定向、渠道映射和响应模型恢复遵守共同模型链，媒体专用路由模型只用于能力选择，不能覆盖用户账单中的 requested/upstream model。

OAuth 凭据失效、账号资格变化和上游限流使用带凭据快照的分类与 CAS 更新，避免旧请求把刚刷新的账号再次封禁。内容策略 403 与凭据 401/403、付费资格拒绝和可切换上游错误要分别处理；只有可切换且响应未开始的错误进入下一账号。

## 客户端配置

用户可在 API Key 页面通过“使用密钥”生成 Grok Build CLI 或 OpenCode 配置。现有 `config.toml` 应先备份，再合并新模型配置。

Grok Build CLI 的模型配置必须指向 TokenRouter 对外地址（以 `/v1` 结尾），不能直接使用 `api.x.ai` 或内部 OAuth 代理地址。OAuth 流量默认转发到 Grok CLI 订阅代理。

## 默认模型目录

- `grok-4.5`
- `grok-4.3`
- `grok-build-0.1`
- `grok-composer-2.5-fast`
- `grok-4.20-0309-reasoning`
- `grok-4.20-0309-non-reasoning`
- `grok-4.20-multi-agent-0309`
- `grok-imagine`
- `grok-imagine-image`
- `grok-imagine-image-quality`
- `grok-imagine-edit`
- `grok-imagine-video`
- `grok-imagine-video-1.5`

`grok`、`grok-latest` 和 `grok-4.5-latest` 归一化为 `grok-4.5`，其它内置别名由 `internal/pkg/xai/models.go` 维护。模型列表默认展示当前目录，并结合账号模型映射/范围和 API Key 别名；未知模型保持透传，以支持管理员配置的 xAI 兼容上游。

## 环境变量

- `XAI_OAUTH_CLIENT_ID`
- `XAI_OAUTH_SCOPE`
- `XAI_OAUTH_REDIRECT_URI`
- `XAI_OAUTH_AUTHORIZE_URL`
- `XAI_OAUTH_TOKEN_URL`
- `XAI_BASE_URL`
- `XAI_GROK_CLI_VERSION`：覆盖 Grok CLI 客户端版本，配置值不得低于内置支持版本

自定义 base URL 和媒体/billing 子路径都必须通过同一 URL allowlist/SSRF 校验。环境变量中的 client secret、token 和上游 URL 不得进入前端配置或错误响应。

相关文档：[上游账号能力矩阵](upstream_account_matrix.md)、[网关请求生命周期](../architecture/gateway_request_lifecycle.md)、[路由与结算](../domains/routing_and_billing.md)、[HTTP 接口边界](http_api.md)、[接口目录](index.md)。
