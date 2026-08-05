# 接口文档目录

> 上级目录：[工程文档](../index.md)

## 范围

本分类拥有对外 HTTP 表面、配置来源以及第三方上游适配契约。内部领域不变量由领域文档拥有，部署和维护步骤由运维文档拥有。

## 文档

- [HTTP 接口边界](http_api.md)：公共、用户、管理员、支付和网关路由族及认证/错误边界。读取时机：新增或移动路由、调整中间件、认证方式或公共响应语义时读取。
- [配置边界](configuration.md)：默认值、YAML、环境变量、数据库运行时设置和首次初始化之间的边界。读取时机：新增配置项、修改加载优先级、设置页面或部署变量时读取。
- [Antigravity 上游](antigravity_upstream.md)：Antigravity 专用端点、混合调度及模型协议边界。读取时机：修改 Antigravity 账号、OAuth、Claude/Gemini 转换或调度隔离时读取。
- [Grok / xAI 上游](grok_upstream.md)：Grok OAuth/API Key、媒体资格与 OpenAI 兼容转发契约。读取时机：修改 Grok 登录、聊天、图片、视频、计费探测或模型配置时读取。
- [Qoder Native Upstream](qoder_upstream.md)：Qoder 站点、模型别名、Thinking、上下文、计费和刷新契约。读取时机：修改 Qoder 账号、模型能力、请求转换、定价或运维探测时读取。
- [异步图片任务](async_image_tasks.md)：长耗时图片请求的提交、轮询、存储和计费接口。读取时机：修改异步图片端点、对象存储、任务 TTL、幂等或结果读取时读取。
