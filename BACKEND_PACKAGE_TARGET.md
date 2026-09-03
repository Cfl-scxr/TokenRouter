# TokenRouter 后端目标包清单（草案）

> 状态：目标方案，当前尚未实施。
>
> 用途：作为后端逐步拆包时的参考清单。本文描述目标边界，不代表当前代码已经具有这些包结构。

## 使用约定

- 本文中的每一个叶子路径代表一个 Go package；`identity/`、`gateway/` 等父目录只是命名空间分组。
- 目标仍是一个模块化单体，所有包最终编译到同一个服务进程，不拆成微服务。
- `ent/*` 生成包、已经独立的 `internal/pkg/*` 工具/协议包和测试工具不作为本次重构的主要对象。
- 包数量不是验收指标。若两个包长期产生双向依赖，应合并；若一个包同时拥有不同生命周期或不同业务不变量，应继续拆分。
- 不按文件、结构体或构造函数逐个建包；包边界按业务所有权、状态生命周期和修改原因确定。

## 目标包清单

### 身份与访问

| 包 | 责任 |
| --- | --- |
| `internal/identity/user` | `User` 核心实体、启用/停用、资料和安全版本。 |
| `internal/identity/auth` | 密码登录、注册、OAuth、Passkey、TOTP 等认证流程编排。 |
| `internal/identity/authidentity` | 外部身份绑定、解绑、身份接纳和 provider subject 唯一性。 |
| `internal/identity/team` | 团队、成员、邀请、角色和所有权转移。 |
| `internal/identity/apikey` | API Key 生命周期、凭据状态、复合 Key 和认证快照。 |
| `internal/identity/apikey/modelmapping` | API Key 模型重定向、通配符匹配和模型别名的纯逻辑。 |
| `internal/identity/authsession` | JWT、Refresh Token、登录会话撤销、会话绑定和 step-up 状态。 |

### 目录、账号与路由

| 包 | 责任 |
| --- | --- |
| `internal/catalog/group` | 分组状态、客户端协议开关、能力和分组级策略。 |
| `internal/catalog/channel` | 渠道、渠道与分组关系、渠道模型映射。 |
| `internal/catalog/model` | 模型目录、模型市场、可请求模型和公开模型列表。 |
| `internal/catalog/pricing` | 渠道/分组价格配置结构、价格卡和配置校验。 |
| `internal/accounts/account` | 上游账号实体、账号管理和账号与分组关系。 |
| `internal/accounts/credentials` | 凭据归一化、凭据保存、刷新协调和账号类型判断。 |
| `internal/accounts/health` | 账号测试、额度/能力探测、临时不可调度和恢复。 |
| `internal/routing/model` | 客户端模型、Key 映射、渠道映射和上游模型的解析链。 |
| `internal/routing/policy` | 协议准入、模型/能力限制、隐私和 fallback 策略。 |
| `internal/routing/scheduler` | 基础/高级账号选择、评分、调度快照和候选过滤。 |
| `internal/routing/concurrency` | 用户/账号并发槽、限流、等待队列和取消释放。 |

### 网关与上游

| 包 | 责任 |
| --- | --- |
| `internal/gateway/core` | 协议无关的请求生命周期、attempt 编排和服务入口。 |
| `internal/gateway/failover` | 可切换错误分类、重试状态、退避和账号排除。 |
| `internal/gateway/stream` | SSE/流式响应泵、首个真实输出边界和流结束处理。 |
| `internal/gateway/session` | 网关粘性会话、Digest、WebSocket 会话和会话隔离。 |
| `internal/upstream/openai` | OpenAI、Codex、Responses、Chat、Images 和 Realtime 适配。 |
| `internal/upstream/anthropic` | Anthropic Messages、Bedrock 和 Vertex Anthropic 适配。 |
| `internal/upstream/gemini` | Gemini 原生/兼容协议、会话和签名适配。 |
| `internal/upstream/antigravity` | Antigravity 专用端点、Claude/Gemini 转换和混合调度适配。 |
| `internal/upstream/grok` | Grok 文本、图片、视频、搜索、语音和 Realtime 适配。 |
| `internal/upstream/qoder` | Qoder 原生协议、站点、思考和上下文能力适配。 |
| `internal/upstream/compat` | Kimi、Zhipu、DeepSeek、Ollama 等兼容协议的共同适配层。 |

### 用量、计费与商业

| 包 | 责任 |
| --- | --- |
| `internal/billing/usage` | token/媒体用量归一化、Usage Log 输入和用量事实。 |
| `internal/billing/pricing` | 请求成本、账号成本、倍率、长上下文和媒体价格计算。 |
| `internal/billing/settlement` | 幂等结算、余额扣减、Key/团队累计和对账状态。 |
| `internal/billing/entitlement` | 用户余额、平台额度和请求消费资格。 |
| `internal/billing/subscription` | 订阅计划、周期额度、订阅分配和订阅生命周期。 |
| `internal/commerce/payment` | 支付订单、支付 provider、回调、履约和退款。 |
| `internal/commerce/promotion` | Promo Code、兑换码、邀请码、联盟和返利。 |

### 异步任务、审核与运维

| 包 | 责任 |
| --- | --- |
| `internal/jobs/batchimage` | 批量图片任务、队列、对象存储、预占和结算。 |
| `internal/jobs/creative` | 创作台任务、Redis 临时数据、outbox 和 worker。 |
| `internal/jobs/scheduled` | 计划任务、定时测试和其它可调度后台作业。 |
| `internal/moderation/content` | 内容审核、关键词/Hash、审核 API 和同步阻断。 |
| `internal/moderation/policy` | 风险处置、错误透传、封禁和本地策略拒绝。 |
| `internal/operations/observability` | 指标、日志、告警、运行时快照和 Ops 信号。 |
| `internal/operations/analytics` | Dashboard、用量聚合、趋势和统计查询。 |
| `internal/operations/data` | 数据共享、会话导出、脱敏和隐私生命周期。 |
| `internal/operations/maintenance` | 备份、清理、迁移、过期处理和恢复任务。 |

### HTTP 接口层

HTTP 包按路由族组织，不要求和业务包一一对应；它们只负责输入输出、协议错误和调用应用服务。

| 包 | 责任 |
| --- | --- |
| `internal/interfaces/http/middleware` | JWT/API Key、请求体、限流、审计和安全中间件。 |
| `internal/interfaces/http/routes` | Gin engine 的路由注册和中间件顺序。 |
| `internal/interfaces/http/auth` | 登录、注册、OAuth、Passkey 和会话接口。 |
| `internal/interfaces/http/user` | 用户、身份和个人设置接口。 |
| `internal/interfaces/http/team` | 团队、成员和邀请接口。 |
| `internal/interfaces/http/apikey` | API Key 管理、模型映射和 Key 用量接口。 |
| `internal/interfaces/http/gateway` | Messages、Responses、Chat、Gemini、媒体和实时接口。 |
| `internal/interfaces/http/admin` | 管理端账号、分组、渠道、计费、运维和审核接口。 |

### 存储、传输与组合

| 包 | 责任 |
| --- | --- |
| `internal/repository/postgres` | PostgreSQL/Ent 查询、事务、聚合和迁移运行时实现。 |
| `internal/repository/redis` | Redis 缓存、限流、并发、锁、队列和失效广播实现。 |
| `internal/repository/objectstore` | 本地、S3 兼容和 GCS 对象存储实现。 |
| `internal/transport/httpclient` | 上游 HTTP 客户端、响应限制、连接池和超时。 |
| `internal/transport/proxy` | 代理解析、代理生命周期和出站代理选择。 |
| `internal/transport/tls` | TLS 指纹 Profile/Router 和连接级安全配置。 |
| `internal/transport/websocket` | WebSocket 建连、帧转发、关闭和连接池基础设施。 |
| `internal/config` | 启动配置、默认值、环境映射和校验。 |
| `internal/setup` | 首次设置、自动初始化和设置模式入口。 |
| `internal/server` | HTTP server 生命周期、静态资源交付和优雅关闭。 |
| `cmd/server` | 唯一的组合根：Wire provider、具体实现绑定和启动流程。 |

## 依赖规则

目标依赖方向如下。箭头表示“可以依赖/导入”。

```text
interfaces/http/*  ->  identity/catalog/routing/gateway/billing/operations

identity/*         ->  稳定领域类型、标准库和本包定义的窄接口
gateway/*          ->  identity/apikey、routing、accounts、upstream、billing/usage
upstream/*         ->  accounts、transport 和协议公共类型
billing/*          ->  identity、catalog、accounts 的快照或窄接口
repository/*       ->  各业务包定义的 repository 接口和持久化类型

cmd/server         ->  所有具体实现，只负责组装，不写业务规则
```

以下依赖禁止出现：

- 领域/策略包直接导入 Gin、Ent、Redis 客户端或具体上游 SDK。
- HTTP handler 直接访问 repository、数据库或 Redis。
- `auth`、`apikey`、`team` 互相直接调用具体 service 形成环；跨边界使用 ID、快照、窄接口或事件。
- `gateway/session` 与 `identity/authsession` 共享一个泛化的 `Session` 大包。
- 把所有跨包类型塞进一个 `common` 包，或让通用工具反向依赖业务包。

接口优先放在使用接口的一方。只有当多个包确实共享稳定的跨域数据契约、且无法通过调用方接口避免循环时，才增加小型 `contract` 子包；不预先建立一个包含所有接口的全局 `ports` 包。

## 关键边界说明

### 身份边界

- `auth` 负责“如何登录”，`user` 负责“用户是什么状态”，`authidentity` 负责“外部主体属于谁”。
- `team` 独立拥有成员角色、邀请和所有权规则；登录流程只能通过窄接口读取团队状态。
- `apikey` 拥有 Key 生命周期和认证快照；模型映射可以保持为其下的纯子包。
- 浏览器认证 Session 与网关粘性 Session 是两种不同领域，不能合并。

### 目录、路由与上游边界

- `catalog/pricing` 只管理价格配置；`billing/pricing` 负责运行时计算。
- `accounts` 管理账号和健康状态；`routing/scheduler` 只做本次请求的候选选择。
- `gateway/core` 负责请求编排；各 `upstream/*` 负责平台协议和供应商差异。
- Gemini 与 Antigravity、Qoder 与其它兼容协议不能因为 wire format 相似而强行合并。

### 计费与任务边界

- `billing/usage`、`billing/pricing`、`billing/settlement` 分别负责事实、计算和资金事务。
- `billing/subscription` 与 `commerce/payment` 分开；订阅是权益状态，支付是外部订单状态。
- `batchimage` 和 `creative` 都是异步任务，但队列、临时数据和生命周期不同，应分别拥有状态机。
- `operations/data` 的数据共享生命周期不能混入普通 Ops 查询或网关业务。

### 存储与接口边界

- repository 不按每张表创建一个包；按 PostgreSQL、Redis、对象存储和事务边界组织。
- HTTP handler 不必和业务包一一对应；管理端可以在一个接口包中协调多个业务能力。
- `ent/*` 继续作为生成代码使用，不把生成包重新包装成大量业务包。

## 建议的迁移顺序

1. 先拆 `identity/apikey/modelmapping` 这类无外部副作用的纯逻辑。
2. 再拆 `identity/authsession`、`identity/apikey`、`identity/team` 和 `identity/authidentity`。
3. 拆 `catalog`、`accounts`、`routing`，建立网关使用的快照和窄接口。
4. 拆 `gateway` 和 `upstream`，保持每个平台现有协议与流式行为不变。
5. 最后拆 `billing`、`commerce`、`jobs`、`moderation`、`operations` 以及 HTTP handler。

每次只迁移一个包或一个垂直能力，并保留旧包中的薄适配层，直到所有调用方和测试完成迁移。目标是逐步减少未经约束的依赖边，而不是一次性移动所有文件。
