# API Key 上游用量查询

本文定义管理员对 `type=apikey` 账号执行的手动上游用量查询。它是展示型能力，不参与 TokenRouter 调度、自动暂停、账号倍率、本地配额或结算；`type=bedrock` 不在范围内。OAuth/Setup Token 的官方用量窗口仍由 `AccountUsageService` 独立维护。

## 配置

账号 `extra.upstream_usage_query` 只保存非敏感配置：

```json
{
  "enabled": true,
  "adapter": "sub2api",
  "base_url": "https://gateway.example.com"
}
```

缺少对象时按 `enabled=true`、`adapter=sub2api` 处理，根地址复用账号现有 API Base URL。只有显式 `enabled=false` 才关闭查询。`adapter` 只能是已注册的 `sub2api`、`new_api` 或 `zivv`；`base_url` 只能覆盖查询根地址，不能携带用户信息、查询串或片段。后端继续使用现有 HTTPS、allowlist、私网地址和 URL 格式校验。

API Key 永远从账号 `credentials` 读取。它不能写进 `extra`、接口响应、审计请求体、浏览器缓存或日志；用户也不能配置任意路径、方法、Header 模板或脚本。

New API 钱包若需要用户级认证，可在 `credentials` 中保存
`new_api_user_access_token`（敏感字段，只写入不回显）和可选的
`new_api_user_id`。前者只用于钱包查询，不能参与账号转发；后端响应只返回
`credentials_status.has_new_api_user_access_token`。

## 适配器

### Sub2API

严格请求 `GET /v1/usage`，携带 `Authorization: Bearer <api_key>`。`quota_limited` 归一化为 Key 总限额和 `rate_limits`；`unrestricted` 归一化为钱包余额或订阅。订阅的日、周、月使用量、限额、重置时间、套餐和到期时间会进入 `subscription.limits`。上游 `remaining=-1` 只在适配器内部识别为 `unlimited=true`，不会传给前端。钱包出现负余额时保留真实负数；周期限额的使用量和限额必须是有限非负数，且响应内的剩余值一致。

### New API

严格请求 API Key 专用的 `/api/usage/token/`，把 `total_granted`、`total_used` 和 `total_available` 归一化为当前 Key 的 `limits` 与 `subscription`，不写入结果的 `balance`。再读取 `/api/status` 的 `quota_display_type`、`quota_per_unit` 和可选 `usd_exchange_rate` 换算为 `USD`、`CNY` 或 `TOKENS`。`expires_at` 转为 UTC 到期时间，`unlimited_quota=true` 归一化为 `subscription.unlimited=true`，即使上游同时返回整数溢出的负额度也忽略这些字段，不向前端传递哨兵值。

钱包余额按以下固定顺序查询：若 token 响应包含 fork 扩展的 `user_balance_display`/`user_balance`，直接使用；否则优先使用配置的用户访问令牌请求 `/api/user/self`（可带固定的 `New-Api-User` 用户 ID），只把当前 `quota` 归一化为钱包 `remaining`，不把生命周期 `used_quota` 拼成虚构的钱包总额；最后尝试允许 API Key 访问的 `/user/balance`，解析 `balance_infos[].total_balance`。钱包余额才进入结果的 `balance`，因此即使 Key 是无限量，也不会把 `100000000` 或整数溢出值显示成余额。官方 New API 未开放 API Key 钱包端点且未配置用户访问令牌时，返回 `UPSTREAM_USAGE_WALLET_UNAVAILABLE`，不降级为 token quota。

不再请求用户级 `/v1/dashboard/billing/subscription` 或 `/v1/dashboard/billing/usage`，避免把全局额度或无限量哨兵误当作钱包；`/api/status` 失败时使用 New API 默认的 `500000` quota/单位，仅影响内部 quota 换算，不阻断钱包/Token 查询。

### Zivv

严格请求 `GET /v1/user/balance`，携带 `Authorization: Bearer <api_key>`。响应中的 `balance` 是钱包剩余余额，`total_used` 是累计已用金额；`key_limit`/`key_used` 归一化为 Key 限额，`key_limit=0` 表示不限量，`plan_name` 进入订阅计划展示。`currency` 目前支持 `USD`、`CNY` 和 `TOKENS`。Zivv 的公开开发者文档说明余额位于控制台钱包页面，适配器使用其前端生成的固定余额接口，不请求任意路径或脚本。参见 [Zivv 计费说明](https://docs.zivv.pro/billing/overview) 与 [API 端点](https://docs.zivv.pro/reference/endpoints)。

适配器拒绝 HTTP 非成功、认证失败、限流、超时、重定向、超大响应体、缺字段或不一致数值。选择的适配器失败时不会自动回退到另一个协议，也不会修改账号配置。

## 管理员接口

- `POST /api/v1/admin/accounts/:id/upstream-usage/query`
- `POST /api/v1/admin/accounts/upstream-usage/query/batch`，请求体为 `{ "account_ids": [1, 2] }`，最多 100 个正整数 ID。

成功结果在顶层包含 `account_id`、`adapter`、`provider`、UTC `observed_at`、`mode`、`unit`、`balance`、`limits`、`subscription` 和 `expires_at`；New API 的 `balance` 是钱包余额，`limits`/`subscription` 是当前 Key 的配额信息。`mode` 为 `balance`、`quota`、`limits` 或 `subscription`。批量响应将成功结果和每个账号的结构化错误分开，单个账号失败不取消其它账号。

每次操作使用约 60 秒总超时、512 KiB 响应体上限、禁止重定向，并复用账号代理、TLS 指纹、Header Override 和 `HTTPUpstream`。查询前后重新读取账号；凭据、代理、Base URL、TLS 连接设置或规范化配置改变时返回 `UPSTREAM_USAGE_IDENTITY_CHANGED`。同一账号和配置指纹使用 singleflight，等待方可以独立取消。

## 前端生命周期

列表加载、滚动进入视口和自动刷新不会请求上游。管理员只能通过行内刷新按钮或批量操作触发查询；成功结果按管理员身份、账号 ID、`updated_at`、代理/Base URL 和规范化配置写入 `sessionStorage` 五分钟，失败结果不缓存。强制刷新绕过缓存；账号保存、凭据/代理/Base URL/配置变化立即失效。该缓存只保存归一化结果，不保存任何凭据。

旧的 `upstream_billing_probe` 是已移除的自动倍率探测能力，本功能不恢复它，也不写入旧快照或调度状态。
