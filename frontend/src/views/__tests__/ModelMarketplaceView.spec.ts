import { defineComponent, nextTick } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ModelMarketplaceView from '../ModelMarketplaceView.vue'
import type { MarketplaceGroup, MarketplaceModelPricing } from '@/types'

enableAutoUnmount(afterEach)

const getMarketplaceModels = vi.hoisted(() => vi.fn())
const getBusinessUsageStats = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin/usage', () => ({ getStats: getBusinessUsageStats, default: { getStats: getBusinessUsageStats } }))
const getMarketplaceRoutingHealth = vi.hoisted(() => vi.fn())
const checkAuth = vi.hoisted(() => vi.fn())
const fetchPublicSettings = vi.hoisted(() => vi.fn())
const copyToClipboard = vi.hoisted(() => vi.fn())
const authState = vi.hoisted(() => ({ isAuthenticated: true, isAdmin: false }))

vi.mock('@/api/marketplace', () => ({
  getMarketplaceModels,
  getMarketplaceRoutingHealth,
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: { value: false }, copyToClipboard }),
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    get isAuthenticated() {
      return authState.isAuthenticated
    },
    get isAdmin() {
      return authState.isAdmin
    },
    checkAuth,
  }),
  useAppStore: () => ({
    siteName: 'TokenRouter',
    siteLogo: '',
    docUrl: '',
    cachedPublicSettings: null,
    publicSettingsLoaded: true,
    fetchPublicSettings,
  }),
}))

vi.mock('@/composables/useTheme', () => ({
  initTheme: vi.fn(),
  useTheme: () => ({ isDark: { value: true }, toggleTheme: vi.fn() }),
}))

vi.mock('@/composables/useBalanceDisplay', () => ({
  useBalanceDisplay: () => ({
    balanceUnitName: { value: '点' },
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'zh-CN' },
      t: (key: string, params?: Record<string, string>) => {
        if (key === 'marketplace.rateMultiplierValue') {
          return `marketplace.rateMultiplierValue ${params?.multiplier || ''}`
        }
        if (key === 'marketplace.imageRateMultiplierValue') {
          return `marketplace.imageRateMultiplierValue ${params?.multiplier || ''}`
        }
        if (key === 'marketplace.maxDiscountOff') {
          return `marketplace.maxDiscountOff ${params?.percent || ''}`
        }

        return key
      },
    }),
  }
})

const SelectStub = defineComponent({
  name: 'TokenSelectStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: null,
    },
    options: {
      type: Array,
      default: () => [],
    },
  },
  emits: ['update:modelValue', 'change'],
  template: `
    <div class="select-stub">
      <button
        v-for="option in options"
        :key="String(option.value)"
        type="button"
        :data-testid="'select-option-' + String(option.value)"
        @click="$emit('update:modelValue', option.value); $emit('change', option.value, option)"
      >
        {{ option.label }}
      </button>
    </div>
  `,
})

const SearchInputStub = defineComponent({
  name: 'SearchInput',
  props: {
    modelValue: {
      type: String,
      default: '',
    },
  },
  emits: ['update:modelValue'],
  template: `
    <input
      data-testid="marketplace-search"
      :value="modelValue"
      @input="$emit('update:modelValue', $event.target.value)"
    />
  `,
})

const tokenPricing: MarketplaceModelPricing = {
  pricing_mode: 'token',
  price_status: 'priced',
  input_price_per_token: 0.000001,
  image_input_price_per_token: 0.000003,
  output_price_per_token: 0.000002,
  fast_image_input_price_per_token: 0.000006,
}

const imagePricing: MarketplaceModelPricing = {
  pricing_mode: 'image',
  price_status: 'priced',
  image_price_1k: 0.5,
}

const unpricedPricing: MarketplaceModelPricing = {
  pricing_mode: 'unknown',
  price_status: 'unpriced',
}

function marketplaceFixture(): MarketplaceGroup[] {
  return [
    marketplaceGroup(1, 'Plus', false, [
      marketplaceModel('gpt-5.5', 'GPT 5.5', tokenPricing),
      marketplaceModel('legacy-unpriced', 'Legacy Unpriced', unpricedPricing),
    ]),
    marketplaceGroup(2, 'Pro', false, [
      marketplaceModel('gpt-5.5', 'GPT 5.5', tokenPricing),
    ]),
    marketplaceGroup(3, 'Plus Data Sharing', true, [
      marketplaceModel('gpt-5.5', 'GPT 5.5', tokenPricing),
    ]),
    marketplaceGroup(4, 'Pro Data Sharing', true, [
      marketplaceModel('gpt-5.5', 'GPT 5.5', tokenPricing),
    ]),
  ]
}

function marketplaceGroup(id: number, name: string, dataSharingEnabled: boolean, models: MarketplaceGroup['models']): MarketplaceGroup {
  return {
    id,
    name,
    description: `${name} group`,
    platform: 'openai',
    display_brand: 'OpenAI',
    sort_order: id,
    rate_multiplier: id,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    official_price_ratio: id / 10,
    official_price_rmb_equivalent: id,
    data_sharing_enabled: dataSharingEnabled,
    capacity: {
      concurrency_used: id,
      concurrency_max: 10,
      sessions_used: id,
      sessions_max: 20,
      rpm_used: id,
      rpm_max: 60,
    },
    model_count: models.length,
    models,
  }
}

function marketplaceModel(id: string, displayName: string, pricing: MarketplaceModelPricing): MarketplaceGroup['models'][number] {
  return {
    id,
    display_name: displayName,
    pricing,
  }
}

function routingHealthFixture() {
  return {
    available: true,
    schemaVersion: 1,
    state: 'healthy',
    observedAt: '2026-09-05T02:17:38+08:00',
    currentHit: null,
    providers: [
      {
        supplierName: 'Input Air',
        names: { group: 'input-air', account: 'input-air', key: 'input-air' },
        manual: {
          enabled: true,
          groupEnabled: true,
          accountEnabled: true,
          accountSchedulable: true,
          keyEnabled: true,
        },
        schedulable: true,
        routeState: 'available',
        healthLevel: 'healthy',
        healthScore: 98,
        business: { total: 10, success: 10, successRate: 1 },
        health: { consecutiveFailures: 0, cooling: false, warming: false, lastLatencyMs: 1250 },
        scheduledTest: null,
        availabilityProbe: null,
      },
    ],
  }
}

async function mountMarketplace() {
  const wrapper = mount(ModelMarketplaceView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot name="page-heading-actions" /><slot /></div>' },
        RouterLink: { template: '<a><slot /></a>' },
        Icon: { template: '<span />' },
        LoadingSpinner: { template: '<span />' },
        LocaleSwitcher: { template: '<span />' },
        ProviderIcon: { template: '<span />' },
        GroupCapacityBadge: { template: '<span data-testid="group-capacity" />' },
        SearchInput: SearchInputStub,
        Select: SelectStub,
      },
    },
  })
  await flushPromises()
  return wrapper
}

// HelpTooltip 通过 Teleport 挂到 body，隐藏时用 v-show（display:none）控制。
function visibleTooltips() {
  return Array.from(document.body.querySelectorAll<HTMLElement>('[role="tooltip"]'))
    .filter((el) => el.style.display !== 'none')
}

describe('ModelMarketplaceView', () => {
  beforeEach(() => {
    localStorage.clear()
    authState.isAuthenticated = true
    authState.isAdmin = false
    getMarketplaceModels.mockReset()
    getMarketplaceModels.mockResolvedValue(marketplaceFixture())
    getMarketplaceRoutingHealth.mockReset()
    getBusinessUsageStats.mockReset()
    getBusinessUsageStats.mockResolvedValue({ total_requests: 2, total_input_tokens: 100, total_output_tokens: 40, total_cache_read_tokens: 300, total_cache_creation_tokens: 100 })
    checkAuth.mockClear()
    fetchPublicSettings.mockClear()
    copyToClipboard.mockClear()
  })

  it('管理员按渠道查询统一的滚动 24 小时业务用量，并限制刷新频率', async () => {
    authState.isAdmin = true
    getMarketplaceRoutingHealth.mockResolvedValue(routingHealthFixture())
    const wrapper = await mountMarketplace()
    expect(getBusinessUsageStats).toHaveBeenCalledTimes(4)
    const ranges = getBusinessUsageStats.mock.calls.map(([params]) => params)
    expect(ranges.map(params => params.group_id)).toEqual([1, 2, 3, 4])
    expect(new Set(ranges.map(params => params.end_date)).size).toBe(1)
    expect(Date.parse(ranges[0].end_date) - Date.parse(ranges[0].start_date)).toBe(86_400_000)
    expect(wrapper.findAll('[data-testid="group-business-usage"]')).toHaveLength(4)
    expect(wrapper.get('[data-testid="group-business-usage"]').text()).toContain('60.0%')
    window.dispatchEvent(new Event('focus'))
    await flushPromises()
    expect(getBusinessUsageStats).toHaveBeenCalledTimes(4)
  })

  it('普通用户不查询或显示管理员业务用量', async () => {
    const wrapper = await mountMarketplace()
    expect(getBusinessUsageStats).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="group-business-usage"]').exists()).toBe(false)
  })

  it('业务统计失败保留渠道主体，并明确显示用量不可用', async () => {
    authState.isAdmin = true
    getMarketplaceRoutingHealth.mockResolvedValue(routingHealthFixture())
    getBusinessUsageStats.mockRejectedValue(new Error('offline'))
    const wrapper = await mountMarketplace()
    expect(wrapper.findAll('[data-testid="marketplace-group-section"]')).toHaveLength(4)
    expect(wrapper.get('[data-testid="group-business-usage"]').text()).toContain('marketplace.businessUsageError')
  })

  it('管理员健康快照可用时展示全渠道表', async () => {
    authState.isAdmin = true
    getMarketplaceRoutingHealth.mockResolvedValue(routingHealthFixture())

    const wrapper = await mountMarketplace()

    expect(wrapper.get('[data-testid="routing-health-overview"]')).toBeTruthy()
    expect(wrapper.findAll('[data-testid="routing-health-provider"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('input-air')
  })

  it.each([
    [401, 'marketplace.routingHealthAuthRequired'],
    [403, 'marketplace.routingHealthForbidden'],
    [0, 'marketplace.routingHealthNetworkError'],
  ])('管理员健康请求状态 %s 显示可判断错误', async (status, translationKey) => {
    authState.isAdmin = true
    getMarketplaceRoutingHealth.mockRejectedValue({ status, message: 'redacted' })

    const wrapper = await mountMarketplace()

    expect(wrapper.get('[data-testid="routing-health-load-state"]').text()).toContain(translationKey)
  })

  it('后端可访问但健康快照源不可用时显示源状态', async () => {
    authState.isAdmin = true
    getMarketplaceRoutingHealth.mockResolvedValue({
      available: false,
      schemaVersion: 1,
      state: 'unavailable',
      providers: [],
    })

    const wrapper = await mountMarketplace()

    expect(wrapper.get('[data-testid="routing-health-load-state"]').text())
      .toContain('marketplace.routingHealthSourceUnavailable')
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('默认按分组-模型展示', async () => {
    const wrapper = await mountMarketplace()

    expect(wrapper.findAll('[data-testid="marketplace-group-section"]')).toHaveLength(4)
    expect(wrapper.findAll('[data-testid="marketplace-group-section"]').map((section) => section.text()).join('\n')).toContain('marketplace.dataSharingTag')
  })

  it('用户侧不展示分组容量，并将可用率状态条靠右放置', async () => {
    const fixture = marketplaceFixture()
    fixture[0] = {
      ...fixture[0],
      availability: {
        window_days: 1,
        bucket_minutes: 15,
        success_count: 90,
        total_count: 96,
        availability_rate: 90 / 96,
        last_status: 'failed',
        last_checked_at: '2026-09-04T09:05:00Z',
        last_latency_ms: 4321,
        consecutive_failures: 3,
        days: [],
      },
    }
    getMarketplaceModels.mockResolvedValue(fixture)

    const wrapper = await mountMarketplace()

    expect(wrapper.findAll('[data-testid="group-capacity"]')).toHaveLength(0)
    expect(wrapper.get('[data-testid="marketplace-group-availability"]').classes()).toContain('xl:w-[560px]')
    expect(wrapper.get('[data-testid="probe-current-status"]').text()).toContain('marketplace.probeStatusUnavailable')
    expect(wrapper.get('[data-testid="probe-current-latency"]').text()).toBe('4.32 s')
    expect(wrapper.get('[data-testid="probe-consecutive-failures"]').text()).toBe('3')
    expect(wrapper.get('[data-testid="probe-history-bar"]').findAll('span')).toHaveLength(96)
  })

  it('探测成功但超过八秒时显示为响应慢，而不是异常', async () => {
    const fixture = marketplaceFixture()
    fixture[0] = {
      ...fixture[0],
      availability: {
        window_days: 1,
        bucket_minutes: 15,
        success_count: 96,
        total_count: 96,
        availability_rate: 1,
        last_status: 'success',
        last_checked_at: '2026-09-04T09:05:00Z',
        last_latency_ms: 12_001,
        consecutive_failures: 0,
        days: [],
      },
    }
    getMarketplaceModels.mockResolvedValue(fixture)

    const wrapper = await mountMarketplace()

    const status = wrapper.get('[data-testid="probe-current-status"]')
    expect(status.text()).toContain('marketplace.probeStatusSlow')
    expect(status.classes()).toContain('text-amber-700')
    expect(status.text()).not.toContain('marketplace.probeStatusUnavailable')
  })

  it('每 30 秒静默刷新，并在窗口重新聚焦时立即刷新', async () => {
    vi.useFakeTimers()
    const wrapper = await mountMarketplace()
    expect(getMarketplaceModels).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(30_000)
    await flushPromises()
    expect(getMarketplaceModels).toHaveBeenCalledTimes(2)

    window.dispatchEvent(new Event('focus'))
    await flushPromises()
    expect(getMarketplaceModels).toHaveBeenCalledTimes(3)

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(getMarketplaceModels).toHaveBeenCalledTimes(3)
  })

  it('分组头部用最高优惠标签替换模型数标签，无有效折扣的分组不渲染', async () => {
    const fixture = marketplaceFixture()
    // ratio >= 1 表示无折扣；undefined 表示后端未下发官方价比例。
    fixture[1] = { ...fixture[1], official_price_ratio: 1.2 }
    fixture[2] = { ...fixture[2], official_price_ratio: undefined }
    getMarketplaceModels.mockResolvedValue(fixture)

    const wrapper = await mountMarketplace()
    const sections = wrapper.findAll('[data-testid="marketplace-group-section"]')

    // fixture 中 group 1 的 official_price_ratio 为 0.1，即最高优惠 90%。
    expect(sections[0].text()).toContain('marketplace.maxDiscountOff 90')
    expect(sections[0].text()).not.toContain('marketplace.modelsStat')
    expect(sections[1].text()).not.toContain('marketplace.maxDiscountOff')
    expect(sections[2].text()).not.toContain('marketplace.maxDiscountOff')
    expect(sections[3].text()).toContain('marketplace.maxDiscountOff 60')

    // 移动端点击 tag 也能查看说明：点击后弹出提示气泡。
    await sections[0].get('[data-testid="group-max-discount-tag"]').trigger('click')
    await nextTick()
    expect(visibleTooltips().some((el) => el.textContent?.includes('marketplace.maxDiscountHint'))).toBe(true)
    await sections[0].get('[data-testid="group-rate-multiplier-tag"]').trigger('click')
    await nextTick()
    expect(visibleTooltips().some((el) => el.textContent?.includes('marketplace.rateMultiplierHint'))).toBe(true)
  })

  it('模型卡片中的模型 ID 支持一键复制', async () => {
    const wrapper = await mountMarketplace()

    // 卡片 ID 行的复制按钮直接复制模型 ID。
    const cardCopyButtons = wrapper.findAll('[data-testid="model-id-copy"]')
    expect(cardCopyButtons.length).toBeGreaterThan(0)
    await cardCopyButtons[0].trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith('gpt-5.5')
  })

  it('模型卡片右上角展示输入输出能力标签', async () => {
    const wrapper = await mountMarketplace()

    const sections = wrapper.findAll('[data-testid="marketplace-group-section"]')
    const gptCards = sections[0].findAll('article')
    // fixture 中 gpt-5.5 定价数据带图片输入价，应展示 文字·图片 -> 文字。
    expect(gptCards[0].get('[data-testid="model-capability-tags"]').findAll('[data-modality]').map((tag) => tag.attributes('data-modality')))
      .toEqual(['input-text', 'input-image', 'output-text'])
    // 无定价数据的模型回退为纯文本能力。
    expect(gptCards[1].get('[data-testid="model-capability-tags"]').findAll('[data-modality]').map((tag) => tag.attributes('data-modality')))
      .toEqual(['input-text', 'output-text'])
  })

  it('能力标签优先使用接口下发的模态元数据', async () => {
    const fixture = marketplaceFixture()
    fixture[0] = {
      ...fixture[0],
      models: [
        // 本地规则会把 gpt-image-2 识别为 text+image 输入，接口数据应覆盖为纯文字输入。
        { ...marketplaceModel('gpt-image-2', 'GPT Image 2', imagePricing), input_modalities: ['text'], output_modalities: ['image'] },
      ],
    }
    getMarketplaceModels.mockResolvedValue(fixture)

    const wrapper = await mountMarketplace()
    const sections = wrapper.findAll('[data-testid="marketplace-group-section"]')

    expect(sections[0].get('[data-testid="model-capability-tags"]').findAll('[data-modality]').map((tag) => tag.attributes('data-modality')))
      .toEqual(['input-text', 'output-image'])
  })

  it('xAI 品牌在分组模式下展示 Grok 图标而不是字母占位', async () => {
    const fixture = marketplaceFixture()
    fixture[0] = {
      ...fixture[0],
      name: 'Grok',
      display_brand: 'xAI',
    }
    getMarketplaceModels.mockResolvedValue(fixture)

    const wrapper = await mountMarketplace()

    // 品牌名 xAI 应映射到现有 Grok SVG，不能退回紫色字母 X。
    const grokGroup = wrapper.findAll('[data-testid="marketplace-group-section"]')
      .find((section) => section.get('h2').text() === 'Grok')
    expect(grokGroup?.find('.model-icon').exists()).toBe(true)
    expect(grokGroup?.find('.model-icon-fallback').exists()).toBe(false)
  })

  it('展示开启独立配置的生图倍率', async () => {
    const fixture = marketplaceFixture()
    fixture[0] = {
      ...fixture[0],
      image_rate_independent: true,
      image_rate_multiplier: 0.5,
      models: [
        marketplaceModel('gpt-image-1', 'GPT Image', imagePricing),
      ],
    }
    getMarketplaceModels.mockResolvedValue(fixture)

    const wrapper = await mountMarketplace()

    expect(wrapper.findAll('[data-testid="marketplace-group-section"]').map((section) => section.text()).join('\n')).toContain('marketplace.imageRateMultiplierValue x0.50')
  })

  it('模型卡片可展开抽屉式定价面板并切换 fast mode', async () => {
    const wrapper = await mountMarketplace()

    const toggle = wrapper.get('[data-testid="model-pricing-toggle"]')
    expect(toggle.attributes('aria-expanded')).toBe('false')

    await toggle.trigger('click')
    await nextTick()

    // 面板原地展开，展示完整定价（标准计费行），且 fast mode 切换可用。
    const sections = wrapper.findAll('[data-testid="marketplace-group-section"]')
    const firstCard = sections[0].findAll('article')[0]
    expect(firstCard.text()).toContain('marketplace.input')
    expect(firstCard.text()).toContain('marketplace.imageInput')
    expect(firstCard.text()).toContain('marketplace.output')

    const fastSwitch = firstCard.get('[data-testid="pricing-fast-switch"]')
    await fastSwitch.findAll('button')[1].trigger('click')
    await nextTick()

    // fast mode 下展示 fast 加价行，标准行隐藏。
    expect(firstCard.text()).toContain('marketplace.fastImageInput')
    expect(firstCard.text()).not.toContain('marketplace.imageInput')

    await wrapper.get('[data-testid="model-pricing-toggle"]').trigger('click')
    await nextTick()
    expect(wrapper.get('[data-testid="model-pricing-toggle"]').attributes('aria-expanded')).toBe('false')
  })
})
