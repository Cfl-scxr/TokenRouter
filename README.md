<div align="center">
<h1>TokenRouter</h1>

[![Go](https://img.shields.io/badge/Go-1.25.7-00ADD8.svg)](https://golang.org/)
[![Vue](https://img.shields.io/badge/Vue-3.4+-4FC08D.svg)](https://vuejs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15+-336791.svg)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D.svg)](https://redis.io/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED.svg)](https://www.docker.com/)

**AI API 网关平台 - 订阅配额分发管理**

</div>

## 项目概述

TokenRouter 是一个 AI API 网关平台，用于分发和管理 AI 产品订阅的 API 配额。用户通过平台生成的 API Key 调用上游 AI 服务，平台负责鉴权、计费、负载均衡和请求转发。

TokenRouter 基于 [Sub2API](https://github.com/Wei-Shaw/sub2api) 开发，在此感谢上游项目的贡献。

## 部署方式

详细部署说明见 [DEPLOY_GUIDE.md](docs/DEPLOY_GUIDE.md)。Apple 芯片 Mac 也可按 [Apple container 部署指南](deploy/APPLE_CONTAINER.md) 运行本地三服务栈。

## 赞助商

<table>
<tr>
<td width="180"><a href="https://cctk.ai/register?aff=SUB2API"><img src="assets/partners/logos/cctk.jpg" alt="CCTK.AI" width="150"></a></td>
<td>感谢 CCTK.AI 赞助了本项目！<a href="https://cctk.ai/register?aff=SUB2API">CCTK.AI</a> 是一个专注于稳定与性价比的 AI API 网关平台，提供 Claude、OpenAI、Gemini 等主流模型的高速中转服务，无缝兼容 Claude Code、Codex 等主流编程工具，以远低于官方的成本获得同等的模型能力。点击<a href="https://cctk.ai/register?aff=SUB2API">此链接</a>注册，即刻体验更快、更稳、更省的 AI API 接入。</td>
</tr>
</table>

## Grok / xAI 支持

TokenRouter 支持 Grok OAuth 订阅账号和标准 xAI API-key 账号，并通过 OpenAI 兼容的
Responses、Chat Completions、Messages 和 WebSocket 入口转发请求。管理员可在控制台
选择 OAuth 或 API Key 创建账号；用户可在 API Key 页面通过“使用密钥”生成 Grok
Build CLI 或 OpenCode 配置。Grok 分组还支持图片生成/编辑、视频生成/编辑/扩展以及视频状态查询。

Grok Build CLI 的模型配置应指向 TokenRouter 对外地址（以 `/v1` 结尾），不能直接使用
`api.x.ai` 或内部 OAuth 代理地址。OAuth 流量默认转发到 Grok CLI 订阅代理；可通过
`XAI_GROK_CLI_VERSION` 覆盖客户端版本，但版本不得低于内置支持版本。

OAuth 账号可通过控制台创建或重新授权；API-key 账号默认使用
`https://api.x.ai/v1`。创建 Grok 分组并绑定账号后，用户即可生成分组 API Key；现有
`config.toml` 应先备份再合并新模型配置。

## OpenAI Responses WebSocket 首消息超时

`gateway.openai_ws.client_first_message_timeout_seconds` 限制 WebSocket 升级后完整读取并
解压首条客户端 `response.create` 消息的总时间，默认 30 秒。大上下文、图片较多或慢链路
场景可调高到 120-300 秒。该截止时间在 HTTP bridge 路由判断前生效，bridge 模式不会绕过它。

```yaml
gateway:
  openai_ws:
    client_first_message_timeout_seconds: 30
```

## 许可证

This project is licensed under the [GNU Lesser General Public License v3.0](LICENSE) (or later).

Copyright (c) 2026 Wesley Liddick & TokenFlux
