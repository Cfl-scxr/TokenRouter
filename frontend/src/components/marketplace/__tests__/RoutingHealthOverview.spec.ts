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
  it('同供应商只标记真实命中分组，并展示脱敏换路原因', () => {
    const channels = [provider(1), provider(2)]
    channels.forEach((item) => { item.supplierName = 'Input' })
    const wrapper = mount(RoutingHealthOverview, { props: { snapshot: {
      available: true, schemaVersion: 1, state: 'observed', providers: channels,
      currentHit: { supplierName: 'Input', groupName: 'channel-2', model: 'gpt-5.6-sol' },
      lastSwitch: { from: 'PQ', to: 'channel-2', reason: 'timeout', model: 'gpt-5.6-sol', durationMs: 15020 },
    } } })
    expect(wrapper.findAll('[data-recent-hit="true"]')).toHaveLength(1)
    expect(wrapper.get('[data-recent-hit="true"]').text()).toContain('channel-2')
    expect(wrapper.get('[data-testid="routing-last-switch"]').text()).toContain('15.02 s')
    expect(wrapper.get('[data-testid="routing-last-switch"]').text()).toContain('marketplace.routingSwitchReasons.timeout')
  })
  it('统一状态优先于旧健康等级，分数可视化不改变长期分', () => {
    const channel = provider(1)
    channel.currentStatus = 'interrupted'
    channel.healthScore = 81
    const wrapper = mount(RoutingHealthOverview, { props: { snapshot: { available: true, schemaVersion: 1, state: 'observed', providers: [channel] } } })
    expect(wrapper.text()).toContain('marketplace.probeStatusInterrupted')
    expect(wrapper.get('[role="meter"]').attributes('aria-valuenow')).toBe('81')
  })
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
    expect(wrapper.get('th[title="marketplace.routingHealthScoreHint"]').exists()).toBe(true)
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

  it('对尚未探测状态给出非故障解释', () => {
    const snapshot: MarketplaceRoutingHealthSnapshot = {
      available: true,
      schemaVersion: 1,
      state: 'observed',
      observedAt: '2026-09-04T12:39:31.675Z',
      routingChainId: 'tokenrouter-primary',
      providers: [provider(1, 'unknown')],
    }

    const wrapper = mount(RoutingHealthOverview, { props: { snapshot } })

    expect(wrapper.text()).toContain('marketplace.probeStatusUnknown')
    expect(wrapper.get('[data-testid="routing-health-provider"] span[title="marketplace.probeStatusUnknownHint"]').exists()).toBe(true)
  })

  it('单次失败进入冷却时显示波动并保留健康分', () => {
    const transient = provider(1, 'unavailable')
    transient.routeState = 'cooldown'
    transient.healthScore = 76
    transient.health.consecutiveFailures = 1
    if (transient.scheduledTest) transient.scheduledTest.result = 'failed'
    const snapshot: MarketplaceRoutingHealthSnapshot = {
      available: true,
      schemaVersion: 1,
      state: 'observed',
      providers: [transient],
    }

    const wrapper = mount(RoutingHealthOverview, { props: { snapshot } })
    const cells = wrapper.get('[data-testid="routing-health-provider"]').findAll('td')

    expect(cells[1].text()).toContain('marketplace.routingHealthDegraded')
    expect(cells[2].text()).toBe('76')
    expect(cells[1].get('span').attributes('title')).toBe('marketplace.routingHealthRouteCooldownHint')
  })

  it('只有累计故障才显示故障和零分', () => {
    const failed = provider(1, 'unavailable')
    failed.routeState = 'unavailable'
    failed.healthScore = 0
    failed.health.consecutiveFailures = 2
    const snapshot: MarketplaceRoutingHealthSnapshot = {
      available: true,
      schemaVersion: 1,
      state: 'observed',
      providers: [failed],
    }

    const wrapper = mount(RoutingHealthOverview, { props: { snapshot } })
    const cells = wrapper.get('[data-testid="routing-health-provider"]').findAll('td')

    expect(cells[1].text()).toContain('marketplace.routingHealthUnavailable')
    expect(cells[2].text()).toBe('0')
    expect(cells[1].get('span').attributes('title')).toBe('marketplace.routingHealthRouteUnavailableHint')
  })

  it('累计失败达到门槛时即使仍在冷却也显示故障', () => {
    const failed = provider(1, 'unavailable')
    failed.routeState = 'cooldown'
    failed.healthScore = 0
    failed.health.consecutiveFailures = 3
    const snapshot: MarketplaceRoutingHealthSnapshot = {
      available: true,
      schemaVersion: 1,
      state: 'observed',
      providers: [failed],
    }

    const wrapper = mount(RoutingHealthOverview, { props: { snapshot } })
    const cells = wrapper.get('[data-testid="routing-health-provider"]').findAll('td')

    expect(cells[1].text()).toContain('marketplace.routingHealthUnavailable')
    expect(cells[2].text()).toBe('0')
    expect(cells[1].get('span').attributes('title')).toBe('marketplace.routingHealthRouteUnavailableHint')
  })

  it('恢复观察不显示为未知或故障', () => {
    const recovering = provider(1, 'recovering')
    recovering.routeState = 'warming'
    recovering.healthScore = 69
    recovering.health.warming = true
    const snapshot: MarketplaceRoutingHealthSnapshot = {
      available: true,
      schemaVersion: 1,
      state: 'observed',
      providers: [recovering],
    }

    const wrapper = mount(RoutingHealthOverview, { props: { snapshot } })
    const cells = wrapper.get('[data-testid="routing-health-provider"]').findAll('td')

    expect(cells[1].text()).toContain('marketplace.routingHealthRecovering')
    expect(cells[2].text()).toBe('69')
    expect(cells[1].get('span').attributes('title')).toBe('marketplace.routingHealthRouteWarmingHint')
  })
})
