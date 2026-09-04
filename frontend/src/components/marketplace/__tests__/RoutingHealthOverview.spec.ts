import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import RoutingHealthOverview from '../RoutingHealthOverview.vue'
import type { MarketplaceRoutingHealthProvider, MarketplaceRoutingHealthSnapshot } from '@/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    locale: { value: 'zh-CN' },
    t: (key: string, params?: Record<string, string>) => {
      if (key === 'marketplace.routingHealthCurrentHit') {
        return `${params?.supplier} / ${params?.model}`
      }
      return key
    },
  }),
}))

function provider(index: number, healthLevel = 'healthy'): MarketplaceRoutingHealthProvider {
  const group = index === 9 ? 'omni' : `channel-${index}`
  const manualEnabled = index !== 9
  return {
    supplierName: group,
    names: { group, account: group, key: group },
    manual: {
      enabled: manualEnabled,
      groupEnabled: true,
      accountEnabled: true,
      accountSchedulable: manualEnabled,
      keyEnabled: true,
    },
    schedulable: manualEnabled,
    routeState: manualEnabled ? 'available' : 'manual_disabled',
    healthLevel: manualEnabled ? healthLevel : 'manual_disabled',
    healthScore: manualEnabled ? 95 : null,
    business: { total: 3, success: 2, successRate: 2 / 3 },
    health: {
      lastLatencyMs: null,
      consecutiveFailures: index,
      cooling: false,
      warming: false,
    },
    scheduledTest: {
      kind: 'scheduled_test',
      result: 'success',
      observedAt: '2026-09-04T12:38:03.028Z',
      latencyMs: 2988,
    },
    availabilityProbe: manualEnabled ? {
      kind: 'availability_probe',
      result: 'success',
      observedAt: '2026-09-04T12:39:03.028Z',
      consecutiveSuccesses: 2,
      nextProbeAt: '2026-09-04T12:42:03.028Z',
    } : null,
  }
}

describe('RoutingHealthOverview', () => {
  it('展示全部九家渠道并区分人工停用和业务健康字段', () => {
    const snapshot: MarketplaceRoutingHealthSnapshot = {
      available: true,
      schemaVersion: 1,
      state: 'observed',
      observedAt: '2026-09-04T12:39:31.675Z',
      routingChainId: 'tokenrouter-primary',
      currentHit: { supplierName: 'PQ', model: 'gpt-5.6-sol' },
      providers: Array.from({ length: 9 }, (_, index) => provider(index + 1, index === 1 ? 'degraded' : 'healthy')),
    }
    snapshot.providers[0].health.lastLatencyMs = 12.80328799970448

    const wrapper = mount(RoutingHealthOverview, { props: { snapshot } })

    expect(wrapper.findAll('[data-testid="routing-health-provider"]')).toHaveLength(9)
    expect(wrapper.text()).toContain('PQ / gpt-5.6-sol')
    expect(wrapper.text()).toContain('66.7% (2/3)')
    expect(wrapper.text()).toContain('2.99 s')
    expect(wrapper.text()).toContain('13 ms')
    expect(wrapper.text()).toContain('marketplace.routingHealthManualDisabled')
    expect(wrapper.text()).toContain('marketplace.routingHealthNoAutoReturn')
  })

  it('健康快照不可用时显示独立降级状态', () => {
    const snapshot: MarketplaceRoutingHealthSnapshot = {
      available: false,
      schemaVersion: 1,
      state: 'unavailable',
      providers: [],
    }

    const wrapper = mount(RoutingHealthOverview, { props: { snapshot } })

    expect(wrapper.text()).toContain('marketplace.routingHealthSourceUnavailable')
    expect(wrapper.findAll('[data-testid="routing-health-provider"]')).toHaveLength(0)
  })
})
