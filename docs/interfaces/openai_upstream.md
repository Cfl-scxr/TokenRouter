# OpenAI 上游

本文描述 OpenAI OAuth/API Key 账号，以及 Responses、Chat、Messages、Embeddings、Images、Realtime 和 Codex 兼容能力的当前契约。它不枚举会随上游变化的完整模型列表，也不把所有 OpenAI 形状的入口都解释为任意平台可用。

## 章节导航

- [账号与凭据](#账号与凭据)：修改 OAuth、API Key、隐私或客户端限制时读取。
- [协议与传输](#协议与传输)：修改 Responses、WebSocket、Realtime 或兼容转换时读取。
- [模型与能力](#模型与能力)：修改模型别名、endpoint capability 或推理参数时读取。
- [额度与调度](#额度与调度)：修改窗口配额、评分、粘性或自动暂停时读取。
- [失败与诊断](#失败与诊断)：修改错误分类、刷新、CAS 状态或 failover 时读取。

## 账号与凭据

OpenAI 正式支持 `oauth` 与 `apikey`。OAuth 账号保存 access/refresh token、账号/组织上下文和 Codex 能力元数据，后台与请求路径都可触发刷新；API Key 账号保存 key、base URL 和可探测的 endpoint capability。其它通用导入类型不构成 OpenAI 转发支持，详见[上游账号能力矩阵](upstream_account_matrix.md)。

OAuth 账号可受 Codex CLI-only、允许客户端、agent identity、privacy status 和 OAuth passthrough 策略限制。API Key 账号不应借用 OAuth-only 的内部端点或身份元数据。Header override、代理、base URL 和 TLS 配置属于出站安全边界，不能覆盖受保护认证头或绕过目标校验。

<a id="openai_protocol_dispatch"></a>
## 协议与传输

OpenAI 平台拥有以下正式协议族：

| 协议 | 处理边界 |
| --- | --- |
| Responses HTTP/SSE | 原生 OAuth/API Key 转发；支持允许的 `/responses/*` 子路径 |
| Responses WebSocket | 根据账号 transport capability 选择 WS 或兼容传输；连接建立后遵守流式不可换账号边界 |
| Chat Completions | 可原生转发或转换到 Responses；每次 attempt 重建协议状态 |
| Anthropic Messages | 转换到 OpenAI 请求并把事件、工具、thinking/usage 恢复为 Anthropic 形状 |
| Embeddings | 仅 OpenAI 分组，账号必须声明或探测到相应 endpoint capability |
| Images | OpenAI 图片生成/编辑；当前网关保留同步生命周期，批量图片由 Gemini/Vertex 专题定义 |
| Realtime/Live/sideband、Alpha Search | 仅 OpenAI 分组，并受分组开关、账号类型和 transport capability 限制 |

OpenAI 分组支持 Messages、Responses 和 Chat，新建时默认启用 Responses 与 Chat；三项都可关闭。已有分组迁移时仅在旧 `allow_messages_dispatch` 开启时加入 Messages。该旧字段只作为 Messages 的弃用兼容镜像，专用 `messages_dispatch_model_config` 仍只负责 Claude 到 GPT 模型映射。Responses WebSocket 是 OpenAI/Grok 的原生传输能力，不因其它平台启用兼容 Responses 而开放。

`/backend-api/codex` 和无 `/v1` 别名服务特定客户端兼容，但仍经过 TokenRouter Key 鉴权、分组准入、调度和结算。Responses WebSocket 不支持 Qoder；其它平台是否可进入 OpenAI 兼容处理器由路由和平台专题共同决定，不能仅凭 URL 推断。

OAuth passthrough 的 Codex 请求可以省略 `instructions`，网关会按请求模型补入内置 Codex 基础指令；显式提供的非空字符串保持不变，空白或非字符串值仍在本地拒绝。该规则同时适用于 Responses SSE 与 Compact 请求。

OpenAI OAuth 的普通 Responses 请求默认原样保留 Codex namespace 工具声明，并保留 `function_call`、`tool_call`、`custom_tool_call`、`mcp_tool_call` 历史项上的 `namespace`；普通消息等非调用项上的残留字段仍会清理。Compact 请求始终摊平 namespace 并移除输入项字段，API Key 出口也按标准 Responses schema 清理。仅当 OAuth 账号的兼容中转不接受 namespace 时，才应启用账号 `extra.openai_responses_flatten_namespaces=true` 恢复平名行为。每次 failover attempt 都会清空上一账号登记的平名映射，避免响应还原状态串到下一账号。

Responses 请求降级到 Chat Completions 时，工具结果中的 `input_image`、`image_url` 和完整图片 data URL 不能留在只接受文本的 `tool` message。转换器会按 `call_id` 从工具结果中提取图片，把原位置替换为稳定标记，并在对应的一组工具回复后追加用户多模态消息；并行调用按工具声明顺序归属图片，孤儿或未回答调用不携带媒体。没有可识别图片的工具结果必须保留原始字节，避免无关 JSON 重编码改变提示缓存前缀。

## 模型与能力

客户端模型先经过 Key、渠道和账号层映射。OpenAI 内置别名、reasoning effort 归一化、compact 支持、图像/embedding 能力和传输能力会影响候选账号；模型列表只公开当前分组可请求的结果。

API Key endpoint capability 可通过探测或配置表达 `responses`、`chat_completions`、`embeddings` 等能力。OAuth/Codex 账号还可能包含 Realtime、WebSocket、compact 和客户端身份限制。未知模型可以在管理员明确配置的兼容上游中透传，但没有定价或能力证据时不能虚构价格与功能。

## 额度与调度

OpenAI 使用专用账号调度器，在共同 active/schedulable、分组、模型、限流和并发筛选之外，还会考虑所需 transport/capability、账号优先级、负载、排队、错误率、近期延迟、配额余量和粘性上下文。previous response、WebSocket 会话和显式 session 可约束账号复用；只有策略允许时才能迁移。

OAuth 账号的 5 小时、7 天等上游窗口和重置时间保存在账号运行状态中，可触发临时限流或自动暂停；API Key endpoint capability 仍可独立探测。OpenAI 不再采集上游站点声明倍率，也不按该值进行低倍率优先或高级评分。账户本地 `rate_multiplier` 和渠道上游计费模型来源继续用于 TokenRouter 结算，但都不是用户余额、订阅、Key 限额或用户平台额度。

## 失败与诊断

账号状态更新使用凭据快照/CAS，避免较早请求在 token 已刷新后再次封禁账号。401/403、429、endpoint 不支持、内容策略、网络错误和上游 5xx 分别分类；只有可切换且客户端响应未开始的失败才进入下一账号。Responses HTTP 与 WebSocket v2 首次发送时保留加密 reasoning/compaction；若上游明确返回 `invalid_encrypted_content`，同账号恢复最多重试一次，清理账号绑定的加密状态但保留未加密 compaction。

流式错误要保持 SSE/WebSocket 协议完整；Responses 可产生 `response.failed`，非流接口返回相应 OpenAI envelope。客户端尚未收到业务输出时，池模式账号的瞬态流内处理错误可在请求级预算内重试同一账号；一旦输出开始，网关不得重放请求或切换账号。最终错误还可命中[网关错误响应策略](gateway_error_policy.md)，但规则不会把失败结算成成功。排障应同时检查账号类型、required transport/capability、客户端限制、privacy status、模型映射、quota reset、代理/TLS 和 attempt 记录。

相关文档：[网关请求生命周期](../architecture/gateway_request_lifecycle.md)、[账号调度与缓存一致性](../architecture/account_scheduling_and_cache.md)、[模型目录与市场](model_catalog_and_marketplace.md)。
