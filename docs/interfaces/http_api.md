# HTTP 接口边界

本文描述 TokenRouter 的稳定路由族、认证方式、共同中间件、响应形状和路由所有权。它用于新增或移动接口时选择正确边界，不逐项复制全部 endpoint，也不替代各上游协议规范或领域状态机。

## 章节导航

- [全局入口](#全局入口)：理解所有请求共享的处理顺序。
- [路由族](#路由族)：选择 URL、认证和拥有者。
- [认证方式](#认证方式)：区分 JWT、管理密钥、API Key 和签名票据。
- [外部支付管理集成](#外部支付管理集成)：服务间充值和嵌入页对接边界。
- [响应与错误](#响应与错误)：保持面板与协议兼容形状。
- [请求关联](#请求关联)：正确透传 request ID。
- [变更规则](#变更规则)：新增接口时检查。

## 全局入口

`SetupRouter` 在一个 Gin engine 上安装共同中间件并依次注册 common、auth、user、admin、gateway、payment 和 page routes。主要全局顺序为：

```text
RequestLogger
  -> security client IP / session binding context
  -> access logger
  -> CORS
  -> security headers / CSP
  -> optional Server-Timing
  -> embedded frontend and API routes
```

`X-Request-ID` 是服务端请求关联 ID：长度和字符合法时沿用客户端值，否则生成 UUID，并写回响应和 request context。网关路由另外安装 `ClientRequestID`，以 `X-Client-Request-ID` 关联一次客户端业务请求、Ops 记录和后台结算；两者用途不同，不能互相覆盖。

入口体积限制和错误采集按路由族叠加。网关在读取 JSON/multipart 之前应用通用或文本 body limit、client request ID、Ops error logger、endpoint 归一化和 API Key auth。面板接口使用全局/重查询限流和审计；高风险公开认证接口使用独立 Redis 限流并在依赖故障时 fail-close。

## 路由族

| 路由族 | 认证 | 主要所有者与用途 |
| --- | --- | --- |
| `/health`、`/setup/status` | 无 | `routes/common.go`；进程健康与正常模式 setup 状态 |
| `/api/event_logging/batch` | 无 | Claude Code 遥测兼容空接收，固定返回成功 |
| `/api/v1/auth/*` | 大多公开，账户管理子流程按路由加 JWT/短期状态 | `routes/auth.go`；注册、登录、刷新、密码恢复、OAuth、Passkey 登录和身份完成 |
| `/api/v1/user/*`、`/keys`、`/team`、`/groups`、`/subscriptions`、`/redeem` 等 | 用户 JWT | `routes/user.go`；用户面板资源、团队、Key、用量、数据共享和权益自省 |
| `/api/v1/admin/*` | 管理员 JWT 或受限管理密钥；部分操作另需 step-up | `routes/admin.go`；用户、分组、账号、渠道、设置、运维、备份、支付和安全管理 |
| `/api/v1/payment/*` | 用户 JWT | `routes/payment.go`；配置/套餐读取、下单、查单、取消、invoice 和退款申请 |
| `/api/v1/payment/public/*` | 签名 resume token 或遗留订单验证约束 | 支付结果恢复；不得扩展为匿名订单枚举接口 |
| `/api/v1/payment/webhook/*` | 提供商验签 | EasyPay、Alipay、WeChat Pay、Stripe、Airwallex 通知 |
| `/v1/*` 和兼容裸别名 | TokenRouter API Key | Anthropic/OpenAI 兼容消息、Responses、Chat、图片、视频、模型、用量与异步/批任务 |
| `/v1beta/*` | TokenRouter API Key | Gemini 原生模型 URL、生成、流式生成和 token 统计 |
| `/antigravity/*` | TokenRouter API Key + 强制平台 | Antigravity 专用 Claude/Gemini 入口与管理型自省 |
| `/backend-api/codex/*` | TokenRouter API Key | Codex Responses、Realtime 与 sideband 兼容入口 |
| `/api/v1/pages/*` 等 page routes | 按页面类型为用户或管理员 JWT | 服务端生成/读取的 pricing、账单或管理页面数据 |

部分下载路由使用短期签名票据，以支持浏览器原生下载大文件；票据只授权一个预生成资源，不能等价为用户 JWT。模型列表、用量、账单自省和既有异步任务读取虽然可能跳过消费余额检查，仍要执行 Key 身份和资源归属验证。

路由前缀不独自决定协议处理器。例如 `/v1/messages` 会根据分组平台分派到 Anthropic、OpenAI/Grok 或 Qoder handler；路由层拥有分派，handler/service 不能通过字符串猜测调用方已经具备某个平台能力。

## 认证方式

| 凭据 | 接收位置 | 权限边界 |
| --- | --- | --- |
| JWT access token | 面板 `Authorization: Bearer`，少量 WebSocket 子协议 | 当前用户状态、token version、可选 session binding；管理员还校验当前 role |
| Refresh token | `/api/v1/auth/refresh` 与 logout payload/cookie 约定 | 只用于轮换/撤销，不可直接访问业务资源 |
| 管理 API Key | 管理接口 `x-api-key` | 绑定首个真实管理员；启用敏感 step-up 后不能执行需近期 TOTP 的操作 |
| TokenRouter API Key | 网关 `Authorization: Bearer`、`x-api-key`，Gemini 兼容 `x-goog-api-key` | Key、用户/团队、分组、IP、额度/订阅和请求资源归属；通用网关不接受 query Key |
| OAuth/pending completion 状态 | auth callback 和完成接口 | provider state、浏览器会话、一次性完成码与过期时间共同约束 |
| 支付 webhook 签名 | 原始 body/query + provider headers | 只授权解释一条已绑定本地订单的通知，仍需校验金额和 metadata |
| 下载/resume ticket | 指定公共恢复或下载路由 | 有时限、限定资源和操作，不能升级为一般会话 |

认证成功只建立主体。资源 owner、团队成员、管理员 step-up、支付订单 user ID、异步任务 owner 和分组能力仍由相应 handler/service 检查。不得因为路由已挂认证中间件就省略对象级授权。

## 外部支付管理集成

外部支付服务应使用管理 API Key 通过 `x-api-key` 调用管理路由，管理员 JWT 只适合交互式管理端。支付成功后的余额发放优先使用 `POST /api/v1/admin/redeem-codes/create-and-redeem`，由服务端原子创建并兑换余额兑换码；调用方必须提供稳定的业务 `code` 和 `Idempotency-Key`，同一操作重试时复用二者，并按 200、409 和业务错误区分重放、冲突与失败。`GET /api/v1/admin/users/:id` 可用于前置查询，`POST /api/v1/admin/users/:id/balance` 只用于明确的人工增减或补偿，同样需要幂等键。

购买页和用户自定义页面由前端追加 `user_id`、`token`、`theme`、`lang`、`ui_mode`、`src_host` 和 `src_url`。其中 `token` 是用户 Bearer 凭据，只能发送到部署者信任且使用 HTTPS 的页面来源；接收方不得写入访问日志、分析参数或转发给第三方。完整请求示例和重试约定见 [外部支付管理 API 指南](../guides/payments/admin_integration_api.md)。

## 响应与错误

面板和内部 REST 接口通常使用统一 envelope：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

业务错误由 `ApplicationError` 映射为 HTTP status，并可返回 `reason` 和字符串 `metadata`。未知错误按 500 处理并只在服务端记录脱敏详情。分页数据使用 `items`、`total`、`page`、`page_size` 和 `pages`；创建与异步接受分别可以返回 201/202。

网关错误必须保持调用协议形状：OpenAI 入口使用 `error` 对象，Anthropic 使用 `type: error` 与嵌套错误，Google 使用 HTTP code/message/status。认证、未分组、复合 Key 和本地能力拒绝都选择当前协议 writer；不能为了复用面板 helper 把一个 Google/Anthropic 客户端错误改成面板 envelope。

错误响应不得包含上游凭据、代理 URL、原始 service account、数据库错误或未经脱敏的请求正文。流式响应开始后不能再写普通 JSON 错误；只能按当前 SSE/流协议结束或发送允许的错误事件。

## 请求关联

- `X-Request-ID` 用于一次 HTTP 调用的日志和审计关联，最长持久化长度受限。
- `X-Client-Request-ID` 用于网关业务链路和结算幂等来源；缺失时由入口生成并写回。
- 上游 request ID 属于供应商观测字段，需单独保存，不能替换本地 ID。
- 后台 worker 从请求派生所需 metadata 后使用受超时约束的新 Context；不得继续持有已取消请求的 body 或 Gin context。

当客户端允许重试时，应保留同一业务 request ID，但服务端必须结合 API Key 和请求指纹识别冲突。只根据请求 ID 字符串判断“重复”会把不同用户或不同 payload 混在一起。

## 变更规则

新增或移动接口时至少核对：

- 由 common、auth、user、admin、payment、gateway 还是 page route 拥有，是否使用既有前缀和 handler 分派。
- 需要无认证、JWT、管理员、step-up、API Key、provider 签名还是短期票据；是否还需要对象级 owner 检查。
- body/header 限制、面板限流、审计、Ops 采集、request/client request ID 和 Server-Timing 是否适用。
- 返回面板 envelope 还是 OpenAI/Anthropic/Google 协议形状，流式开始后的错误路径是否有效。
- 后端 route contract tests、handler/service 测试和前端 API 模块是否同时更新。
- 是否无意新增冲突的动态路由；例如 wildcard/subpath 不得吞掉已明确移除或专用的固定 endpoint。

相关文档：[身份与租户](../domains/identity_and_tenancy.md)、[网关请求生命周期](../architecture/gateway_request_lifecycle.md)、[支付与权益](../domains/payments_and_entitlements.md)、[接口目录](index.md)。
