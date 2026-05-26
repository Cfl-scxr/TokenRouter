✅ fix: 修复 Anthropic API Key 透传流式保活: https://github.com/Wei-Shaw/sub2api/pull/2552
  - 同步方式：cherry-pick PR head 164e2f610c45ec8062d746c7bd62318cf6639a18。
  - 决策：保留 fork 现有 Anthropic API Key 透传计费、断连后继续读取上游用量和流数据间隔超时逻辑，仅加入空闲 SSE ping 保活。
  - 测试：`go test ./internal/service -run 'TestGatewayService_AnthropicAPIKeyPassthrough_Streaming(SendsKeepaliveDuringIdle|KeepaliveDoesNotInterleavePartialEvent)'`。
✅ fix(payment): 修复支付宝官方扫码二维码生成错误: https://github.com/Wei-Shaw/sub2api/pull/2541
  - 同步方式：cherry-pick PR head 1b03240515a465f878f421cd04f00550ed70e0ac。
  - 决策：保留 fork 现有支付服务商配置 UI 和支付结果处理；支付宝 page.pay 只返回 pay_url，不再把收银台 URL 当作 QRCode。
  - 测试：`go test -tags unit ./internal/payment/provider -run TestCreateTrade -count=1 -v`；`pnpm test:run src/components/payment/__tests__/PaymentProviderDialog.spec.ts src/components/payment/__tests__/providerConfig.spec.ts`。
✅ fix: 统一 Ops SLA 与请求错误统计口径: https://github.com/Wei-Shaw/sub2api/pull/2548
  - 同步方式：按 PR 内提交顺序 cherry-pick ae6ee23e..5dae745e，共 6 个提交。
  - 决策：保留 fork 现有 Ops 上游错误事件、请求钻取和错误趋势字段；仅统一本地路由容量/客户端认证类错误的 SLA 排除口径，并让前端错误图表使用 SLA scoped 数据。
  - 测试：`go test ./internal/handler -run 'TestClassifyOps(NoAvailableAccountsExcludedFromSLA|RoutingCapacityMarkerExcludesMaskedSelectionFailureFromSLA|AuthClientErrorsExcludedFromSLA|UnsupportedModelExcludedFromSLA|UnmarkedNoAvailableTextStillCountsForSLA|UpstreamAuthTextStillCountsForSLA|UpstreamNoAvailableTextStillCountsForSLA)'`；`pnpm test:run src/views/admin/ops/components/__tests__/OpsErrorScopeCharts.spec.ts`。
✅ fix(docker): 固定前端构建阶段 pnpm 版本为 v9: https://github.com/Wei-Shaw/sub2api/pull/2527
  - 同步方式：cherry-pick PR head 44995404ef409772f6779efe4ba1f6b77c07a58f。
  - 决策：仅同步 upstream 实际修改的根 `Dockerfile`；`deploy/Dockerfile` 仍保留 fork 当前内容，未在本条做额外扩大修改。
  - 测试：`git diff --check`；未运行 Docker 构建，因为当前环境没有 `docker` 命令。
✅ fix(codex-transform): preserve underscore when rewriting `call_*` tool-call ids: https://github.com/Wei-Shaw/sub2api/pull/2499
  - 同步方式：cherry-pick PR head 348a487739cf5d0b7e4b44f76003a38adba7a89c。
  - 决策：保留 fork 的 Codex OAuth transform 行为，仅修正 `call_*` 到 `fc_*` 的前缀转换，避免生成缺少下划线的非法 id。
  - 测试：`go test ./internal/service -run 'Test.*Codex.*(Call|Tool|Reference|Input|Transform)'` 当前失败在旧断言仍期待 `fc1`/`fccustom`，下一条 direct commit 会同步这些断言到 `fc_` 口径。
✅ test: align codex tool-call id assertions with fc_ prefix: https://github.com/TokenFlux/TokenRouter/commit/a729752de6953e0b92e85926340ee40a730113fa
  - 同步方式：fetch origin direct commit a729752de6953e0b92e85926340ee40a730113fa 后 cherry-pick。
  - 决策：仅同步 Codex transform 测试断言，使其匹配上一条 `call_*` -> `fc_*` 修复后的真实输出。
  - 测试：`go test ./internal/service -run 'Test.*Codex.*(Call|Tool|Reference|Input|Transform)'`。
✅ 修复 Claude 映射 GPT 后被记为 0 token 的计费漏洞: https://github.com/Wei-Shaw/sub2api/pull/2505
  - 同步方式：cherry-pick PR head 0393bd7c82da1ca385af0744e9e19c1490c93bc0，并补充 fork 的 `populateOpenAIUsageFromResponseJSON` 路径使用同一 usage 解析函数。
  - 决策：同时兼容 Responses usage（input/output）与 Chat Completions usage（prompt/completion），避免 Claude 映射到 GPT 后因上游返回 chat usage 字段而按 0 token 计费。
  - 测试：`go test ./internal/service -run 'TestForwardAsAnthropic_MappedClaudeModelAcceptsChatUsageShape|TestParseSSEUsage_SelectiveParsing|TestExtractOpenAIUsageFromJSONBytes_AcceptsResponseAndChatUsageShapes|TestParseOpenAIWSResponseUsageFromCompletedEvent|TestPopulateOpenAIUsageFromResponseJSONAcceptsChatUsageShape'`；`go test ./internal/pkg/apicompat -run TestResponsesUsageUnmarshalAcceptsChatUsageShape`。
✅ fix(admin/settings): make tab shell readable in dark mode: https://github.com/Wei-Shaw/sub2api/pull/2504
  - 同步方式：cherry-pick PR head b0c7723393a43ffca8277f597f375e148ba3ff2f，手动解决 `SettingsView.vue` 样式冲突。
  - 决策：保留 fork 现有 Settings Tab 结构和样式参数；仅把暗色模式覆盖迁移到非 scoped `<style>` 块，并将说明注释改为中文。
  - 测试：`pnpm test:run src/views/admin/__tests__/SettingsView.spec.ts`。
✅ fix: 修复 OpenAI 模型容量错误未进入自动重试: https://github.com/Wei-Shaw/sub2api/pull/2481
  - 同步方式：cherry-pick PR head 9f07741c139e05463983b236faf9a2b269af0fe4，并补充 fork 的定向单元测试。
  - 决策：保留 fork 现有 OpenAI failover/重试框架，仅把 `selected model is at capacity` 纳入临时处理错误，并让流式 `response.failed` 复用同一判定。
  - 测试：`go test ./internal/service -run 'TestIsOpenAITransientProcessingError|TestOpenAIStreamingResponseFailed(BeforeOutput|CapacityBeforeOutput)ReturnsFailover|TestOpenAIGatewayService_Forward_RetriesOpenAITransientProcessingError'`。
✅ 修复图片计费尺寸归一化与使用记录展示: https://github.com/Wei-Shaw/sub2api/pull/2401
  - 同步方式：按 PR 内提交顺序 cherry-pick bb4c1abe285661aaaea2bf4241bc2c3c03912439、4840194b181784e2cecac7bcc8b2addedc747a14、115535e1d51a429cd15e21d32b853b384945853e。
  - 决策：保留 fork 的订阅/余额拆分计费、`billing_allocations`、`request_type`、OpenAI WS 标记、账号统计成本、service tier、端点和模型映射链字段，同时合入 upstream 的图片尺寸归一化、图片尺寸来源/明细持久化和用量展示修复。
  - 测试：`go test ./internal/service -run 'Test(ClassifyImageBillingTier|ResolveImageBillingSize|GatewayServiceRecordUsage_(ImageIndependentMultiplierUsesImageRate|EmptyImageSizeDefaultsBeforeBillingAndPersistence)|OpenAIGatewayService_.*Image|.*Image.*Billing|ExtractImageSize)'`；`go test ./internal/repository -run 'TestPrepareUsageLogInsert_|TestAppendUsageLogBillingModeWhereCondition|TestScanUsageLogRequestTypeAndLegacyFallback|TestUsageLogRepositoryCreate'`；`go test ./internal/handler/dto -run 'TestUsageLogFromService_.*Image'`；`pnpm test:run src/components/admin/usage/__tests__/UsageTable.spec.ts src/views/user/__tests__/UsageView.spec.ts`。
✅ fix: use tier cooldown for google one gemini 429: https://github.com/Wei-Shaw/sub2api/pull/2502
  - 同步方式：cherry-pick PR head 87fac30459a66c803c294f1368b6a94d1474f437，并将新增注释改为中文。
  - 决策：保留 fork 现有 Gemini 429/error policy 流程，仅让 Google One OAuth 账号在无 reset 时间时复用 tier cooldown，不再走 PST 午夜重置。
  - 测试：`go test -tags unit ./internal/service -run 'TestHandleGeminiUpstreamError_GoogleOneCapacityExhaustedUsesTierCooldown|TestGeminiErrorPolicy|TestShouldFailoverGeminiUpstreamError'`。
✅ fix: 修复管理后台分组页可用账号数显示错误: https://github.com/Wei-Shaw/sub2api/pull/2508
  - 同步方式：cherry-pick PR head 360f8dec1aefedf700f0eb7b71f5feac8658e424。
  - 决策：分组页“可用账号”直接展示后端给出的 `active_account_count`，不再前端扣减 `rate_limited_account_count`；验证时顺手补上上一条图片用量 tooltip 的 `tooltipData` 空值收窄，保证全量前端 typecheck 通过。
  - 测试：`pnpm test:run src/views/admin/__tests__/groupsMessagesDispatch.spec.ts`；`pnpm typecheck`；`pnpm test:run src/components/admin/usage/__tests__/UsageTable.spec.ts src/views/user/__tests__/UsageView.spec.ts src/views/admin/__tests__/groupsMessagesDispatch.spec.ts`。
✅ fix(gateway): 修复图片生成上游请求过早取消: https://github.com/Wei-Shaw/sub2api/pull/2461
  - 同步方式：cherry-pick PR head a611742910bd8e33f19fa79fa74687e3618cf77c。
  - 决策：保留 fork 当前图片计费/尺寸统计逻辑，仅将 API Key 与 OAuth 图片生成上游请求改为无条件 `detachUpstreamContext`，避免客户端断连提前取消上游生成。
  - 测试：`go test ./internal/service -run 'Test(OpenAIImages|ForwardOpenAIImages|ParseOpenAIImages|BuildOpenAIImages|HandleOpenAIImages|OpenAIImage|DownloadOpenAIImage|DetachUpstreamContext|GatewayService_Detach)'`。
✅ fix(channels): 按次/图片计费模式下区间校验跳过 token 上下文重叠规则: https://github.com/Wei-Shaw/sub2api/pull/2473
  - 同步方式：PR head 含 upstream main merge，未整体 cherry-pick；仅按功能提交同步 b936925c8aa4a0b5d436a39f84339a9f6a38e7fd 与 ff6f1640c46c2fa205bcb4b49e95e72d91afe632。
  - 决策：保留 fork 当前渠道模型/映射测试边界，只让 token 计费继续做区间重叠和无界区间位置校验；per_request/image 按 tier_label 匹配，跳过 token 区间重叠校验但仍校验单条 min/max 与价格非负。
  - 测试：`go test -tags unit ./internal/service -run 'TestValidateIntervals'`；`pnpm test:run src/components/admin/channel/__tests__/types.spec.ts`；`pnpm typecheck`。
✅ feat(channels): 「可用渠道」对未填价的 pricing 条目按 LiteLLM 默认价展示: https://github.com/Wei-Shaw/sub2api/pull/2475
  - 同步方式：检查 PR 功能提交 c26d3ae1b5e5337fde63bb79ef3dfae3ed1de84f；未应用代码。
  - 决策：当前 fork 没有 upstream 的 `channel_available.go`、`AvailableChannel`、`ListAvailable` 或“可用渠道”展示接口/页面，PR 修改目标链路不存在；为避免引入未接线的新展示服务，本条标记为不适用。
  - 测试：`rg -n "available.*channel|channels.*available|ListAvailable|availableChannels|可用渠道|AvailableChannel" . --glob '!node_modules' --glob '!frontend/dist' --glob '!backend/ent'`。
✅ fix: 默认透传 OpenAI service_tier: https://github.com/Wei-Shaw/sub2api/pull/2457
  - 同步方式：cherry-pick PR head e9637148ddd02e01778975ab6986ecf67ca84c2e。
  - 决策：保留 fork 的 OpenAI fast policy 框架、WS 逐帧计费记录和显式 filter/block 规则能力；仅将默认配置改为空规则，使 HTTP/WS 默认透传上游 `service_tier`，并在 pass 路径继续把 `fast` 别名归一化为 `priority`。
  - 测试：`go test ./internal/service -run 'TestEvaluateOpenAIFastPolicy|TestApplyOpenAIFastPolicyToBody|TestWSResponseCreate|TestPolicyEnforcingFrameConn|TestPassthroughBilling|TestForwardAsAnthropicMessages_BetaFastMode|TestOpenAIServiceTier|TestNormalizeOpenAIServiceTier'`；`go test -tags unit ./internal/server -run TestAPIContracts`。
✅ feat: 支持从上游同步账号可用模型列表: https://github.com/Wei-Shaw/sub2api/pull/2554
  - 同步方式：按 PR 功能提交顺序同步 ba676e43fd3643164f9c6dc9ba849267eb05b539、b9ecf252076563b8326ecd7b347d3f724a4940c8、36c00374d30395c3bfbc75d14f2c52bd4835808a、3b4eccdd5d24e2a3ba0b4c5da7a66de071570d38、5713820813e29eadcfd102c19b0fe7e4c31ea125。
  - 决策：保留 fork 已有 OpenAI OAuth 模型白名单/映射合并语义和 Antigravity 仅映射模式；新增上游模型同步服务、admin API、前端同步按钮，并将 upstream 模块路径与新增注释调整为本 fork 规范。
  - 测试：`go test ./internal/service -run 'Test(BuildV1ModelsURL|BuildGeminiModelsURL|ExtractUpstreamModelIDs|BuildUpstreamModelsRequestsForAPIKeyAccounts|BuildAntigravityAPIKeyModelsRequestRejectsOfficialCloudCodeBase|BuildAnthropicUpstreamModelsRequestRejectsBedrock|FetchUpstreamSupportedModels)'`；`go test ./internal/handler/admin -run 'TestAccountHandler(GetAvailableModels|SyncUpstreamModels)'`；`go test ./internal/pkg/antigravity -run 'TestClient_FetchAvailableModels|TestDefaultModels'`；`pnpm typecheck`；`pnpm test:run src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/BulkEditAccountModal.spec.ts`。
✅ fix: 安装脚本提前检查 Bash 版本: https://github.com/Wei-Shaw/sub2api/pull/2447
  - 同步方式：手工移植 PR head a5acefcc9e598622b5b741540a9e62f7d8011635 的单文件变更。
  - 决策：保留 upstream 的 Bash 4+ 前置检查逻辑，将新增注释改为中文；检查放在 `declare -A` 之前，避免旧 Bash 先因语法报错。
  - 测试：`bash -n deploy/install.sh`。
✅ fix(openai): 修复 chat-completions 转 responses 时 content 为 null 导致上游 400: https://github.com/Wei-Shaw/sub2api/pull/2528
  - 同步方式：cherry-pick PR head df82a3bc694a1b2c9511fb8066f9bc9fcbb23b1c。
  - 决策：保留 fork 现有 chat-completions 到 responses 的图片过滤和 reasoning/thinking 转换语义；仅在无可用 content parts 时把 content 序列化为空字符串，避免上游收到 JSON null。
  - 测试：`go test ./internal/pkg/apicompat -run 'TestChatCompletionsToResponses_(EmptyContentNeverNull|EmptyBase64ImageURLSkipped|WhitespaceOnlyBase64ImageURLSkipped|SystemArrayContent|BasicText|ImageURL)'`。
✅ fix(openai): handle versioned compatible base URLs: https://github.com/Wei-Shaw/sub2api/pull/2424
  - 同步方式：cherry-pick PR head 679c0865a0832a6aac4df88f071c6c87c0dd2790。
  - 决策：保留 fork 已有 OpenAI Responses 探测失败后回退 raw chat-completions、service_tier 过滤计费、图片端点拼接和 /responses/compact 透传语义；新增统一 endpoint URL builder，支持 `/v4` 等版本化兼容上游 base_url。
  - 测试：`go test -tags unit ./internal/service -run 'TestBuildOpenAI(ChatCompletionsURL|ResponsesURL_ProbeURL)|TestForwardAsChatCompletions_UnknownResponsesSupportFallbackUsesVersionedChatURL|TestForwardAsRawChatCompletions_UsesFilteredServiceTierForBilling|TestBuildOpenAIImagesURL_HandlesVersionedBaseURL'`。
✅ fix: 修复兑换码生成后复制全部报错: https://github.com/Wei-Shaw/sub2api/pull/2448
  - 同步方式：手工移植 PR head 4d51e53d20cd46b092117f8157b0d38ec7a2c7e3 的单文件变更。
  - 决策：批量复制生成后的兑换码复用现有 `useClipboard` 封装，与单个复制行为一致，保留 fork 的结果弹窗状态逻辑。
  - 测试：`pnpm typecheck`。
✅ fix: 修复订阅支付商品名前后缀不生效: https://github.com/Wei-Shaw/sub2api/pull/2449
  - 同步方式：cherry-pick PR head 61b6272110914666ebd6e17e95e8980b572bfdca，手动解决 `payment_order.go` 中微信 OAuth fee breakdown 签名差异。
  - 决策：保留 fork 现有微信支付 OAuth `payment.FeeBreakdown` 流程；仅复用 upstream 的商品名前后缀 helper，让订阅计划自定义商品名和默认商品名都应用支付配置的前后缀。
  - 测试：`go test ./internal/service -run 'TestBuildPaymentSubject(AppliesAffixToSubscriptionPlanProductName|AppliesAffixToSubscriptionPlanDefaultName)'`。
✅ feat: 支持后台配置 OpenAI Responses API 路由: https://github.com/Wei-Shaw/sub2api/pull/2450
  - 同步方式：cherry-pick PR head 862819042ca871726710e2cafce0dc9c218b2087，并将新增英文注释改为中文。
  - 决策：保留 fork 已有 OpenAI WS mode、compact、模型白名单/映射和上游模型同步 UI；新增 `openai_responses_mode` 账号级覆盖项，在 auto 模式继续沿用探测结果，强制模式覆盖探测并透传到调度缓存。
  - 测试：`go test ./internal/pkg/openai_compat -run 'Test(ResolveResponsesSupport|ShouldUseResponsesAPI|NormalizeResponsesSupportMode)'`；`go test ./internal/repository -run 'Test(ShouldEnqueueSchedulerOutboxForExtraUpdates_OpenAIResponsesCapabilityKeysAreRelevant|BuildSchedulerMetadataAccount_KeepsOpenAIWSFlags)'`；`go test -tags unit ./internal/service -run 'TestForwardAsChatCompletions_UnknownResponsesSupportFallbackUsesVersionedChatURL'`；`pnpm test:run src/components/account/__tests__/EditAccountModal.spec.ts`；`pnpm typecheck`。
✅ fix(gateway): 修复 Gemini 组 Chat Completions 路由: https://github.com/Wei-Shaw/sub2api/pull/2451
  - 同步方式：按 PR 内提交顺序 cherry-pick 041d138f76d020936a5193a3d7ab7e741f56c0c6、f9d5ccdf24a6d2a00ed7a9969e8fd2b66074c887、2ec1d331e06853a0b5425e9faebb6e99fc97be1c，手动解决 `gateway_handler.go` import 冲突。
  - 决策：保留 fork 已有 Antigravity 默认模型列表、Gemini 多平台调度和图片尺寸归一化计费口径；新增 Gemini 分组的 Chat Completions 兼容转发、Gemini 专用 failover 上限、非 Gemini 账号跳过保护，并让 `/v1/models` 按分组平台过滤模型和回退 Gemini 默认模型。
  - 测试：`go test ./internal/service -run 'TestGeminiForwardAsChatCompletions|TestGeminiMessagesCompatServiceForward'`；`go test ./internal/handler -run 'TestGatewayModels_GeminiGroup'`；`go test ./internal/service -run 'TestGemini(ForwardAsChatCompletions|MessagesCompatServiceForward|MessagesCompatService_SelectAccountForModelWithExclusions|ErrorPolicy|ForwardNative)'`；`go test ./internal/handler -run 'TestGatewayModels_GeminiGroup|TestGeminiV1Beta|TestGatewayHandler'`。
✅ fix(account): 保留模型白名单和模型映射组合配置: https://github.com/Wei-Shaw/sub2api/pull/2454
  - 同步方式：检查 PR head 827764d7bdbb1b2d4e554088b618d3293aa3a3f3；未照搬 upstream 的 `combined` 存储实现，仅补 fork 语义下的回归测试。
  - 决策：当前 fork 已将普通账号最终白名单拆到独立 `model_whitelist` 字段，请求侧映射继续放 `model_mapping`，比 upstream 把两者合并进 `model_mapping` 更能避免歧义；保留该行为，并确认编辑白名单时不会丢失已有映射。
  - 测试：`pnpm test:run src/components/account/__tests__/EditAccountModal.spec.ts`；`pnpm test:run src/composables/__tests__/useModelWhitelist.spec.ts`；`pnpm typecheck`。
✅ fix(repo): 公告查询添加分页上限，优化分组按账户数排序的数据加载方式: https://github.com/Wei-Shaw/sub2api/pull/2456
  - 同步方式：cherry-pick PR head ab6510f1a0f0b1a6cbccc00c035f12d00fd1f25a。
  - 决策：保留 fork 现有分组 Lite 查询和账号统计字段语义；公告活跃查询限制为最多 200 条，分组按账号数排序改为先查轻量 ID/sort_order 和账号统计，分页后只加载当前页完整实体。
  - 测试：`go test ./internal/repository -run 'Test(Group|Announcement)'`。
✅ fix(passthrough): 修复WSv2模式下first_token_ms测量错误: https://github.com/Wei-Shaw/sub2api/pull/2444
  - 同步方式：cherry-pick PR head be15a3e6cebe68cf9878d9331c508c46229da451。
  - 决策：保留 fork 现有 WSv2 relay 的 per-turn usage/trace 逻辑；新增 active turn timing，用真实 OpenAI 流里不带 response_id 的 delta 事件也能记录本轮 first_token_ms，避免等到 terminal 事件才打点。
  - 测试：`go test ./internal/service/openai_ws_v2 -run 'TestRelay_OnTurnComplete_(RealOpenAIStream_FirstTokenMs|ProvidesTurnMetrics|PerTerminalEvent)'`。
✅ fix: 修复日卡跨日重复刷新额度: https://github.com/Wei-Shaw/sub2api/pull/2557
  - 同步方式：按 PR 功能提交手工移植后端一次性日额度判断、前端日卡额度结束提示和 i18n 文案。
  - 决策：保留 fork 当前订阅授予模型，未照搬 upstream 旧的过期订阅复用更新路径；当前 fork 对过期同计划续费会创建新的当前周期，未过期同计划续费继续创建 pending 链，避免破坏已有排队订阅语义。
  - 测试：`go test ./internal/service -run 'Test(AssignOrExtendSubscription_ExpiredDailyCardStartsNewOneTimeQuota|UserSubscriptionNeedsDailyReset|UserSubscriptionDailyResetTime|CheckAndResetWindows|ValidateAndCheckLimits_DailyCard|CalculateProgress_DailyCard)'`；`go test ./internal/service -run 'Test(AssignSubscription|AssignOrExtendSubscription|UserSubscription|CheckAndResetWindows|ValidateAndCheckLimits|CalculateProgress|AdminResetQuota)'`；`pnpm typecheck`；`pnpm test:run src/views/user/__tests__/SubscriptionsView.spec.ts`；`git diff --check`。
✅ fix(gemini): 修复 chat completions compat 编译失败: https://github.com/TokenFlux/TokenRouter/commit/0c34e31881129520b97647903f06ba682b2fc84b
  - 同步方式：cherry-pick direct commit 0c34e31881129520b97647903f06ba682b2fc84b，手动解决 `gemini_chat_completions_compat_service.go` 图片尺寸来源冲突。
  - 决策：保留 fork 当前从转换后的 Gemini 请求体 `geminiReq` 读取 `imageInputSize` 的行为；同步 direct commit 中补齐 `ForwardResult.ImageInputSize` 的修复，避免 Chat Completions compat 图片路径缺失原始尺寸字段。
  - 测试：`go test ./internal/service -run 'TestGeminiForwardAsChatCompletions|TestGeminiMessagesCompatServiceForward|TestGemini(ForwardAsChatCompletions|MessagesCompatServiceForward)'`。
✅ fix(auth): prefer OIDC compat email in pending flow: https://github.com/Wei-Shaw/sub2api/pull/2395
  - 同步方式：cherry-pick PR head 224e9fc6c2bdd4a20d66e7cd2ec96fc68a0f43a2。
  - 决策：仅同步 OIDC callback 前端 pending 账号邮箱优先级，补充 `compat_email` 字段并在 `resolved_email` 前使用，后端 pending OAuth 流程保持 fork 当前实现。
  - 测试：`pnpm typecheck`；`NODE_OPTIONS="--localstorage-file=/tmp/tokenrouter-oidc-vitest-localstorage" pnpm test:run src/views/auth/__tests__/OidcCallbackView.spec.ts`。直接运行同一测试在当前 Node 26 环境会因未配置 `--localstorage-file` 缺少全局 `localStorage` 而在测试初始化失败。
✅ fix(openai): add codex auto review model pricing: https://github.com/Wei-Shaw/sub2api/pull/2412
  - 同步方式：手工移植 PR head 32a79be962663eb22e5e6fce0fc474d536880d98 的模型映射、测试和默认价格资源变更。
  - 决策：保留 fork 当前更完整的 Codex 模型归一化和运行时 `backend/data/model_pricing.json`；仅补齐 tracked fallback 资源里的 `codex-auto-review`，并显式透传该模型，避免被通用 codex 规则折到 `gpt-5.3-codex`。
  - 测试：`go test ./internal/service -run 'Test(NormalizeCodexModel_Gpt53|NormalizeOpenAIModelForUpstream|UsageBillingModelCandidatesPreserveCodexAutoReviewModel|DefaultPricingIncludesCodexAutoReview|GetModelPricing_)'`；`go test ./internal/service -run 'Test.*Codex.*|Test.*Pricing.*|TestNormalizeOpenAIModelForUpstream|TestUsageBillingModelCandidatesPreserveCodexAutoReviewModel'`；`go test ./internal/service -run TestDefaultPricingIncludesCodexAutoReview`；`python -m json.tool backend/resources/model-pricing/model_prices_and_context_window.json >/dev/null`；`git diff --check`。
✅ fix: add autocomplete for TOTP autofill support: https://github.com/Wei-Shaw/sub2api/pull/2393
  - 同步方式：手工移植 PR head 4467922199f8dd2bc777c5eca7fe39befbcbf14c 的登录 2FA 弹窗改动，并补充本地组件测试。
  - 决策：保留 fork 当前分格验证码输入和 toast 错误展示；新增隐藏 `autocomplete="one-time-code"` 输入承接密码管理器/TOTP 自动填充，并复用同一填充函数处理粘贴和自动填充。
  - 测试：`pnpm test:run src/components/auth/__tests__/TotpLoginModal.spec.ts`；`pnpm typecheck`；`git diff --check`。
✅ style(openai-ws): 修复 passthrough_relay gofmt 对齐: https://github.com/TokenFlux/TokenRouter/commit/b006e36af912dee1dcdf16dd8cd6c564947c6dff
  - 同步方式：按 direct commit b006e36af912dee1dcdf16dd8cd6c564947c6dff 对 `passthrough_relay.go` 运行 gofmt。
  - 决策：仅同步字段对齐格式修复，不改变 WSv2 passthrough relay 行为。
  - 测试：`go test ./internal/service/openai_ws_v2 -run TestRelay`；`git diff --check`。
✅ feat(usage): 用户用量按平台拆分 + UsersView 列设置可配置 + 用量列排序: https://github.com/Wei-Shaw/sub2api/pull/2416
  - 同步方式：cherry-pick PR 功能提交 664e9fdcd4408ed9c7489c0f1d70dc9c81340db8，手动解决 `UserDashboardStats.vue` 与 `UsersView.vue` 冲突，并补充批量用量平台拆分单元测试。
  - 决策：保留 fork 的自定义余额单位展示，平台费用组件统一使用 `useBalanceDisplay().formatBalanceAmount(..., { fractionDigits: 4 })`；平台口径采用 upstream 的有效平台语义 `COALESCE(NULLIF(g.platform,''), a.platform)`，即分组 platform 优先，否则回退账号 platform。
  - 决策：保留 upstream 的 `actual_cost > 0` 平台统计过滤，避免失败占位 usage log 污染平台拆分；因此成功但 0 费用/未定价请求不会进入平台拆分的请求数与 token 数，作为 upstream 既有口径记录。
  - 决策：UsersView 新增平台用量列默认隐藏，并通过列设置版本 2 给老用户自动隐藏一次；`last_active_at` 不再强制可见，允许用户隐藏；用量列排序维持 upstream 的当前页前端排序，不改变服务端排序状态。
  - 测试：`go test ./internal/repository -run 'TestUsageLogRepositoryGetBatchUserUsageStatsSplitsByEffectivePlatform|Test.*BatchUserUsageStats|Test.*UserDashboardStats|TestPrepareUsageLogInsert_|TestAppendUsageLogBillingModeWhereCondition|TestUsageLogRepositoryCreate'`；`NODE_OPTIONS="--localstorage-file=/tmp/tokenrouter-usersview-vitest-localstorage" pnpm test:run src/views/admin/__tests__/UsersView.spec.ts`；`pnpm typecheck`；`git diff HEAD --check`。
  - 备注：当前 Node 26 环境直接运行 UsersView 测试会缺少全局 `localStorage`，需带 `--localstorage-file`。
✅ feat(dingtalk): 钉钉 OAuth 登录接入 + internal_only 用户属性同步: https://github.com/Wei-Shaw/sub2api/pull/2492
  - 同步方式：cherry-pick PR 功能提交 b19da9c7fea8a6ae075d2d265dec30c0ed75e8d9，手动解决配置、handler、service、后台设置页和登录页冲突。
  - 决策：保留 fork 已有 GitHub/Google 邮箱 OAuth、LinuxDo/OIDC/WeChat pending OAuth、用户/支付/用量 handler 构造签名、余额单位和订阅发放语义；钉钉注册/绑定统一接入 fork 的 auth source defaults 与 pending OAuth 流程。
  - 决策：保留 fork 当前邀请返利实现（`affiliate_enabled` + `referral_reward_amount`），不引入 upstream 未在本 fork 落地的 `AffiliateRebate*` 设置；钉钉 `aff_code` 透传到既有 referral 解析，pending OAuth 创建账号不提前核销邀请码。
  - 决策：钉钉 internal_only、bypass_registration、企业邮箱/姓名/部门属性同步、合成邮箱、账号绑定状态与后台设置项采用 upstream 行为；同步 attr definition 缺失时只记录日志，不阻塞登录。
  - 决策：修正 upstream module path 为 `github.com/TokenFlux/TokenRouter`，并把新增/冲突解析注释调整为中文。
  - 测试：`go test ./internal/config -run TestDingTalk`；`go test ./internal/service -run 'Test.*DingTalk|TestAuthService_LoginOrRegisterOAuthWithTokenPair|Test.*AuthSource.*Default|Test.*ReservedEmail'`；`go test -tags unit ./internal/service -run 'TestRegisterOAuthEmailAccount|TestAuthService_LoginOrRegisterOAuthWithTokenPair|Test.*DingTalk|Test.*AuthSource.*Default|Test.*ReservedEmail'`。
  - 测试：`go test ./internal/handler -run 'Test.*DingTalk|Test.*OAuth.*Pending|Test.*LinuxDo|Test.*OIDC|Test.*WeChat'`；`go test -tags unit ./internal/server -run 'TestAPIContracts|TestBackendMode'`；`pnpm typecheck`；`NODE_OPTIONS="--localstorage-file=/tmp/tokenrouter-settings-vitest-localstorage" pnpm test:run src/views/admin/__tests__/SettingsView.spec.ts`；`git diff --check`。
  - 备注：SettingsView 单测通过，但仍输出既有 `router-link` 组件未注册 warning；未影响断言。
✅ fix: avoid ops deep link initialization error: https://github.com/Wei-Shaw/sub2api/pull/2501
  - 同步方式：cherry-pick PR 提交 e46d2c2112b9d232c945231954f7fdc1a972909c，无冲突。
  - 决策：仅把 `applyRouteQueryToState()` 延后到 Ops 深链相关响应式状态初始化之后，避免 `open_error_details`/`open_alert_rules` 等深链参数在初始化阶段访问未初始化的 ref；不改变查询同步和自动刷新逻辑。
  - 测试：`pnpm typecheck`；`pnpm test:run src/views/admin/ops/components/__tests__/OpsErrorScopeCharts.spec.ts src/views/admin/ops/components/__tests__/OpsOpenAITokenStatsCard.spec.ts src/views/admin/ops/utils/__tests__/errorDetailResponse.spec.ts`；`git diff --check`；`git diff --cached --check`。
  - 备注：OpsOpenAITokenStatsCard 测试中的失败日志来自用例主动模拟接口异常；Vitest 输出 Node `localStorage` warning，未影响断言。
✅ fix(openai): 新建账号弹窗补全 Responses API 路由选项: https://github.com/TokenFlux/TokenRouter/commit/90a389342c5ce81b5b9e2a41c2f982ea0a238dd7
  - 同步方式：cherry-pick direct commit 90a389342c5ce81b5b9e2a41c2f982ea0a238dd7，无冲突；将新增 HTML 注释改为中文。
  - 决策：仅在 OpenAI API Key 新建账号弹窗加入 Responses API 支持模式选择，并在提交时写入既有 `extra.openai_responses_mode`；默认 `auto` 时不写入，保持既有账号创建行为。
  - 测试：`pnpm typecheck`；`NODE_OPTIONS="--localstorage-file=/tmp/tokenrouter-account-tests-localstorage" pnpm test:run src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/BulkEditAccountModal.spec.ts src/components/account/__tests__/AccountTestModal.spec.ts`；`git diff --cached --check`。
  - 备注：不带 `--localstorage-file` 运行 AccountTestModal 测试会因 Node 26 无全局 `localStorage` 失败，带参数重跑通过。
✅ fix(setup): 初始化完成后阻止访问 setup 页面: https://github.com/Wei-Shaw/sub2api/pull/2568
  - 同步方式：cherry-pick PR 提交 a9a357e9ab4c37c2b87dbaf58bfde139af9aa4e9，无冲突；将新增英文注释改为中文。
  - 决策：访问 `/setup` 时查询 setup 状态，若后端已初始化则按当前登录状态跳转到 `/login`、`/dashboard` 或 `/admin/dashboard`；状态查询失败时保留 setup 页面可访问，避免阻断首次安装或后端异常恢复。
  - 测试：`pnpm test:run src/router/__tests__/guards.spec.ts`；`pnpm typecheck`；`git diff --cached --check`。
✅ fix(ops): 用户 IP 限制导致的 ACCESS_DENIED 不计入 SLA 错误: https://github.com/Wei-Shaw/sub2api/pull/2570
  - 同步方式：cherry-pick PR 提交 271aba1abe1976f8c3b2f8f3edd9ca43ab31b118，无冲突；将新增注释改为中文。
  - 决策：API Key IP 黑白名单拒绝仍写入 ops_error_logs 便于审计，但通过 `OpsClientBusinessLimited` 标记归为客户端侧业务限制，从 SLA/错误率计算排除；非该标记的内部错误继续计入 SLA。
  - 测试：`go test ./internal/handler -run 'TestClassifyOps.*SLA|TestClassifyOpsIPRestriction|TestClassifyOpsOtherErrors|TestClassifyOpsUnsupportedModel'`；`go test -tags unit ./internal/server/middleware -run 'TestAPIKeyAuthIPRestriction|TestAPIKeyAuth'`；`git diff --check`。
✅ fix(security): 屏蔽 admin 账号接口返回的敏感凭证字段: https://github.com/Wei-Shaw/sub2api/pull/2271
  - 同步方式：按 PR 内提交顺序同步 0f8e2d0934434e5c99d2c2a0b128eb07b6586f1d 与 3ca232ad0611b883ed25ad4e878c399abcfa5851；第二个前端兼容提交因第一个提交已在工作区调整注释和模块路径，改为手工移植。
  - 决策：admin 账号 list/get/create/update/refresh/batch 等 DTO 响应统一通过 `RedactCredentials` 剥离 `access_token`、`refresh_token`、`api_key`、Service Account 私钥等敏感子键，并用 `credentials_status.has_<key>` 暴露存在性；`/admin/accounts/data` 导出继续使用独立 `DataAccount`，保留完整 credentials 作为管理员显式备份路径。
  - 决策：`UpdateAccount` 对敏感 credentials 子键采用 incoming 未提供则保留 existing 的合并语义，避免前端拿到脱敏响应后全对象 PUT 清空旧 token；非敏感键仍由 incoming 决定，允许删除/修改。前端账号编辑弹窗优先读 `credentials_status`，并兼容旧后端仍返回明文 credentials 的灰度场景。
  - 测试：`go test ./internal/handler/dto -run 'Test(RedactCredentials|AccountFromServiceShallow_RedactsSensitiveCredentials|AccountFromServiceShallow_NilCredentialsOmitsStatus)'`；`go test -tags unit ./internal/service -run 'Test(MergePreservingSensitiveCreds|UpdateAccount_(PreservesSensitiveCredsWhenIncomingOmits|ExplicitNewTokenOverwrites|EmptyCredentialsSkipsUpdate))'`；`NODE_OPTIONS="--localstorage-file=/tmp/tokenrouter-account-tests-localstorage" pnpm test:run src/components/account/__tests__/EditAccountModal.spec.ts`；`pnpm typecheck`；`git diff --check`。
  - 备注：补充运行 `go test ./internal/handler/admin -run TestExportDataIncludesSecrets` 时，当前包编译被既有 `setting_handler_auth_source_defaults_test.go` 中 `NewSettingHandler` 参数数量不匹配阻塞，未进入导出用例；本条通过保留独立 `DataAccount` 路径避免导出被 DTO 脱敏影响。
✅ fix(openai): 识别上游静默拒绝(空流+finish_reason=stop)并触发 failover: https://github.com/Wei-Shaw/sub2api/pull/2572
  - 同步方式：PR 分支包含大量已同步历史，仅 cherry-pick tip 提交 6381f9e37d7b740459b88eae99b20188de94349e；手动解决 `openai_chat_completions.go` 与 `openai_gateway_handler.go` 中 failover 分支冲突，并修正 upstream 模块路径。
  - 决策：保留 fork 的 OpenAI Cyber warning 记录逻辑；当上游返回静默拒绝触发 `UpstreamFailoverError` 时仍记录风控 warning，但若本次 Forward 已经写出客户端流响应，则立即走流式错误收尾，不再继续同账号重试或切换账号，避免二次写响应。
  - 决策：OpenAI Responses->Chat Completions 转换流和 raw Chat Completions 直转流都延迟写入下游 SSE header/chunk，直到确认不是“大请求空内容 + finish_reason=stop + 无 usage”的静默拒绝；tool/function call、reasoning、正常 content、usage/error 继续直接放行。failover 耗尽时统一返回 502 `openai_silent_refusal` 客户端文案，并写入 Ops upstream error。
  - 测试：`go test -tags unit ./internal/service -run 'Test(ForwardAsRawChatCompletions_(SilentRefusal|ForcesStreamUsageUpstreamAndPassesUsageDownstream|ClientDisconnectDrainsUsage|UpstreamRequestIgnoresClientCancel|UsesFilteredServiceTierForBilling)|HandleChatStreamingResponse_SilentRefusal)'`；`go test ./internal/handler -run 'Test.*Failover|Test.*OpenAI.*Silent|Test.*ChatCompletions|Test.*Responses'`；`git diff --check`；`git diff --cached --check`。
✅ feat(redeem): 兑换码支持设置使用有效期: https://github.com/Wei-Shaw/sub2api/pull/2573
  - 同步方式：PR 分支包含大量已同步历史，未整段 cherry-pick；检查 tip 提交 e4aaf0af297abe70122a73891aeb3d1fc0064d19 后手工移植缺口。
  - 决策：保留 fork 已有 `redeem_codes.expires_at` 迁移 108、Ent 生成物、`plan_id` 订阅套餐模型、`max_uses/used_count` 多次兑换语义和导出列；不引入 upstream 旧的 `group_id/validity_days` 订阅兑换码模型，也不新增重复迁移 137。
  - 决策：新增 admin 生成/固定码接口对 `expires_in_days` 的兼容解析，并继续支持既有 `expires_at` Unix 秒；两者同时传入时报错，绝对过期时间必须是未来时间。邀请码现在可设置过期时间，但仍固定单次使用。
  - 决策：兑换码有效状态、OAuth/普通注册邀请码校验、repository 列表/筛选状态统一使用 `CanUse`/自然过期语义，过期邀请码不再被视为可用；create-and-redeem 对已由同一用户完成的固定码仍保持幂等成功。
  - 测试：`go test ./internal/service -run 'TestRedeemCodeExpirySemantics|TestAdminService_UpdateRedeemCode_(UnusedBalanceUpdatesValueLimitAndExpiry|RestoresExpiredCodeWhenExpiryCleared|InvitationKeepsExpiry|RejectsSystemRecords)'`；`go test -tags unit ./internal/service -run 'TestRegisterOAuthEmailAccount(RejectsExpiredInvitation|RollsBackCreatedUserWhenTokenPairGenerationFails)'`；`go test -tags integration ./internal/repository -run 'TestRedeemCodeRepoSuite/Test(ListWithFilters_StatusExpiredIncludesInvitationExpiry|Use_ExpiredInvitationRejected)'`；`go test ./internal/handler/admin/redeem_handler.go ./internal/handler/admin/idempotency_helper.go ./internal/handler/admin/admin_service_stub_test.go ./internal/handler/admin/redeem_handler_test.go -run 'TestResolveRedeemCodeExpiresAt|TestGenerate_AcceptsInvitationExpiry|TestCreateAndRedeem_(SubscriptionRequiresPlanID|SubscriptionRequiresPositivePlanID|SubscriptionValidParamsPassValidation|BalanceIgnoresSubscriptionFields|TypeDefaultsToBalance)'`；`pnpm typecheck`；`git diff --check`。
  - 备注：`go test ./internal/handler/admin -run 'TestResolveRedeemCodeExpiresAt|TestGenerate_AcceptsInvitationExpiry'` 仍被既有 `setting_handler_auth_source_defaults_test.go` 中 `NewSettingHandler` 参数数量不匹配阻塞，未进入本条用例；已用兑换码 handler 文件级测试覆盖本次新增逻辑。
✅ fix(accounts): show email and fix usage refresh for OpenAI OAuth accounts: https://github.com/Wei-Shaw/sub2api/pull/2571
  - 同步方式：按 PR 顺序套用 upstream 提交 `6db02f00` 与 `41e7ae53`，随后做 fork 适配复查。
  - 决策：保留 fork 当前账号用量缓存、懒加载和手动刷新机制；OpenAI OAuth 行内主动查询改为传 `force=true`，后端仅在主动刷新时绕过 Codex 探测缓存，避免普通列表渲染重复探测上游。
  - 决策：账号列表邮箱展示兼容 `extra.email_address`、`extra.email` 和已脱敏返回中的 `credentials.email`，不恢复任何已屏蔽的敏感凭证字段；前端刷新按钮改用既有 `Icon` 组件而非手写 SVG。
  - 测试：`go test ./internal/service -run 'TestAccountUsageService_.*OpenAI|TestShouldRefreshOpenAICodexSnapshot|TestAccountUsageService_ShouldProbeOpenAICodexSnapshot_ForceBypassesCache|TestExtractOpenAICodexProbeUpdatesAccepts429WithCodexHeaders|TestBuildCodexUsageProgressFromExtra'`；`pnpm test:run src/components/account/__tests__/AccountUsageCell.spec.ts`；`pnpm typecheck`；`git diff --check`；`git diff --cached --check`。
✅ fix(apicompat): preserve empty streaming thinking blocks: https://github.com/Wei-Shaw/sub2api/pull/2546
  - 同步方式：PR 分支 base 与当前 fork 差异很大，未整段 cherry-pick；手工移植单提交 `e9a25e7b` 中 apicompat 的最小改动。
  - 决策：为 `AnthropicContentBlock` 增加自定义 JSON 序列化，仅让 `text`/`thinking` 块保留空字符串字段，避免流式 `content_block_start` 丢失 Anthropic 协议要求的 `thinking` 键；其它 block 类型继续沿用默认 `omitempty` 行为。
  - 测试：`go test ./internal/pkg/apicompat -run 'TestStreamingReasoning|TestResponsesToAnthropic|TestAnthropicToResponses'`；`go test ./internal/pkg/apicompat`；`git diff --check`；`git diff --cached --check`。
✅ Fix wxpay pending order reconciliation: https://github.com/Wei-Shaw/sub2api/pull/2574
  - 同步方式：PR 分支包含大量已同步历史，仅手工移植单提交 `dbd80a04` 的支付补偿改动，并按 fork 当前支付状态机适配。
  - 决策：保留 fork 已有 `checkPaid(ctx, order)` 只查询不取消上游、取消/过期路径才关闭上游单据的行为；新增定时任务先补偿未过期的微信 pending 订单，再执行过期处理。
  - 决策：前端等待面板在微信订单本地仍为 `PENDING` 且有 `out_trade_no` 时节流调用 authenticated verify；Stripe 继续沿用既有主动 verify，支付宝等其它方式保持只读轮询。结果页优先 authenticated verify，失败后回退 public lookup，不放宽 public 接口能力。
  - 测试：`go test -tags unit ./internal/service -run 'Test(VerifyOrderByOutTradeNo(BackfillsTradeNoFromPaidQuery|RetriesZeroAmountPaidQueryOnce|RejectsPaidQueryWithZeroAmount|UsesOutTradeNoWhenPaymentTradeNoAlreadyExistsForAlipay|DoesNotCancelPendingUpstreamOrder)|CancelOrderStillClosesPendingUpstreamOrder|ReconcilePendingWxpayOrdersBackfillsPaidOrder|PaymentOrder(QueryReference|AllowsRegistryFallback))'`；`pnpm test:run src/components/payment/__tests__/PaymentStatusPanel.spec.ts src/views/user/__tests__/PaymentResultView.spec.ts`；`pnpm typecheck`；`git diff --check`；`git diff --cached --check`。
✅ fix: hide model scopes for non-antigravity plans: https://github.com/Wei-Shaw/sub2api/pull/2521
  - 同步方式：手工移植 PR 单提交 `26ca73a4` 的前端展示、分组提交归一化和单元测试。
  - 决策：保留 fork 现有 Antigravity 模型系列配置 UI；仅在非 Antigravity 套餐展示和分组 create/update payload 中清空隐藏的 `supported_model_scopes`，避免普通平台套餐暴露无效模型范围。
  - 测试：`pnpm test:run src/components/payment/__tests__/SubscriptionPlanCard.spec.ts src/views/admin/__tests__/groupsSupportedModelScopes.spec.ts`；`pnpm typecheck`；`git diff --check`。
✅ fix(openai): 修复 access_token 已过期且 refresh_token 缺失时 持续命中，呈现502错误的bug: https://github.com/Wei-Shaw/sub2api/pull/2514
  - 同步方式：PR 分支包含大量已同步历史和 merge，仅手工移植功能提交 `bec1e2b6` 的缺失 refresh_token 禁用逻辑。
  - 决策：保留 fork 当前 OAuth refresh API、token 缓存和 401 临时冷却机制；仅当 OAuth 账号 refresh_token 缺失且 access_token 已过期或上游 401 时，直接 `SetError` 并清理 token 缓存，避免账号冷却后反复命中导致用户持续 502。有 refresh_token 的 OpenAI/Gemini OAuth 401 仍按原路径失效缓存、写入 `expires_at` 并临时不可调度。
  - 测试：`go test -tags unit ./internal/service -run 'Test(OpenAITokenProvider_(NoRefreshTokenExpired_DisablesAccount|NoRefreshTokenExpiredAccessTokenReturnsError|Real_ExpiredWithoutRefreshToken)|RateLimitService_HandleUpstreamError_OAuth401)'`；`go test -tags unit ./internal/service -run 'Test(OpenAITokenProvider|OpenAIOAuthService_RefreshAccountToken_NoRefreshTokenUsesExistingAccessToken|OpenAITokenRefresher_NeedsRefresh|RateLimitService_HandleUpstreamError_(OAuth401|OpenAIOAuth403|NonOAuth401))'`；`git diff --check`。
✅ fix: preserve DeepSeek reasoning_content in chat compatibility paths: https://github.com/Wei-Shaw/sub2api/pull/2543
  - 同步方式：PR 分支包含大量已同步历史，仅手工移植功能提交 `1d47fd63` 与 errcheck 修正 `fe3283a1` 的 apicompat 转换和回归测试。
  - 决策：保留 fork 当前 raw Chat Completions 直转、Responses 探测和静默拒绝 failover 逻辑；仅让 Chat Completions 历史 assistant 消息里的 `reasoning_content` 在转 Responses 时以 `<thinking>...</thinking>` 文本保留，并用 raw chat 回归测试锁定 DeepSeek 请求/响应字段不会被直转路径丢弃。
  - 测试：`go test -tags unit ./internal/pkg/apicompat ./internal/service -run 'TestChatCompletionsToResponses_Assistant(ReasoningContentPreserved|ThinkingTagPreserved)|TestForwardAsRawChatCompletions_PreservesDeepSeekReasoningContent|TestForwardAsRawChatCompletions_ForcesStreamUsageUpstreamAndPassesUsageDownstream'`；`go test -tags unit ./internal/pkg/apicompat -run 'TestChatCompletionsToResponses|TestResponsesToChatCompletions|TestStreamingReasoning'`；`go test -tags unit ./internal/service -run 'TestForwardAsRawChatCompletions'`；`git diff --check`。
✅ 降低大请求体场景下的 Ops 错误日志内存放大: https://github.com/Wei-Shaw/sub2api/pull/2581
  - 同步方式：PR 分支包含大量已同步历史，仅 cherry-pick 功能提交 `2eb622f2` 与 `e009d12a`，手动解决 Ops logger/service/repository 冲突。
  - 决策：保留 fork 现有 Ops SLA 分类、上游错误事件、请求钻取和错误趋势逻辑；移除 Ops 重试接口、请求体/请求头重放字段、retry attempt 表和 UI 重试入口，避免大请求体在 gin context、队列、DB 和前端详情间被重复保留。
  - 决策：保留 fork 的 OpenAI service_tier/reasoning_effort 计费记录语义；仅在上游响应后释放已解析请求体缓存，减少大请求生命周期内的内存保留。
  - 测试：`go test ./internal/handler -run 'TestClassifyOps|Test.*Ops|Test.*Failover|Test.*ChatCompletions|Test.*Responses'`；`go test ./internal/service -run 'TestOps|Test.*Ops'`；`go test ./internal/repository -run 'TestOps|Test.*Replay'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`pnpm test:run src/views/admin/ops/utils/__tests__/errorDetailResponse.spec.ts`；`pnpm typecheck`；`git diff --check`；`git diff --cached --check`。
✅ feat(payment): 支持强制移动端统一使用二维码支付: https://github.com/Wei-Shaw/sub2api/pull/2578
  - 同步方式：PR 分支包含大量已同步历史，仅 cherry-pick 单提交 `e4c7927e`，手动解决设置 handler、支付流程测试和用户支付页冲突。
  - 决策：保留 fork 当前 Stripe billing info、微信 OAuth/JSAPI 恢复、Airwallex 路由和移动端失败后二维码兜底逻辑；新增支付宝移动端强制二维码开关，开启后支付宝下单传 `is_mobile=false` 并让前端跳过移动端 pay_url 优先分支。
  - 决策：未引入 PR 分支中夹带但本 fork 当前未落地的 Channel Monitor、Available Channels、Affiliate 字段。
  - 测试：`go test ./internal/service -run 'Test.*Payment|Test.*PaymentConfig|Test.*Config'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`pnpm test:run src/components/payment/__tests__/paymentFlow.spec.ts`；`pnpm test:run src/components/payment/__tests__/PaymentStatusPanel.spec.ts src/views/user/__tests__/PaymentResultView.spec.ts`；`pnpm typecheck`；`git diff --check`；`git diff --cached --check`。
✅ fix(apicompat): Responses 转换为推理模型时剥离不支持的 temperature 参数: https://github.com/Wei-Shaw/sub2api/pull/2580
  - 同步方式：PR 分支包含大量已同步历史，仅 cherry-pick 功能提交 `276b5c77`，并将新增注释改为中文。
  - 决策：在 Anthropic->Responses 与 Chat Completions->Responses 两条转换路径里，对 `gpt-5*` 推理模型剥离 `temperature/top_p`，避免 OpenAI Responses API 返回 unsupported parameter；非推理模型继续保留采样参数。
  - 测试：`go test ./internal/pkg/apicompat -run 'Test(AnthropicToResponses_TemperatureStripped|ChatCompletionsToResponses_Temperature|ChatCompletionsToResponses|AnthropicToResponses)'`；`git diff --check`；`git diff --cached --check`。
✅ feat(channels): 模型定价支持一键同步最新模型: https://github.com/Wei-Shaw/sub2api/pull/2582
  - 同步方式：PR 分支包含大量已同步历史，仅 cherry-pick 功能提交 `92ad68a3`，手动解决 Wire 注入和 channel handler 测试冲突。
  - 决策：保留 fork 当前渠道平台、账号统计定价和模型映射页面结构；只新增按平台从 LiteLLM 定价目录拉取最新模型名的 admin API 与前端按钮，新增模型以未填价格的 token 定价条目加入，便于管理员人工确认价格。
  - 决策：未引入 PR 分支中夹带的 Channel Monitor handler/template 依赖，避免把未同步的监控模块接入当前 fork。
  - 测试：`go test ./internal/service -run 'TestListModelNamesByProvider|TestParsePricingData|Test.*Pricing'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`go test -tags unit ./internal/handler/admin/channel_handler.go ./internal/handler/admin/channel_handler_test.go -run 'TestSyncPricingModels|TestPricingRequestToService|TestChannelToResponse'`；`pnpm test:run src/components/admin/channel/__tests__/types.spec.ts`；`pnpm typecheck`；`git diff --check`；`git diff --cached --check`。
  - 备注：`go test -tags unit ./internal/handler/admin -run 'TestSyncPricingModels|TestPricingRequestToService|TestChannelToResponse'` 被既有 `setting_handler_auth_source_defaults_test.go` 中 `NewSettingHandler` 参数数量不匹配阻塞，未进入本条用例；已用 channel handler 文件级测试覆盖本次新增逻辑。
✅ 优化渠道监控 OpenAI 检测协议与内置模板: https://github.com/Wei-Shaw/sub2api/pull/2585
  - 同步方式：检查 PR 功能提交 `9055612d`..`f8488515`；未应用代码。
  - 决策：当前 fork 不包含 upstream 的 Channel Monitor 后端、仓储、迁移、前端 `components/admin/monitor` 与相关 API 页面，PR 2585 修改目标链路不存在；为避免把未同步的渠道监控模块整套引入，本条标记为不适用。
  - 测试：`rg --files backend/internal/service backend/internal/handler/admin backend/internal/repository backend/ent/schema backend/migrations frontend/src | rg 'channel_monitor|ChannelMonitor|monitor|Monitor'`；`rg -n "channelMonitor|channel-monitor|ChannelMonitor|MonitorForm|MonitorTemplate|channel_monitor" frontend/src backend/internal/server/routes backend/internal/handler/admin backend/internal/service backend/internal/repository backend/ent/schema backend/migrations`。
✅ feat: add gemini-3.5-flash model support across backend and frontend: https://github.com/TokenFlux/TokenRouter/commit/3d22dd34d3de9076804858f979f60fccdf9f2de1
  - 同步方式：fetch direct commit `3d22dd34` 后 cherry-pick，无冲突。
  - 决策：仅补充 Gemini/Gemini CLI 默认模型、前端白名单预设、测试弹窗优先模型顺序、账号状态缩写和使用说明里的模型能力参数；不调整 Gemini 路由、计费或模型映射语义。
  - 测试：`go test ./internal/pkg/gemini ./internal/pkg/geminicli`；`NODE_OPTIONS="--localstorage-file=/tmp/tokenrouter-account-tests-localstorage" pnpm test:run src/components/account/__tests__/AccountTestModal.spec.ts src/components/account/__tests__/AccountStatusIndicator.spec.ts src/composables/__tests__/useModelWhitelist.spec.ts`；`pnpm typecheck`；`git diff --check`；`git diff --cached --check`。
  - 备注：不带 `--localstorage-file` 运行 AccountTestModal 测试会因 Node 26 无全局 `localStorage` 失败，带参数重跑通过。
✅ feat(risk-control): 内容审计新增关键词拦截: https://github.com/TokenFlux/TokenRouter/commit/91da815993732e6536be8c702168822e482cd850
  - 同步方式：fetch direct commit `91da8159` 后 cherry-pick，手动解决内容审计服务和风险控制页面冲突。
  - 决策：保留 fork 既有 Cyber warning / auto-ban 配置、记录 tab 和告警统计语义；新增关键词列表与 `keyword_only` / `keyword_and_api` / `api_only` 策略作为独立的前置拦截控制。
  - 决策：关键词命中仅在 `pre_block` 模式下短路拦截并记录 `keyword_block` 日志；`keyword_only` 未命中直接放行以节省上游审计 API 调用，`api_only` 保持只走上游 API。
  - 测试：`go test ./internal/service -run 'Test(NormalizeBlockedKeywords|MatchBlockedKeyword|ContentModerationCheck_|NormalizeKeywordBlockingMode|ContentModerationConfigNormalize|ContentModerationUpdateConfig)'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`pnpm typecheck`；`git diff --check`；`git diff --cached --check`。
  - 备注：尝试运行 `pnpm test:run src/views/admin/__tests__/RiskControlView.spec.ts`，当前仓库没有该测试文件，未执行到用例。
✅ feat(openai-gateway): Codex OAuth 账号浏览器 UA 自动改写规避 Cloudflare: https://github.com/TokenFlux/TokenRouter/commit/878ad3b56952c964cd4507f2b4d190c330130c5e
  - 同步方式：fetch direct commit `878ad3b5` 后 cherry-pick，手动解决 `setting_service.go` 中默认订阅 reader 与新增 UA 缓存类型的冲突。
  - 决策：保留 fork 当前 `DefaultSubscriptionPlanReader` 默认订阅套餐校验，不回退到 upstream 旧的默认分组 reader。
  - 决策：保留 fork 既有 OAuth 非 Codex UA 的 `codex_cli_rs` 兜底；浏览器 UA 优先使用后台可配置的 OpenAI Codex UA，避免旧兜底先改写导致新配置不生效。
  - 测试：`go test ./internal/pkg/openai -run 'TestIs(BrowserUserAgent|Codex)'`；`go test -tags unit ./internal/service -run 'Test(OpenAIGatewayService_OAuthPassthrough_(NonCodexUAFallbackToCodexUA|BrowserUAUsesConfiguredCodexUA)|SettingService_(UpdateSettings_OpenAICodexUserAgent|GetOpenAICodexUserAgent_Precedence))'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`pnpm test:run src/views/admin/__tests__/SettingsView.spec.ts`；`pnpm typecheck`；`git diff --check`；`git diff --cached --check`。
✅ fix(openai-images): 修复 /v1/images/generations 的 n 参数不生效: https://github.com/Wei-Shaw/sub2api/pull/2593
  - 同步方式：PR 分支包含已处理历史，仅 cherry-pick 功能提交 `2c14efea`。
  - 决策：保留 fork 当前 OpenAI Images OAuth 走 Responses image tool 的路由、尺寸归一化、流式断连后继续读取上游用量和图片计费记录逻辑；仅对非 `dall-e-3` 且 `n > 1` 的生成请求透传 `tools[0].n`，并返回上游产生的全部图片。
  - 测试：`go test ./internal/service -run 'Test(OpenAIGatewayServiceForwardImages|BuildOpenAIImagesResponsesRequest|CollectOpenAIImages|ExtractOpenAIImages)'`；`git diff --check`。
✅ test(api-contract): fix admin/settings expected map missing openai_codex_user_agent: https://github.com/TokenFlux/TokenRouter/commit/825834b5cb818b7161969b0d147a476ec3ff6df9
  - 同步方式：检查 direct commit `825834b5`；未新增代码提交。
  - 决策：该提交的两处 `openai_codex_user_agent` API contract 期望已在本 fork 同步 `878ad3b5` 时一并补入 `backend/internal/server/api_contract_test.go`，当前无需重复 cherry-pick。
  - 测试：`rg -n "openai_codex_user_agent" backend/internal/server/api_contract_test.go`。
✅ test(group): 补充分组列表账号统计回归测试: https://github.com/Wei-Shaw/sub2api/pull/2595
  - 同步方式：PR 分支包含已处理历史，仅 cherry-pick 功能提交 `5465003d`。
  - 决策：保留 fork 当前 `service.Group` 结构，不引入 upstream 测试里引用但本 fork 不存在的 `SubscriptionType` 字段；仅同步分组账号总数、可用账号数和限流账号数统计回归测试。
  - 测试：`go test -tags=integration ./internal/repository -run 'TestGroupRepoSuite/TestListWithFilters_(ActiveAccountCount_LessThanTotal|RateLimitedAccountCount)|TestGroupRepoSuite/TestListWithAccountCountSort_AttachesActiveCount'`；`git diff --check`。
✅ feat: 添加邮件模板编辑器与通知邮件模板化: https://github.com/Wei-Shaw/sub2api/pull/2599
  - 同步方式：按 PR 2599 功能提交顺序套用 `ee1bb847`..`e1b53fde`，手动解决设置 DTO、公开退订入口、支付、Wire、前端设置页和内容审计冲突。
  - 决策：保留 fork 当前支付状态机、微信 pending 补偿、Stripe billing info、邀请奖励发放和当前订阅按 `PlanID` 绑定的模型；仅在订单完成后异步发送模板化成功邮件，不引入 upstream 分支中当前 fork 不存在的 `AffiliateService`、`SubscriptionGroupID`/`SubscriptionDays` 字段。
  - 决策：保留 fork 既有 Cyber warning / keyword blocking 风控语义；模板邮件仅作为通知投递层接入，投递类错误不再 fallback 发送旧模板以避免重复邮件，模板/配置错误才 fallback。
  - 决策：后台邮件模板编辑器接入当前 Email 设置页，保留现有 SMTP、余额提醒、账号限额提醒和支付配置 UI；新增注释按仓库要求改为中文。
  - 测试：`go test ./cmd/server ./internal/handler/admin ./internal/handler ./internal/server ./internal/service -run 'Test(NotificationEmail|ContentModeration|EmailQueue|Ops|Payment|SubscriptionExpiry|APIContracts|Settings|SendVerify|PasswordReset|Totp|Auth)'`；`go test ./internal/service -run 'Test(OpenAITokenProvider|RateLimitService|BalanceNotify|AccountQuota|Subscription|Payment)'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`go test ./internal/service -run 'TestNotificationEmail|TestEmailQueue|TestOps.*Email|TestPayment.*Notification|TestSubscriptionExpiry'`；`pnpm typecheck`；`pnpm test:run src/views/admin/__tests__/SettingsView.spec.ts`；`git diff --check`；`git diff --cached --check`。
  - 备注：`SettingsView.spec.ts` 通过时仍输出既有 `router-link` 未 stub 警告；不影响测试结果。
✅ fix(openai): /v1/responses 入口尊重 force_chat_completions 设置: https://github.com/Wei-Shaw/sub2api/pull/2606
  - 同步方式：PR 分支包含大量已同步历史，仅 cherry-pick 单提交 `cae93ae1`，手动解决 `openai_gateway_service.go` import 冲突并按 fork 模块路径适配新增文件。
  - 决策：保留 fork 现有 `/v1/responses` WS、图片生成、Codex OAuth、service_tier/fast policy、静默拒绝 failover 和 raw Chat Completions header 白名单语义；仅在 API Key 账号 `openai_responses_mode=force_chat_completions` 时把 `/v1/responses` 入站请求桥接为上游 `/v1/chat/completions`，并将 Chat Completions 响应转换回 Responses 格式。
  - 决策：新增注释按仓库要求改为中文；未引入 upstream 分支中与本条无关的历史改动。
  - 测试：`go test -tags unit ./internal/pkg/apicompat ./internal/service -run 'Test(ResponsesToChatCompletions|ChatCompletionsToResponses|ForwardResponses_|ForwardAsRawChatCompletions)'`；`go test ./internal/pkg/apicompat -run 'Test(ResponsesToChatCompletions|ChatCompletionsToResponses|StreamingReasoning|ResponsesUsage)'`；`go test ./internal/service -run 'Test(OpenAIGatewayService|ForwardAsRawChatCompletions|OpenAIServiceTier|EvaluateOpenAIFastPolicy)'`；`go test -tags unit ./internal/service -run 'Test(ForwardResponses_|ForwardAsRawChatCompletions|BuildOpenAIChatCompletionsURL|OpenAIServiceTier|EvaluateOpenAIFastPolicy)'`；`go test ./cmd/server ./internal/handler ./internal/server ./internal/service -run 'Test.*(OpenAI|Responses|ChatCompletions|APIContracts|Failover)'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`golangci-lint run --timeout=30m ./...`；`git diff --check`；`git diff --cached --check`。
✅ fix email reminder dedup keys: https://github.com/TokenFlux/TokenRouter/commit/dd4d482a7011bd7dcc2dbcc3eadd685a589e93e8
  - 同步方式：fetch direct commit `dd4d482a` 后 cherry-pick，无冲突。
  - 决策：邮件投递去重与退订 key 改为短 hash v2，保留读取 legacy key 的兼容逻辑，避免已写入的旧去重/退订状态失效；投递去重记录写入失败时返回错误，避免邮件已发但去重状态丢失后重复发送。
  - 测试：`go test ./internal/service -run 'TestNotificationEmail'`；`git diff --cached --check`。
✅ fix: 修正分组账号可用计数口径: https://github.com/TokenFlux/TokenRouter/commit/df2b02e61c84d87bf9725135ec2b4830c7ce3092
  - 同步方式：fetch direct commit `df2b02e` 后 cherry-pick，手动解决 group repository 与集成测试冲突。
  - 决策：保留 fork 当前 `sqlExecutorFromContext(ctx)`，确保事务内测试和仓储调用继续使用同一执行器；合入 upstream 的可用账号统计口径，使 `ActiveAccountCount` 与真实可调度账号一致，并让 `RateLimitedAccountCount` 覆盖 rate_limit/overload/temp_unschedulable 临时退出调度窗口。
  - 测试：`go test -tags=integration ./internal/repository -run 'TestGroupRepoSuite/TestListWithFilters_(ActiveAccountCount_LessThanTotal|RateLimitedAccountCount)|TestGroupRepoSuite/TestListWithAccountCountSort_AttachesActiveCount'`；`git diff --cached --check`。
✅ test(repository): 补充 AES Encryptor 单元测试: https://github.com/Wei-Shaw/sub2api/pull/2611
  - 同步方式：PR 分支包含大量已同步历史，仅 cherry-pick 功能提交 `cbdfedab`。
  - 决策：仅同步 AES Encryptor 单元测试，模块路径改为本 fork 的 `github.com/TokenFlux/TokenRouter`；不触碰加密实现。
  - 测试：`go test -tags unit ./internal/repository -run 'TestAES|TestNewAESEncryptor|TestAESEncryptor'`；`git diff --cached --check`。
✅ fix(auth): 停用/删除分组后阻断已发放 API Key 的请求: https://github.com/Wei-Shaw/sub2api/pull/2612
  - 同步方式：PR 分支包含已处理历史，仅 cherry-pick 功能提交 `22ff1acd`，手动解决 `group_repo.go` 冲突。
  - 决策：保留 fork 当前分组软删除、事务执行器和 `sqlExecutorFromContext` 语义；删除分组时不再清空 API Key 的 `group_id`，让认证层能识别绑定到已删除分组的已发放 Key 并拒绝请求。
  - 决策：OpenAI/Gemini 两套 API Key 中间件都在基础鉴权阶段检查分组可用性；API Key 认证缓存版本提升到 v10，避免旧快照缺少分组状态导致继续放行。
  - 测试：`go test -tags unit ./internal/server/middleware -run 'TestAPIKeyAuth|TestAPIKeyAuthGoogle'`；`go test -tags unit ./internal/service -run 'TestAdminService_DeleteGroup'`；`go test -tags integration ./internal/repository -run 'TestGroupRepository_DeleteCascade|TestUserRepository_RemoveGroupFromAllowedGroups'`；`git diff --cached --check`。
✅ feat(usage): 用户 API Key 用量页支持按日明细: https://github.com/Wei-Shaw/sub2api/pull/2613
  - 同步方式：PR 分支包含大量已同步历史，仅 cherry-pick 功能提交 `90b2b2a7`，手动解决 gateway usage、前端 usage API 和 KeyUsageView 测试冲突。
  - 决策：保留 fork 当前 `/v1/usage` 的余额单位配置、订阅上下文读取和 15/30 分钟精确时间范围；在此基础上追加 `days`、`timezone` 与 `daily_usage` 响应字段。
  - 决策：前端保留 fork 的余额格式化和既有无限订阅额度测试；仅同步 KeyUsage 页的按日明细表和 7/30/90 天切换。移除 PR 中夹带但不属于本条功能的兑换码批量修改 i18n 文案。
  - 测试：`go test ./internal/handler -run 'TestGetMyAPIKeyDailyUsage|TestGatewayHandler_Usage|Test.*Usage'`；`go test ./internal/service -run 'Test.*Usage'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`NODE_OPTIONS="--localstorage-file=/tmp/tokenrouter-keyusage-vitest-localstorage" pnpm test:run src/views/__tests__/KeyUsageView.spec.ts`；`pnpm typecheck`；`git diff --cached --check`。
✅ ci: restrict cla workflow to upstream repo: https://github.com/TokenFlux/TokenRouter/commit/5c074e6fe8a03eb653ea1fe52887d87cdcf54a87
  - 同步方式：fetch direct commit `5c074e6f` 后 cherry-pick，遇到 `.github/workflows/cla.yml` modify/delete 冲突。
  - 决策：当前 fork 的 HEAD 已删除 CLA workflow；删除比 upstream 的 `github.repository == 'Wei-Shaw/sub2api'` 条件更严格，能确保本 fork 不运行 CLA Assistant。因此保留删除并用空提交记录本条已同步。
  - 测试：`git cat-file -e HEAD:.github/workflows/cla.yml` 确认当前 HEAD 不包含该 workflow；`git diff --cached --check`。

✅ fix: 命中"thinking block must contain thinking"上游错误时触发重试: https://github.com/Wei-Shaw/sub2api/pull/2636
  - 同步方式：cherry-pick PR 单提交 `8211aa70`，无冲突。
  - 决策：保留 fork 现有 thinking signature rectifier、Ops 上游错误记录和重试预算逻辑；仅把 `thinking block must contain thinking` 上游错误纳入既有 thinking block 过滤重试路径，新增注释保持中文。
  - 测试：`go test -tags unit ./internal/service -run 'TestFilterThinkingBlocksForRetry_(DropsThinkingBlockWithEmptyContent|RemovesRedactedThinkingAndKeepsValidContent|EmptyContentGetsPlaceholder|StripsEmptyTextBlocks)'`；`git diff --check HEAD~1..HEAD`。
✅ feat(bedrock): add Claude Code compatibility transformations: https://github.com/Wei-Shaw/sub2api/pull/2642
  - 同步方式：cherry-pick PR 单提交 `4fd21994`，手动解决 `ChannelsView.vue` 中 Bedrock CC 开关插入位置与当前 fork 渠道表单的冲突，并将新增注释改为中文。
  - 决策：保留 fork 当前 Bedrock 模型映射、beta policy、cache_control 清理和 Codex 图片生成桥接配置；新增渠道级 `bedrock_cc_compat` 开关，在转发前对 Anthropic 平台请求做 thinking 类型/budget 和 tool_use ID 兼容清理，Bedrock 原生转发继续走现有请求体准备与签名流程。
  - 测试：`go test ./internal/service -run 'Test(IsBedrockOpus47OrNewer|SanitizeBedrockThinking|SanitizeBedrockToolUseIDs|PrepareBedrockRequestBodyWithTokens_CCCompat|ResolveBedrockBetaTokens|PrepareBedrockRequestBody|BuildBedrockURL|AdjustBedrockModelRegionPrefix)'`；`pnpm typecheck`；`git diff --check HEAD~1..HEAD`；`git diff --cached --check`。
  - 备注：尝试运行 `pnpm test:run src/views/admin/__tests__/ChannelsView.spec.ts`，当前仓库没有该测试文件，未执行到用例。
✅ fix(ops): 排除本地客户端限制错误的 SLA 计数: https://github.com/Wei-Shaw/sub2api/pull/2643
  - 同步方式：cherry-pick PR 单提交 `69305a60`，无冲突。
  - 决策：保留 fork 已有 API Key IP 限制 `OpsClientBusinessLimited` 标记和上游错误上下文判定；新增 API Key 过期/停用/用户不存在、查询参数 key 弃用、订阅/余额/本地限额等本地客户端限制的 SLA 排除规则，并确保带上游上下文的同类文本仍归 provider/upstream 计入 SLA。
  - 测试：`go test ./internal/handler -run 'TestClassifyOps(AuthClientErrorsExcludedFromSLA|LocalBusinessLimitErrorsExcludedFromSLA|IPRestrictionAccessDeniedExcludedFromSLA|OtherErrorsStillCountForSLA|UnsupportedModelExcludedFromSLA|UnmarkedNoAvailableTextStillCountsForSLA|UpstreamAuthTextStillCountsForSLA|UpstreamNoAvailableTextStillCountsForSLA)'`；`git diff --check HEAD~1..HEAD`。
✅ fix(channel-monitor): 兼容 Responses reasoning 输出: https://github.com/Wei-Shaw/sub2api/pull/2641
  - 同步方式：检查 PR 单提交 `d3d5843b`；未应用代码。
  - 决策：当前 fork 不包含 upstream 的 `channel_monitor_checker.go`、Channel Monitor service/repository/API 或前端监控页面，PR 修改目标链路不存在；延续此前 PR 2585 的处理，为避免引入整套未接线 Channel Monitor 模块，本条标记为不适用。
  - 测试：`rg --files backend/internal/service backend/internal/handler backend/internal/repository backend/ent/schema backend/migrations frontend/src | rg 'channel_monitor|ChannelMonitor|monitor|Monitor'`；`rg -n 'channelMonitor|channel-monitor|ChannelMonitor|MonitorForm|MonitorTemplate|channel_monitor|monitor templates|responses reasoning' frontend/src backend/internal/server/routes backend/internal/handler backend/internal/service backend/internal/repository backend/ent/schema backend/migrations`。
✅ PR：为 API Key IP 白/黑名单增加可配置的反代真实 IP 判断: https://github.com/Wei-Shaw/sub2api/pull/2645
  - 同步方式：按功能提交顺序同步 `08c8c67d` 与 `1d2445ff`，手动解决后台设置 handler、访问日志 middleware 和 setting service 测试冲突。
  - 决策：保留 fork 现有 `GetTrustedClientIP` 默认安全口径、API Key IP 限制 Ops business-limited 标记、OpenAI Codex UA 设置测试和完整 Settings 字段；新增后台/API/前端开关 `api_key_acl_trust_forwarded_ip`，默认关闭，开启后 API Key 白/黑名单改用 `CF-Connecting-IP` / `X-Real-IP` / `X-Forwarded-For` 的真实客户端 IP；访问日志继续与使用记录一致使用 `ip.GetClientIP`。
  - 测试：`go test -tags unit ./internal/server/middleware -run 'TestAPIKeyAuthIPRestriction|TestLogger_AccessLog'`；`go test -tags unit ./internal/service -run 'TestSettingService_(UpdateSettings_APIKeyACLTrustForwardedIPRefreshesConfig|ParseSettings_APIKeyACLTrustForwardedIPFallsBackToConfigWhenMissing|UpdateSettings_OpenAICodexUserAgent)'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`pnpm typecheck`；`git diff --check HEAD~2..HEAD`。
✅ fix(auth): user_provider_default_grants 加入 github/google/dingtalk: https://github.com/Wei-Shaw/sub2api/pull/2648
  - 同步方式：手工移植 upstream 单提交 `4bfb707f` 的迁移缺口，并补充本 fork 的迁移回归测试。
  - 决策：当前 fork 已有 GitHub/Google/DingTalk 默认授权应用层配置；本条仅补 `user_provider_default_grants` 的 provider_type check 约束，使其与 134/136 已更新的 OAuth provider 集合一致。
  - 测试：`go test ./migrations -run 'TestMigration(134AllowsEmailOAuthProviderTypes|140ExtendsUserProviderDefaultGrantsProviderTypes)'`；`git diff --check`。
✅ feat(redeem): 兑换码支持批量修改: https://github.com/Wei-Shaw/sub2api/pull/2615
  - 同步方式：手工移植 upstream 单提交 `3263ca63` 的批量修改能力，避开 PR 分支中已同步历史和本 fork 不存在的订阅分组模型。
  - 决策：保留 fork 当前兑换码的 `plan_id`、`max_uses`、`used_count`、自然过期和 `expires_at` 语义；批量修改仅开放状态、过期时间和备注，显式拒绝批量修改 type/value/max_uses/plan_id，避免绕过单条编辑里的权益一致性校验。
  - 决策：已部分或全部使用的兑换码不能批量修改状态或过期时间，备注-only 修改仍允许；`disabled` 状态按不可用状态处理，并从有效可用状态筛选中排除。
  - 测试：`go test -tags unit ./internal/service -run 'TestRedeem(CodeExpirySemantics|Service_BatchUpdate)'`；`go test ./internal/handler/admin/redeem_handler.go ./internal/handler/admin/idempotency_helper.go ./internal/handler/admin/admin_service_stub_test.go ./internal/handler/admin/redeem_handler_test.go -run 'TestRedeemBatchUpdate|TestResolveRedeemCodeExpiresAt|TestGenerate_AcceptsInvitationExpiry'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`go test -tags integration ./internal/repository -run 'TestRedeemCodeRepoSuite/Test(ListWithFilters_Status|Use|Update)'`；`NODE_OPTIONS="--localstorage-file=/tmp/tokenrouter-redeem-batch-localstorage" pnpm test:run src/views/admin/__tests__/RedeemView.batchUpdate.spec.ts`；`pnpm typecheck`；`git diff --check`。
✅ fix(i18n):: https://github.com/TokenFlux/TokenRouter/commit/9673a22f26e1f81d0abda3e6d8ca8cd7d8fb74d4
  - 同步方式：检查 direct commit `9673a22f`；无需额外代码变更。
  - 决策：该提交删除 PR 2613/2615 叠加后重复的兑换码批量修改 i18n key。本 fork 在手工移植 PR 2615 时已只保留一组 `admin.redeem` 批量修改文案，并按当前兑换码模型移除 upstream 的 `group` 批量字段，因此无需 cherry-pick。
  - 测试：`rg -n "batchUpdate|selectedCount|clearSelection|batchFields|batchNotesPlaceholder" frontend/src/i18n/locales/en.ts frontend/src/i18n/locales/zh.ts`；`pnpm typecheck`。
✅ fix: mark reused refresh tokens non-retryable and unschedule errored accounts: https://github.com/Wei-Shaw/sub2api/pull/2374
  - 同步方式：按 PR 功能提交 `49b415e3`、`202aab8e` 手工移植；`refresh_token_reused` 不可重试判断已在本 fork 现有实现中包含，本条补充对应回归测试。
  - 决策：保留 fork 当前 OAuth 刷新失败后的隐私设置尝试、临时不可调度、Redis 临时不可调度缓存清理、scheduler cache/outbox 快照刷新逻辑；在 `Update` 和 `SetError` 写入 error 状态时同步关闭 `schedulable`，避免错误账号被重新调度。
  - 测试：`go test -tags unit ./internal/service -run 'Test(IsNonRetryableRefreshError|TokenRefreshService_RefreshWithRetry_NonRetryableErrorAllPlatforms)'`；`go test -tags integration ./internal/repository -run 'TestAccountRepoSuite/Test(SetError|UpdateErrorStatusUnschedulesAccount|ListSchedulable)'`；`git diff --check`。
✅ fix: clear scheduler cache when deleting accounts: https://github.com/Wei-Shaw/sub2api/pull/2375
  - 同步方式：手工移植 PR 单提交 `60f6602b`，接入本 fork 现有 `schedulerCache` helper 风格。
  - 决策：删除账号事务提交后立即调用 `DeleteAccount` 清理单账号调度缓存，避免粘性会话或延迟 outbox worker 继续读到旧快照；保留既有 scheduler outbox 事件，用于分组相关异步刷新。
  - 测试：`go test -tags integration ./internal/repository -run 'TestAccountRepoSuite/TestDelete'`；`git diff --check`。
✅ fix(openai): surface image moderation errors: https://github.com/Wei-Shaw/sub2api/pull/2399
  - 同步方式：手工移植 PR 单提交 `888cd809`，适配本 fork 已有 OpenAI Images OAuth Responses 路由、n 参数透传、用量计费、failover 和 Ops/Cyber 记录逻辑。
  - 决策：解析 upstream Responses SSE 中的 `error` / `response.failed`，将 `moderation_blocked` / `image_generation_user_error` 透出为客户端 400 或流式 error event；这类用户侧图片风控错误不再标记账号调度失败、不触发账号切换或 generic fallback。
  - 决策：保留 fork 当前失败路径的 Ops upstream error 上下文和图片转发结构；补齐 `auth_oauth_pending_flow_test` 的 redeem repo fake `BatchUpdate` 方法，使 handler 包在 PR 2615 接口扩展后能完整编译。
  - 测试：`go test ./internal/service -run 'Test(OpenAIGatewayServiceForwardImages|CollectOpenAIImages|BuildOpenAIImages|ExtractOpenAIImages)'`；`go test ./internal/handler -run 'TestOpenAI.*Images|TestImage'`；`git diff --check`。
✅ feat: add subscription expiry email toggle: https://github.com/TokenFlux/TokenRouter/commit/a613a587bab22c36785347e78b35b49c167079d3
  - 同步方式：cherry-pick direct commit `a613a587`，手动解决设置 DTO/API contract、订阅到期服务和测试冲突。
  - 决策：保留 fork 当前订阅到期提醒的发送超时隔离、分页扫描独立 context、`subscriptionExpiryNotificationSender` 测试抽象和 `subscriptionReminderPlanName` 兜底逻辑；新增 `subscription_expiry_notify_enabled` 全局开关，设置读取失败时 fail closed，缺失设置时保持历史默认开启。
  - 决策：前端 Email 设置页新增订阅到期提醒开关，并保留邮件模板编辑器事件说明增强；迁移 141 默认写入开启值。
  - 测试：`go test ./internal/service -run 'TestSubscriptionExpiry|TestSettingService|TestParseSettings|TestUpdateSettings'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`go test ./migrations -run 'TestMigration|Test.*141|Test.*Subscription'`；`go test ./cmd/server ./internal/service ./internal/server -run 'Test(SubscriptionExpiry|Setting|APIContracts|Settings|EmailTemplate|Wire)'`；`pnpm typecheck`；`NODE_OPTIONS="--localstorage-file=/tmp/tokenrouter-settings-vitest-localstorage" pnpm test:run src/views/admin/__tests__/SettingsView.spec.ts`；`git diff --check`；`git diff --cached --check`。
✅ feat(oidc): 上游邮箱已验证时跳过 choice 页直接登录注册: https://github.com/Wei-Shaw/sub2api/pull/2655
  - 同步方式：按 PR 内提交顺序 cherry-pick `39fe7aa0` 与 `55554adc`，并将新增注释改为中文。
  - 决策：保留 fork 现有 OIDC pending choice / 强制邮箱确认 / 邀请码注册流程；仅当上游 compat email 已验证、本地无同邮箱账号、未开启强制邮箱确认且未开启邀请码时，复用已验证邮箱 OAuth 登录注册路径直接完成 OIDC 登录。
  - 决策：fast path 创建失败、后端模式限制或配置不满足时统一回落到原 choice pending 流程；日志只记录 `infraerrors.Reason(err)`，避免暴露邮箱等敏感信息。
  - 测试：`go test ./internal/handler -run 'Test.*OIDC|Test.*OAuth.*Pending|TestTryOIDCVerifiedEmailFastPath'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`git diff --check`；`git diff --cached --check`。
✅ fix(oidc): harden verified-email fast path: https://github.com/TokenFlux/TokenRouter/commit/aae20ef4370516158e681d03ecd98071a45cd267
  - 同步方式：cherry-pick direct commit `aae20ef4`，手动解决 `auth_oidc_oauth.go` 中中文注释上下文冲突。
  - 决策：保留上一条 fast path 的触发边界，并在创建用户前先做 backend-mode 新用户登录拦截；被拦截时直接重定向错误并清理 pending cookies，避免先创建用户再发现后端模式禁止。
  - 决策：fast path 写入 identity metadata 时将真实已验证邮箱规范化为 `email`，把 OIDC 合成邮箱另存为 `synthetic_email`，避免身份元数据继续暴露/混用内部合成邮箱。
  - 测试：`go test ./internal/handler -run 'TestOIDCOAuthCallbackVerifiedEmailFastPath|TestTryOIDCVerifiedEmailFastPath|Test.*OIDC|Test.*OAuth.*Pending'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`git diff --check`；`git diff --cached --check`。
✅ chore: update sponsors: https://github.com/TokenFlux/TokenRouter/commit/16793d3af03324498a506cea0fbc1353c4ee5cfe
  - 同步方式：尝试 cherry-pick direct commit `16793d3a` 后确认不适用，未保留文件变更。
  - 决策：当前 fork 仅保留中文 `README.md`，且已删除 upstream 的 `README_CN.md` / `README_JA.md` 与 README 赞助商表；该提交只向 upstream 多语言 README 赞助商表追加 RunAPI 和 logo 资产，目标展示结构在本 fork 不存在。为避免复活已删除 README 或新增无人引用资产，本条标记为不适用。
  - 测试：`rg -n "<<<<<<<|=======|>>>>>>>|RunAPI|Sponsors|赞助商" README.md README_CN.md README_JA.md assets/partners/logos/runapi.png 2>/dev/null || true`；`git status --short --branch`。
✅ fix(deps): 升级 js-cookie 解决 GHSA-qjx8-664m-686j 阻塞 frontend-security CI: https://github.com/Wei-Shaw/sub2api/pull/2687
  - 同步方式：fetch PR 2687 后 cherry-pick 功能提交 `ffd53343`。
  - 决策：保留 fork 当前前端依赖集合，仅新增 `pnpm.overrides` 将传递依赖 `js-cookie` 固定到 `3.0.7`，并同步 lockfile 中 `ahooks` / `js-beautify` 的解析版本。
  - 决策：本地 pnpm 11.0.5 不接受 upstream lockfile 顶层 `overrides` 配置；按当前工具链重新规范化锁文件，保留 `package.json` 的 override 和 `js-cookie@3.0.7` 锁定，不提交 pnpm 11 生成的 `pnpm-workspace.yaml` build 审批占位文件。
  - 测试：`CI=true pnpm install --frozen-lockfile`；`pnpm typecheck`；`rg -n "js-cookie@3\\.0\\.5|js-cookie: 3\\.0\\.5|js-cookie@3\\.0\\.7|js-cookie: 3\\.0\\.7|\"js-cookie\"" frontend/package.json frontend/pnpm-lock.yaml`；`git diff --check`；`git diff --cached --check`。
✅ fix(frontend): 修正 Cache Hit Rate 计算分母，包含全部 prompt tokens: https://github.com/Wei-Shaw/sub2api/pull/2682
  - 同步方式：fetch PR 2682 后 cherry-pick 功能提交 `6ba20acd`，手动解决 `TokenUsageTrend.vue` 中 fork 已有公式与 upstream 同步公式的上下文冲突。
  - 决策：当前 fork 的图表数据集名为 `Cached Input %`，且组件内已用 `input_tokens + cache_creation_tokens + cache_read_tokens` 作为分母；保留该既有 UI 文案和中文注释，本条只补充 upstream 的回归测试并将测试断言适配 fork label。
  - 测试：`pnpm test:run src/components/charts/__tests__/TokenUsageTrend.spec.ts`；`pnpm typecheck`；`git diff --check`；`git diff --cached --check`。
✅ fix(risk-control): Agent 工具循环中同一用户消息重复审计去重: https://github.com/Wei-Shaw/sub2api/pull/2681
  - 同步方式：fetch PR 2681 后 cherry-pick 功能提交 `199a5bcc`，无冲突。
  - 决策：保留 fork 现有关键词拦截、warning/auto-ban 和图片抽样审计语义；仅将 Anthropic/OpenAI Chat/OpenAI Responses/Gemini 的文本提取从“回溯最后一条用户消息”改为“仅数组末尾本身是用户输入时才审计”，避免 Agent 工具循环中的 assistant/tool/function_call_output 结尾重复审计同一用户输入。
  - 测试：`go test ./internal/service -run 'TestExtractContentModerationInput|TestContentModerationInput|TestContentModerationCheck_|TestNormalizeBlockedKeywords|TestMatchBlockedKeyword|TestNormalizeKeywordBlockingMode|TestContentModerationConfigNormalize'`；`go test ./internal/service -run 'TestContentModeration|TestExtractContentModerationInput|Test.*Moderation.*'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`git diff --check`；`git diff --cached --check`。
✅ feat(account): 测试连接支持 OpenAI-compatible Chat Completions 路径: https://github.com/Wei-Shaw/sub2api/pull/2658
  - 同步方式：cherry-pick PR head `ca60cede`，手动解决账号测试服务和 OpenAI 测试文件冲突，并将新增注释改为中文。
  - 决策：保留 fork 已有 OpenAI Responses 探测、raw Chat Completions 网关 URL 构造、compact/image 测试和 OAuth Codex 行为；当 API Key 账号已探测为不支持 Responses API 时，测试连接改走 `/v1/chat/completions` SSE 路径，而不再提示“测试接口仅支持 Responses API”。
  - 决策：前端测试弹窗仅新增 `status` SSE 事件展示，用于呈现 Chat Completions 路径验证状态，不改变现有 content/image/test_complete 流程。
  - 测试：`go test -tags unit ./internal/service -run 'TestAccountTestService_OpenAI|TestNormalizeAccountTestMode|TestBuildOpenAI(ChatCompletionsURL|ResponsesURL_ProbeURL)'`；`NODE_OPTIONS="--localstorage-file=/tmp/tokenrouter-account-tests-localstorage" pnpm test:run src/components/account/__tests__/AccountTestModal.spec.ts`；`pnpm typecheck`。
✅ feat(bedrock): add Claude Code compatibility for AWS Bedrock: https://github.com/Wei-Shaw/sub2api/pull/2662
  - 同步方式：fetch PR 2662 后 cherry-pick 功能提交 `fe1c6c95`，手动解决 `bedrock_request.go` 模块路径冲突。
  - 决策：保留 fork 现有 Bedrock SigV4/API Key、跨区域模型前缀、渠道模型映射和 beta policy 流程；在渠道映射后应用 Bedrock CC 兼容转换，统一清理 Anthropic API 专有字段、补齐 Bedrock 必填字段、修复 thinking/tool_use ID，并过滤 Bedrock 不支持的 beta token。
  - 决策：upstream 将 `features_config.bedrock_cc_compat` 改为布尔开关；当前 fork 前端仍保存为 `{platform: boolean}`。后端同时兼容新版布尔值和既有 map 格式，避免已有渠道配置失效。
  - 测试：`go test ./internal/service -run 'Test(PrepareBedrock|ResolveBedrock|FilterBedrock|AutoInjectBedrock|SanitizeBedrock|IsBedrock|Channel_IsBedrock|ResolveBedrockBetaTokensForRequest)'`；`go test -tags unit ./internal/service -run 'Test(PrepareBedrock|ResolveBedrock|FilterBedrock|AutoInjectBedrock|SanitizeBedrock|IsBedrock|Channel_IsBedrock|ResolveBedrockBetaTokensForRequest)'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`go test ./internal/handler -run 'TestGatewayHandler|TestGatewayModels|Test.*Bedrock|Test.*Messages'`；`git diff --check`。
✅ fix(apicompat): Responses 转 Chat Completions 时 developer role 映射为 system: https://github.com/Wei-Shaw/sub2api/pull/2656
  - 同步方式：cherry-pick PR head `c4d7edba`，无冲突。
  - 决策：仅调整 Responses API 请求桥接到 Chat Completions 时的 role 归一化；`developer` 映射为 `system`，空 role 仍回退 `user`，其它 Chat Completions 合法 role 保持不变。
  - 测试：`go test ./internal/pkg/apicompat -run 'TestResponsesInputToChatMessages|TestResponsesToChatCompletionsRequest|TestResponsesToChatCompletions|TestChatCompletionsToResponses'`；`git diff --cached --check`。
✅ feat(registration): 邮箱白名单支持后缀通配符匹配(*.edu.cn): https://github.com/Wei-Shaw/sub2api/pull/2672
  - 同步方式：cherry-pick PR 功能提交 `a5b9b68b763417a67443a4288573cfb851c2318c`，手动解决中英文 i18n 文案冲突。
  - 决策：保留 fork 现有邮箱地址归一化开关和重复注册防护文案；仅扩展注册邮箱后缀白名单支持 `*.edu.cn` 这类通配符，后端和前端均保持精确域名 `@qq.com` 不匹配子域名，通配符同时匹配根域名及其子域名。
  - 决策：前端白名单标签从隐式 `@` 展示改为展示完整 token，提交设置时精确域名继续补 `@`，通配符保持 `*.`；新增代码注释按本仓库要求改为中文。
  - 测试：`go test -tags unit ./internal/service -run 'Test.*RegistrationEmail|TestSettingService_.*RegistrationEmail'`；`pnpm test:run src/utils/__tests__/registrationEmailPolicy.spec.ts`；`pnpm typecheck`；`git diff --check`；`git diff --cached --check`。
  - 备注：Vitest 输出 Node `localStorage` warning，未影响断言。
✅ feat(risk-control): 内容审计支持按模型生效: https://github.com/Wei-Shaw/sub2api/pull/2674
  - 同步方式：PR 分支包含大量已同步/无关历史，仅 cherry-pick tip 功能提交 `0d5c6f7cc7d8209aa30c7406a3ca88766ad178f5`。
  - 决策：保留 fork 现有 OpenAI Cyber warning 记录、独立自动封禁、记录 Tab 和详情页；在同一内容审计配置中新增 `model_filter`，按客户端请求模型名决定是否执行内容审计，渠道模型映射后的上游模型不改变匹配结果。
  - 决策：前端仅合入模型范围相关 UI/i18n，不引入 PR 分支夹带的 Channel Monitor 等无关翻译；记录页同时保留 fork 的内容审计/Cyber 警告切换和新增模型范围摘要。
  - 测试：`go test ./internal/service -run 'TestContentModeration(Check_ModelFilter|LoadConfig_LegacyConfigDefaultsModelFilter|UpdateConfig|NormalizeKeyword|Check_Keyword|RecordCyber|Cyber)|TestNormalizeContentModerationModel|TestContentModerationConfig'`；`go test ./internal/service -run 'TestContentModeration|TestNormalizeKeywordBlockingMode|TestIsOpenAICyberWarningText'`；`go test ./internal/handler/admin -run 'Test.*ContentModeration|Test.*RiskControl|Test.*Cyber'`；`pnpm test:run src/views/admin/__tests__/RiskControlView.spec.ts`；`pnpm typecheck`；`git diff --check`；`git diff --cached --check`。
  - 备注：handler/admin 指定测试包编译通过但当前 pattern 未匹配到用例；Vitest 输出 Node `localStorage` warning，未影响断言。
✅ fix: optimize OpenAI account cooldown scheduling: https://github.com/TokenFlux/TokenRouter/commit/1e406fed52f82b7e4812c02ad0d668b5446b97c2
  - 同步方式：cherry-pick direct commit `1e406fed52f82b7e4812c02ad0d668b5446b97c2`，手动解决 Wire 构造、并发服务 import、OpenAI token provider 缺失 refresh_token 处理和 OpenAI 403 冷却策略冲突。
  - 决策：保留 fork 当前 `NewGatewayService`、`NewChannelService`、Ops cleanup、OpenAI 403 阈值/冷却可配置和 token 缓存删除行为；新增 upstream 的 OpenAI 运行态调度屏蔽快路径、429 storm failover 限制、并发负载短 TTL 缓存和 token refresh/clear error 后的调度屏蔽同步。
  - 决策：`AdminService` 与 `TokenRefreshService` 注入 `OpenAIGatewayService` 作为 `AccountRuntimeBlocker`，管理员清错和后台刷新成功会清除运行态屏蔽；OpenAI 403 临时冷却继续使用 fork 设置页配置的阈值、窗口和冷却时长，并额外通知运行态调度屏蔽。
  - 测试：`go test ./cmd/server`；`go test ./internal/service -run 'Test(OpenAIAccountRuntimeBlock|OpenAIAccountScheduler|ConcurrencyService|RateLimitService|OpenAITokenProvider|TokenRefresh|SchedulerSnapshot)'`；`go test -tags unit ./internal/server -run TestAPIContracts`；`go test ./internal/handler -run 'Test.*OpenAI|Test.*Gateway|Test.*Images'`；`go test ./internal/service -run 'TestContentModeration|TestOpenAIGateway|TestForwardAsRawChatCompletions|TestForwardAsChatCompletions|TestOpenAIImages|TestOpenAIWS|TestHandleChatStreamingResponse|TestGeminiMessagesCompatService'`；`go test ./internal/config`；`git diff --cached --check`；`git diff --check`。
✅ fix: update x/net vulnerability dependency: https://github.com/TokenFlux/TokenRouter/commit/b6c0b4084878bd8c77b605dff7cbb1f0955b84de
  - 同步方式：cherry-pick direct commit `b6c0b4084878bd8c77b605dff7cbb1f0955b84de`，无冲突。
  - 决策：保留 fork 当前 HTTP server 超时、请求体大小限制和 H2C 配置项；升级 `golang.org/x/net` 及联动的 `x/crypto`、`x/term`、`x/mod`、`x/sys`、`x/text`、`x/tools`，并将 H2C 接入改为 Go 1.26 标准库 `http.Protocols`/`http2.ConfigureServer`，不再使用 `x/net/http2/h2c` wrapper。
  - 测试：`go test ./cmd/server ./internal/server`；`go test -tags unit ./internal/server -run TestAPIContracts`；`git diff --cached --check`；`git diff --check`。
✅ fix: 修复反代部署下拒绝日志客户端 IP 不准确: https://github.com/Wei-Shaw/sub2api/pull/2698
  - 同步方式：PR 分支包含大量已同步/无关历史，仅 cherry-pick tip 功能提交 `0af44ce4c2e97da35b853f3e96d91aee6e724ab9`，手动解决模块路径 import 冲突。
  - 决策：保留 fork 当前 OpenAI gateway 诊断日志字段；仅将 codex_cli_only 拒绝日志里的 `request_client_ip` 改为复用 `ip.GetClientIP(c)`，让反代头解析与 access log/usage 记录一致，同时继续保留 `request_remote_addr` 排查底层 peer。
  - 测试：`go test ./internal/service -run 'TestLogCodexCLIOnlyDetection|TestOpenAIGatewayService_GetCodexClientRestrictionDetector|TestGetAPIKeyIDFromContext'`；`go test ./internal/service -run 'TestOpenAIGateway|TestForwardAsRawChatCompletions|TestForwardAsChatCompletions|TestHandleChatStreamingResponse'`；`git diff --cached --check`；`git diff --check`。
✅ feat(admin): add compact proxy IP resource link: https://github.com/TokenFlux/TokenRouter/commit/0430899748ac7c6c5d26d94cac8cc9b46bccb8ab
  - 同步方式：cherry-pick direct commit `0430899748ac7c6c5d26d94cac8cc9b46bccb8ab`，无冲突。
  - 决策：保留 fork 当前账号创建/编辑弹窗、代理页批量导入/质量检测等结构；仅新增紧凑 `ProxyAdBanner` 外链组件，并放在账号代理选择器和代理创建弹窗 Tab 行旁边，不改变代理 CRUD/API 逻辑。
  - 测试：`pnpm typecheck`；`pnpm test:run src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts src/views/admin/__tests__/ProxiesView.spec.ts`；`git diff --cached --check`；`git diff --check`。
  - 备注：Vitest 当前仅匹配到既有 `EditAccountModal.spec.ts` 并通过；仓库中没有 `CreateAccountModal.spec.ts` 或 `ProxiesView.spec.ts`。
✅ fix(i18n): escape at-sign in email whitelist placeholder: https://github.com/TokenFlux/TokenRouter/commit/0cfabaa82ea11fe296ce81c786841dbfaeccac36
  - 同步方式：cherry-pick direct commit `0cfabaa82ea11fe296ce81c786841dbfaeccac36`，无冲突。
  - 决策：仅转义中英文注册邮箱白名单 placeholder 里的 `@` 为 Vue I18n 安全写法，保留上一条邮箱通配符同步后的 `*.edu.cn` 示例和 fork 的邮箱归一化设置文案。
  - 测试：`pnpm test:run src/views/admin/__tests__/SettingsView.spec.ts src/utils/__tests__/registrationEmailPolicy.spec.ts`；`pnpm typecheck`；`git diff --cached --check`；`git diff --check`。
  - 备注：Vitest 输出 Node `localStorage` warning 和既有 `router-link` stub warning，未影响断言。
✅ 修复 WS 协议下工具输出续链识别问题: https://github.com/Wei-Shaw/sub2api/pull/2747
  - 同步方式：cherry-pick PR head `fc66cd704a996928bc3f1c3d879bbc4feb12f23d`，无冲突；补充新增 helper 的中文注释。
  - 决策：保留 fork 现有 OpenAI WS v2 ingress、store=false 续链和 previous_response_id 自动修复策略，仅把 Codex 的 `tool_search_output`、`custom_tool_call_output`、`mcp_tool_call_output` 等工具输出纳入续链识别，避免工具输出续轮丢失上一轮响应锚点。
  - 测试：`go test ./internal/service -run 'Test(NeedsToolContinuationSignals|HasFunctionCallOutput|HasToolCallContext|FunctionCallOutputCallIDs|HasFunctionCallOutputMissingCallID|OpenAIWSRawPayloadHasToolCallOutput|BuildOpenAIWSReplayInputSequence|OpenAIGatewayService_ProxyResponsesWebSocketFromClient_StoreDisabled(ToolSearchOutput|FunctionCallOutput))'`；`git diff --check`；`git diff --cached --check`。
✅ fix(openai): emit response.failed when /v1/responses SSE aborted post-flush: https://github.com/Wei-Shaw/sub2api/pull/2732
  - 同步方式：按 PR 分支提交顺序 cherry-pick `5e5c2062`、`cff2f291`、`b34cc71b`，无冲突；将 upstream 模块路径改为本 fork 路径，并把新增英文注释改为中文。
  - 决策：保留 fork 现有 Gateway/OpenAI handler 的 legacy 非 Responses 流式错误格式；仅对 `/v1/responses`、裸 `/responses` 和 codex direct `/responses` 路径，在流已开始或 Writer 已写过后追加协议合规的 `response.failed`，避免 Codex CLI 收到 silent EOF。
  - 测试：`go test ./internal/handler -run 'Test(OpenAIHandleStreamingAwareError|GatewayHandleStreamingAwareError|InboundIsResponses|SynthesizeResponseID|MapResponsesErrorCode|OpenAIEnsureForwardErrorResponse|OpenAIRecoverResponsesPanic|GatewayEnsureForwardErrorResponse)'`；`git diff --check`；`git diff --cached --check`。
✅ style: fix lint errors in response.failed SSE writer: https://github.com/TokenFlux/TokenRouter/commit/53acde1efd236e54b629f2122b7dec6469945508
  - 同步方式：cherry-pick direct commit `53acde1efd236e54b629f2122b7dec6469945508`，手动解决 `stream_error_event.go` 与上一条中文注释/模块路径适配冲突。
  - 决策：保留本 fork 模块路径与中文注释；采用 upstream 的 typed struct + `json.Marshal` 写法，避免手写 `strings.Builder` 带来的 errcheck/lint 问题，并继续保证 `output` 序列化为 `[]`。
  - 测试：`go test ./internal/handler -run 'Test(OpenAIHandleStreamingAwareError|GatewayHandleStreamingAwareError|InboundIsResponses|SynthesizeResponseID|MapResponsesErrorCode|OpenAIEnsureForwardErrorResponse|OpenAIRecoverResponsesPanic|GatewayEnsureForwardErrorResponse)'`；`git diff --check`；`git diff --cached --check`。
✅ chore: update sponsors: https://github.com/TokenFlux/TokenRouter/commit/2f70d965bf5b046ad6e9474a77a493bf4fb60801
  - 同步方式：检查并尝试 cherry-pick direct commit `2f70d965bf5b046ad6e9474a77a493bf4fb60801`，随后手动清理冲突，未应用代码或文档内容。
  - 决策：保留 fork 当前中文 `README.md` 主文档结构，以及 `README_CN.md`、`README_JA.md` 已删除状态；upstream sponsor 更新目标是 upstream 英文 README 与多语言 README 文件，当前 fork 没有对应 sponsor 表结构，避免恢复已删除文档或引入孤立 logo。
  - 测试：`git ls-files -u`；`git diff --check`；`git diff --cached --check`。
