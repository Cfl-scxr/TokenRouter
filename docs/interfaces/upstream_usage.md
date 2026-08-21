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

缺少对象时按 `enabled=true`、`adapter=sub2api` 处理，根地址复用账号现有 API Base URL。只有显式 `enabled=false` 才关闭查询。`adapter` 只能是已注册的 `sub2api` 或 `new_api`；`base_url` 只能覆盖查询根地址，不能携带用户信息、查询串或片段。后端继续使用现有 HTTPS、allowlist、私网地址和 URL 格式校验。

API Key 永远从账号 `credentials` 读取。它不能写进 `extra`、接口响应、审计请求体、浏览器缓存或日志；用户也不能配置任意路径、方法、Header 模板或脚本。

## 适配器

### Sub2API

严格请求 `GET /v1/usage`，携带 `Authorization: Bearer <api_key>`。`quota_limited` 归一化为 Key 总限额和 `rate_limits`；`unrestricted` 归一化为钱包余额或订阅。订阅的日、周、月使用量、限额、重置时间、套餐和到期时间会进入 `subscription.limits`。上游 `remaining=-1` 只在适配器内部识别为 `unlimited=true`，不会传给前端。钱包出现负余额时保留真实负数；周期限额的使用量和限额必须是有限非负数，且响应内的剩余值一致。

### New API

严格请求 `/v1/dashboard/billing/subscription` 和 `/v1/dashboard/billing/usage`。订阅响应的 `hard_limit_usd` 是总额度，使用响应的 `total_usage` 按 `100` 除为美元；`access_until` 转为 UTC 到期时间。`/api/status` 只用于尽力识别 `USD`、`CNY` 或 `TOKENS` 单位，失败或格式未知不影响主查询。New API 不提供站点钱包余额，结果显示为 API Key 配额/订阅。

适配器拒绝 HTTP 非成功、认证失败、限流、超时、重定向、超大响应体、缺字段或不一致数值。选择的适配器失败时不会自动回退到另一个协议，也不会修改账号配置。

## 管理员接口

- `POST /api/v1/admin/accounts/:id/upstream-usage/query`
- `POST /api/v1/admin/accounts/upstream-usage/query/batch`，请求体为 `{ "account_ids": [1, 2] }`，最多 100 个正整数 ID。

成功结果在顶层包含 `account_id`、`adapter`、`provider`、UTC `observed_at`、`mode`、`unit`、`balance`、`limits`、`subscription` 和 `expires_at`；`mode` 为 `balance`、`quota`、`limits` 或 `subscription`。金额/Token 使用 `balance`，周期数据使用 `limits`，订阅数据使用 `subscription`。批量响应将成功结果和每个账号的结构化错误分开，单个账号失败不取消其它账号。

每次操作使用约 10 秒总超时、512 KiB 响应体上限、禁止重定向，并复用账号代理、TLS 指纹、Header Override 和 `HTTPUpstream`。查询前后重新读取账号；凭据、代理、Base URL、TLS 连接设置或规范化配置改变时返回 `UPSTREAM_USAGE_IDENTITY_CHANGED`。同一账号和配置指纹使用 singleflight，等待方可以独立取消。

## 前端生命周期

列表加载、滚动进入视口和自动刷新不会请求上游。管理员只能通过行内刷新按钮或批量操作触发查询；成功结果按管理员身份、账号 ID、`updated_at`、代理/Base URL 和规范化配置写入 `sessionStorage` 五分钟，失败结果不缓存。强制刷新绕过缓存；账号保存、凭据/代理/Base URL/配置变化立即失效。该缓存只保存归一化结果，不保存任何凭据。

旧的 `upstream_billing_probe` 是已移除的自动倍率探测能力，本功能不恢复它，也不写入旧快照或调度状态。
