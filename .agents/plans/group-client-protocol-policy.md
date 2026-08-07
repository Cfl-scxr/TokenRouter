# 分组客户端协议准入重构

## 目标

- 将管理端“接入格式”明确为“上游平台”，并用 `allowed_client_protocols` 独立表达客户端文本协议准入。
- 按平台能力展示协议，新建时使用平台默认值；所有协议均可关闭并允许空集合。
- 保留 `allow_messages_dispatch` 兼容别名，并补齐 Gemini 分组的 OpenAI Responses 转换。

## 公共契约

协议枚举固定为：

- `anthropic_messages`
- `openai_responses`
- `openai_chat_completions`
- `gemini_generate_content`

`allowed_client_protocols` 返回完整有效集合。创建缺省时使用平台默认协议，更新缺省时保持现值；显式列表拒绝未知、重复或不受平台支持的值，空集合对所有平台都合法。旧字段仅在新字段缺省时继续兼容 OpenAI 分组，冲突时以新字段为准。

| 上游平台 | 支持协议 | 新建默认 | 已有分组迁移值 |
| --- | --- | --- | --- |
| Anthropic | Messages、Responses、Chat | Messages | 三项全部启用 |
| OpenAI | Messages、Responses、Chat | Responses、Chat | Responses、Chat + 按旧开关决定 Messages |
| Gemini | 四项全部支持 | Gemini GenerateContent | 四项全部启用 |
| Antigravity | 四项全部支持 | Messages、Gemini GenerateContent | 四项全部启用 |
| Qoder | Messages、Responses、Chat | 空集合 | 三项全部启用 |
| Grok | Messages、Responses、Chat | Responses、Chat | 三项全部启用 |

## 实施

1. 新增迁移 `235_add_group_allowed_client_protocols.sql`，回填已有分组并保留旧列；贯穿领域对象、仓储、管理/公开 DTO、分组复制和认证缓存。发布时通过认证快照版本升级淘汰旧缓存，不支持混合版本运行。
2. 在认证、复合 Key 选组和分组校验后统一执行协议门禁；按 Anthropic、OpenAI、Google 协议返回本地策略 `403`。
3. 为 Gemini Responses 新增正式转发分支，复用现有转换器和 Gemini 上游执行逻辑，覆盖非流、SSE、工具、reasoning、usage、错误和 failover 边界。
4. 管理端展示受支持协议并允许逐项关闭；用户侧客户端指引根据协议集合生成，空集合显示不可用状态。
5. 同步网关策略、HTTP API、能力矩阵和六个平台文档。

## 验证

- 覆盖平台矩阵、全平台空集合、输入校验、迁移回填、仓储/缓存/兼容字段往返和复合 Key。
- 覆盖各协议及别名的允许/拒绝路径，确认拒绝发生在正文读取和调度之前。
- 覆盖 Gemini Responses 非流、SSE、工具、reasoning、usage、错误映射和流式 failover 边界。
- 覆盖管理端协议控件和用户侧客户端标签/空状态。

## 边界

- Live、Responses WebSocket、Embedding、图片和视频继续使用现有独立能力规则。
- `claude_code_only` 与账号 endpoint capability 可以继续收窄协议集合。
- 直接在当前 `main` 实施，不创建分支。
