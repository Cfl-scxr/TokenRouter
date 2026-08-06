# Anthropic 上游

本文描述 Anthropic 平台账号、认证、协议转换、模型与缓存策略、额度限制和失败恢复边界。它不固化上游动态模型清单，也不替代公共网关请求生命周期或用户接入教程。

## 章节导航

- [账号与认证](#账号与认证)：修改 OAuth、Setup Token、API Key、Bedrock 或 Vertex 凭据时读取。
- [协议分派](#协议分派)：修改 Messages、Responses 或 Chat 转换时读取。
- [模型与请求策略](#模型与请求策略)：修改模型映射、thinking、beta 或缓存行为时读取。
- [配额与调度](#配额与调度)：修改账号资格、粘性、等待或成本信号时读取。
- [错误与诊断](#错误与诊断)：修改刷新、重试、故障转移或错误响应时读取。

<a id="anthropic_account_and_transport"></a>
## 账号与认证

Anthropic 管理端正式支持以下账号：

| 类型 | 上游认证和边界 |
| --- | --- |
| `oauth` | 使用 access/refresh token；后台刷新服务和请求路径 token provider 都可刷新临近过期凭据 |
| `setup-token` | 使用推理范围的 setup token；按保存的 access token 转发，不等同于可刷新完整 OAuth 账号 |
| `apikey` | 使用 Anthropic API Key 或配置的 Bearer 方案，可配 base URL、Header override 和上游计费探测 |
| `bedrock` | `sigv4` 使用 AWS 凭据和区域签名；`apikey` 使用 Bedrock API Key；可配置全局端点和模型映射 |
| `service_account` | 使用 Google Service Account 换取 Vertex AI token，并携带 project/location 等 Vertex 上下文 |

`upstream` 和其它历史类型即使能被通用导入器保存，也没有 Anthropic 平台的正式 token provider 契约。完整分类见[上游账号能力矩阵](upstream_account_matrix.md)。所有 base URL、代理和自定义 Header 仍受[上游传输安全](../operations/upstream_transport_security.md)约束。

## 协议分派

Anthropic 原生入口是 `POST /v1/messages` 和 `POST /v1/messages/count_tokens`。同一 Anthropic 分组还可从 OpenAI Chat Completions 和 Responses 入口进入：处理器先把客户端形状归一化为 Anthropic 请求，按 attempt 选账号并转发，再把非流或 SSE 结果恢复成原协议。

API Key 和 OAuth/Setup Token 使用 Anthropic HTTP 路径；Bedrock 走独立签名与响应适配；Service Account 走 Vertex Claude 路径。协议转换不能抹平这些传输差异，尤其是 beta header、模型名称、错误结构和 token usage 的来源。

流式请求只在首个客户端分块写出前允许重试或换账号。每次 attempt 都从原始请求重建转换状态，工具名、停止原因、thinking block、usage 和错误事件必须与客户端协议一致。

## 模型与请求策略

模型依次经过 Key 重定向、渠道映射和账号映射；可请求列表是分组策略、渠道和当前账号能力的交集，不是默认模型常量的直接输出。Bedrock/Vertex 的供应商模型标识可与客户端 Anthropic 名称不同，计费模型也可以由渠道单独指定。

Anthropic 请求策略包括：

- beta header 过滤、补充或阻断，避免把账号不允许的实验能力直接发往上游。
- thinking、tool use、图片和长上下文的协议保真；不同入站协议的推理字段先归一化。
- prompt caching、cache TTL 注入和消息缓存重写；缓存读写 token 进入用量与定价，而不是仅作为诊断字段。
- 可选 web search emulation、Claude Code 客户端约束、metadata/header 策略和长上下文计价。

具体启用条件可能来自全局运行设置、分组/渠道和账号 extra。层级边界见[网关策略控制](../domains/gateway_policy_controls.md)。

## 配额与调度

账号必须通过状态、分组、模型、endpoint、凭据、限流和并发筛选。粘性会话尽量复用同一账号；账号失效、模型限流或策略变化会丢弃旧绑定并重新选择。等待队列只等待可能恢复的并发/限流条件，不会把永久凭据错误变成无限等待。

API Key/Bedrock 可配置本地账号配额和亲和策略。上游 OAuth/API Key 用量探测、账号成本、优先级与近期错误可以参与调度，但不替代用户余额、订阅和用户平台额度。Antigravity 账号只有显式启用 mixed scheduling 后才能加入 Anthropic 候选，并继续遵守 Anthropic 分组语义。

## 错误与诊断

凭据临近过期优先刷新；刷新失败会更新账号错误状态并同步调度快照。401/403 需要区分 token 失效、权限或 beta/模型拒绝；429 需要区分账号、模型和共享容量；可重试 5xx/网络错误只在响应未开始时换账号。

最终错误先经过平台分类，再应用管理员配置的[网关错误响应策略](gateway_error_policy.md)。错误正文、凭据、内部 project/region 和上游标识不得无条件返回客户端。排障应关联 request ID、requested/upstream model、账号 attempt、token refresh、代理/TLS 路由、限流恢复时间和结算记录。

相关文档：[网关请求生命周期](../architecture/gateway_request_lifecycle.md)、[账号调度与缓存一致性](../architecture/account_scheduling_and_cache.md)、[账号维护](../operations/account_maintenance.md)。
