# TLS 指纹收集器实施计划

## Summary

- 在 TLS 指纹模板管理弹窗内新增收集器，支持管理员从页面启动/停止。
- 后端配置只提供默认监听参数；正常采集流程不需要改配置或重启网关。
- 默认不需要手动配置证书：收集器启动时生成临时自签 CA + server cert，页面提供 Claude Code / Codex CLI 对应的 CA 环境变量。
- 采集结果短期保存在内存，可一键填入现有 TLS 模板表单。

## Key Changes

- 后端新增运行时收集器服务：
  - 默认状态为停止。
  - 管理员调用启动接口后，服务用配置中的 `host`、`port`、`public_base_url`、`cert_file`、`key_file` 启动独立 HTTPS listener。
  - 未配置证书时自动生成内存自签 CA 与服务端证书，并通过管理员 API 返回 CA PEM。
  - 停止接口优雅关闭 listener，并清空采集会话，避免误用旧 token。
  - 网关重启后收集器回到停止状态，避免意外长期暴露采集端口。
- 后端新增采集解析能力：
  - 在 TLS 握手前读取原始 ClientHello，解析 cipher suites、extensions 顺序、supported groups、point formats、signature algorithms、ALPN、supported versions、key share groups、PSK modes。
  - GREASE 值统一归一为 `0x0a0a`，并设置 `enable_grease=true`。
  - 计算 JA3 raw/hash；记录 negotiated ALPN 与 HTTP 协议。HTTP/2 SETTINGS 仅作为诊断信息，不写入模板字段。
  - 完成 TLS 握手后返回协议兼容的最小假响应，保证 Claude Code / Codex 发一次请求即可完成采集。
- 管理 API 扩展到现有 TLS 模板 handler：
  - `GET /api/v1/admin/tls-fingerprint-profiles/collector/status`
  - `POST /api/v1/admin/tls-fingerprint-profiles/collector/start`
  - `POST /api/v1/admin/tls-fingerprint-profiles/collector/stop`
  - `POST /api/v1/admin/tls-fingerprint-profiles/collector/sessions`
  - `GET /api/v1/admin/tls-fingerprint-profiles/collector/sessions/:token/captures`
  - `DELETE /api/v1/admin/tls-fingerprint-profiles/collector/sessions/:token`
  - 路由注册必须放在 `/:id` 前，避免被模板详情路由吞掉。
- 前端 `TLSFingerprintProfilesModal.vue`：
  - 在粘贴 YAML 区域旁新增“收集器”入口。
  - 显示收集器状态、监听地址、启动/停止按钮。
  - 启动后可创建 Claude Code / Codex 采集会话。
  - 展示 Token、过期时间、采集地址、Claude Code 命令、Codex CLI 配置片段、CA PEM 下载/复制。
  - 自动轮询采集结果；最新结果支持“填入表单”“复制 YAML”。
  - 收集器未启动或启动失败时显示明确提示，不影响原有手动创建/编辑/删除模板流程。

## Public Interfaces

- 新配置默认值：
  - `server.tls_fingerprint_collector.host="0.0.0.0"`
  - `server.tls_fingerprint_collector.port=8443`
  - `server.tls_fingerprint_collector.public_base_url=""`
  - `server.tls_fingerprint_collector.cert_file=""`
  - `server.tls_fingerprint_collector.key_file=""`
  - `server.tls_fingerprint_collector.session_ttl_seconds=1800`
  - `server.tls_fingerprint_collector.max_records_per_session=20`
- 采集结果 DTO 包含：
  - `id`、`captured_at`、`client_kind`、`request_path`、`user_agent`、`ja3_raw`、`ja3_hash`、`negotiated_alpn`、`http_proto`
  - `profile`：可直接提交给现有模板创建接口的字段集合
  - `yaml`：兼容现有粘贴解析器的 YAML 文本
  - `headers_summary`：脱敏后的关键头摘要

## Deployment Notes

- 默认推荐独立采集端口直连，例如 `https://example.com:8443`，这样 TLS 握手直接到 TokenRouter 收集器，可以采到真实 ClientHello。
- 如果只在内网采集，可以直接开放内网地址和端口。
- 如果要共用 443 或走 Caddy 域名分流，不能用普通 `reverse_proxy`，因为它会终止 TLS，导致 TokenRouter 采不到真实 ClientHello；必须使用 TCP/TLS passthrough。
- Caddy 自动证书不作为默认路径。默认自签 CA 更稳定；页面提供 CA PEM 与客户端环境变量。

## Test Plan

- 后端：
  - ClientHello parser 覆盖 ALPN、extensions 顺序、key share、PSK、GREASE 归一化、JA3 计算。
  - session store 覆盖创建、TTL 过期、每 session 最大记录数、并发写入。
  - collector integration 用 uTLS/crypto-tls 客户端连本地采集端，验证 Claude Code/Codex 路径都能生成 profile。
  - admin handler 覆盖 status、start、stop、create session、list captures、delete session、disabled/未启动状态。
  - 回归现有 TLS profile CRUD 测试。
- 前端：
  - `TLSFingerprintProfilesModal.spec.ts` 覆盖收集器状态展示、启动/停止、启动失败提示、创建 session、轮询结果、填入表单、复制 YAML。
  - 跑 `pnpm --dir frontend typecheck`。
- 验证命令：
  - `go test ./internal/pkg/tlsfingerprint ./internal/service ./internal/handler/admin`
  - `pnpm --dir frontend test:run src/components/admin/__tests__/TLSFingerprintProfilesModal.spec.ts`
  - `pnpm --dir frontend typecheck`

## Assumptions

- 页面只控制启停，不在页面修改监听端口和证书路径。
- 收集器默认不会随网关启动自动开启。
- 内存短期存储足够；重启后采集会话和结果丢失是预期行为。
- Codex 指令基于官方配置键 `openai_base_url` / `chatgpt_base_url`。
- v1 只把现有模板能表达的 TLS 字段写入模板；HTTP/2 SETTINGS 指纹只展示诊断信息，不参与模板保存。
