# 配置边界

本文描述进程配置、首次初始化、数据库运行时设置、领域配置和前端构建变量的来源及优先级。它用于新增配置项或修改管理设置时判断存储位置、校验时机和是否需要重启，不枚举 `Config` 中的每个字段或复制部署样例。

## 章节导航

- [配置分层](#配置分层)：先确定一个选项属于哪一层。
- [进程配置来源](#进程配置来源)：修改默认值、YAML 或环境变量时读取。
- [首次初始化](#首次初始化)：修改 setup 和安全密钥引导时读取。
- [数据库运行时设置](#数据库运行时设置)：修改管理员设置和热更新时读取。
- [领域配置](#领域配置)：避免把业务实体塞入通用键值设置。
- [前端变量](#前端变量)：区分构建/开发变量与后端运行时配置。
- [新增配置检查](#新增配置检查)：实现与测试清单。

## 配置分层

| 层 | 例子 | 权威来源 | 生效方式 |
| --- | --- | --- | --- |
| 进程基础设施配置 | server、database、Redis、日志、CORS、JWT、worker/queue、连接池和硬安全开关 | `config.Config`，由默认值/YAML/环境变量加载 | 通常启动时读取，修改后重启 |
| 引导与持久安全密钥 | 初始 DB/Redis、管理员创建、JWT secret、安装锁 | setup 流程、`config.yaml`、`security_secrets` | 仅首次安装或启动引导阶段 |
| 数据库运行时设置 | 注册、OAuth、SMTP、面板限流、step-up、部分调度/超时、展示和功能开关 | `settings` 表 + `SettingService` | getter/cached snapshot/on-update，按设置实现热生效 |
| 领域配置 | 用户、团队、API Key、分组、渠道、账号、套餐、支付实例 | 对应 Ent schema/service | CRUD 事务后失效领域缓存 |
| 前端构建/开发配置 | API base、WS base、dev proxy、dev port | Vite `VITE_*` | 构建或 dev server 启动时注入 |

同名概念可能跨层，但要有明确接管语义。例如进程 `security.trust_forwarded_ip_for_api_key_acl` 提供启动默认值，数据库设置可以在运行时覆盖安全客户端 IP 策略；读取方必须使用运行时 snapshot，而不是持续读取旧的 struct 字段。相反，数据库地址和 Redis 连接池不能通过管理后台热切换。

<a id="configuration_sources"></a>
## 进程配置来源

`config.load` 使用 Viper，最终优先级为：

```text
环境变量
  > 选中的 config.yaml
  > setDefaults 注册的代码默认值
```

配置文件选择规则为：

1. `CONFIG_FILE` 非空时只使用该显式文件路径。
2. 否则按顺序搜索 `DATA_DIR`（若设置）、`/app/data`、当前目录、`./config`、`/etc/sub2api` 中的 `config.yaml`。
3. 文件不存在允许继续使用默认值和环境变量；文件存在但 YAML 无法读取/解析则启动失败。

环境变量把点分键转成大写下划线，例如 `database.host` 对应 `DATABASE_HOST`，`gateway.max_body_size` 对应 `GATEWAY_MAX_BODY_SIZE`。`setDefaults` 还负责把所有 struct 键注册进 Viper，使纯环境变量部署能被 `Unmarshal` 看到；新增字段不能只加 `mapstructure` tag 而不注册默认/可达键。少量变量有显式绑定或专用解析：`ENABLE_SERVER_TIMING`，逗号分隔的 `SERVER_TRUSTED_PROXIES` 和 `SECURITY_FORWARDED_CLIENT_IP_HEADERS`，以及受兼容条件约束的旧 WeChat 变量。

加载完成后会做字符串规范化、枚举回退、派生默认、文件读取和完整 `Validate`。无效安全 header、URL、数值范围、模式组合或必要 secret 会让启动失败；不应等到某个请求首次使用时才发现。自动生成的 TOTP key 只适合开发，`EncryptionKeyConfigured=false` 会阻止后台把 TOTP 当成生产可用配置。

环境变量优先于 YAML，因此排查“文件修改不生效”时先检查容器环境。不得在日志、错误或管理响应中输出数据库密码、JWT/TOTP secret、OAuth secret、对象存储 secret 或账号凭据。

## 首次初始化

setup 使用 `DATA_DIR > 可写 /app/data > 当前目录` 选择 `config.yaml` 和 `.installed` 的位置。正常情况下，配置文件或安装锁任一存在就不会重新开放初始化；`SKIP_SETUP` 是部署者的显式旁路。修改这套判断必须保持“删除一个文件不能远程强制重装”的防重置边界。

交互或 `AUTO_SETUP` 流程测试 PostgreSQL/Redis、执行迁移、只在空数据库中创建初始管理员、以 `0600` 写入配置并创建安装锁。已有管理员或已有普通用户时不会覆盖密码。自动 setup 的 `DATABASE_*`、`REDIS_*`、`ADMIN_*`、`SERVER_*`、`JWT_*` 和时区变量是生成初始文件的输入；生成后常规启动仍走统一 config loader。

主服务使用 `LoadForBootstrap`，只在引导阶段允许 `jwt.secret` 暂时为空。数据库 repository 初始化会从 `security_secrets` 读取既有 JWT secret，或原子生成并持久化一个新 secret，然后重新执行完整配置校验。多个实例不能各自使用临时随机 JWT key；显式配置与数据库已有 secret 不一致时，以已持久化的安全边界处理，避免滚动部署让会话随机失效。

## 数据库运行时设置

`settings` 是 `key/value/updated_at` 表，删除键表示恢复该 getter 的默认语义。`SettingService` 负责类型解析、范围/组合校验、敏感值保留、批量原子写入和更新后的缓存通知；handler 只负责 HTTP binding、权限、审计和响应。

运行时设置包括注册与邮件验证、第三方登录、SMTP、TOTP/session binding/step-up、登录协议、面板限流、部分冷却与流超时、数据共享、支付展示以及各类功能开关。不同 getter 的回退可能来自代码常量或 `config.Config`，不能假设所有缺失键都等价于 `false`。

热路径设置必须使用以下一种明确策略：

- 原子 snapshot，更新成功后立即替换；安全客户端 IP 策略属于此类。
- 有 TTL 的进程缓存或 stale-while-revalidate，避免每请求访问数据库。
- Redis/跨实例失效通知，使多个进程最终看到同一设置。
- 仅在启动时加载；此类设置应记录需要重启，不能在 UI 暗示即时生效。

设置写入失败时不得先更新内存 snapshot；数据库成功后若通知失败，要保留可观测错误并允许 TTL/重载恢复。敏感字段在更新请求中省略表示保留原值，不能因管理页面返回掩码或空字符串而清空 secret。

## 领域配置

结构化、有关联和独立生命周期的业务配置使用专用表：

- 分组拥有平台、倍率、能力、回退和策略；渠道拥有模型映射与价格；账号拥有上游凭据、代理和调度状态。
- Subscription Plan、支付 provider instance、API Key、团队和用户属性都有各自不变量与审计路径。
- Ops/pre-aggregation 等既有进程 hard switch 又有运行时开关时，进程开关是上限：数据库不能开启部署者显式硬关闭的能力。

不要把需要唯一约束、外键、状态机、列表查询或原子计数的数据编码成一个巨大 settings JSON。相反，只被单个进程组件启动时读取的连接池大小也不应新建业务表。

## 前端变量

Vite 在构建/dev server 启动时读取 `VITE_API_BASE_URL`、`VITE_WS_BASE_URL`、`VITE_DEV_PROXY_TARGET` 和 `VITE_DEV_PORT`。默认 API base 是 `/api/v1`，dev proxy target 是 `http://localhost:8080`，dev port 是 `3000`。

`VITE_*` 会进入客户端 bundle，绝不能放 secret。生产内嵌前端的动态品牌、功能和公开认证配置通过后端设置注入或 API 获取；它们与 Vite 构建变量是不同通道。改变后端 public URL 时，还要核对 OAuth callback、邮件链接、CORS/CSP 和反向代理路径，不能只改前端 base。

## 新增配置检查

- 判断它属于进程、bootstrap、数据库运行时、领域实体还是前端构建层，并选择唯一权威来源。
- 进程字段添加 `mapstructure`、代码默认值、环境变量可达性、规范化和 `Validate`；必要时更新部署样例。
- 明确环境变量名称、YAML 点分键和优先级；新增兼容旧键时规定弃用与新键优先条件。
- 明确是否热生效、是否多实例一致、缓存如何失效以及失败时 fail-open 还是 fail-close。
- secret 使用 write-only/掩码语义并覆盖日志脱敏；不能暴露到 `VITE_*` 或普通设置响应。
- 更新配置和环境变量可达性、校验、setup 往返、SettingService/handler 以及部署冒烟测试。

相关文档：[HTTP 接口边界](http_api.md)、[系统架构](../architecture/system_architecture.md)、[部署与迁移](../operations/index.md)、[接口目录](index.md)。
