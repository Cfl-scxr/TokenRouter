import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ChannelUpstreamAssetCell from '../ChannelUpstreamAssetCell.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      const labels: Record<string, string> = {
        'marketplace.routingHealthUpstreamBalance': `余额 ${params?.value}`,
        'marketplace.routingHealthUpstreamQuotaPercent': `额度 ${params?.percent}% 可用`,
        'marketplace.routingHealthUpstreamQuotaRemaining': `额度剩余 ${params?.value}`,
        'marketplace.routingHealthUpstreamUnlimited': '额度 不限量',
        'marketplace.routingHealthEffectiveMultiplier': `折算倍率 ${params?.multiplier}`,
        'marketplace.routingHealthMoreAccounts': `另有 ${params?.count} 个账号`,
      }
      return labels[key] ?? key
    },
  }),
}))

describe('ChannelUpstreamAssetCell', () => {
  it('紧凑展示上游余额、剩余额度和分组折算倍率', () => {
    const wrapper = mount(ChannelUpstreamAssetCell, { props: { assets: [{
      accountId: 8,
      accountName: 'fastaitoken',
      rateMultiplier: 0.8,
      groupRateMultiplier: 2,
      usage: {
        account_id: 8,
        adapter: 'sub2api',
        observed_at: '2026-09-05T09:00:00Z',
        provider: 'sub2api',
        mode: 'quota',
        unit: 'USD',
        balance: { remaining: 12.5 },
        limits: [{ name: 'key_quota', used: 25, limit: 100, remaining: 75 }],
      },
    }] } })

    expect(wrapper.text()).toContain('余额 $12.5')
    expect(wrapper.text()).toContain('额度 75% 可用')
    expect(wrapper.text()).toContain('折算倍率 x1.6')
  })

  it('支持多币种余额和无限额度，倍率缺失时按一折算', () => {
    const populated = mount(ChannelUpstreamAssetCell, { props: { assets: [{
      accountId: 5,
      accountName: 'deepseek',
      usage: {
        account_id: 5,
        adapter: 'deepseek_balance',
        observed_at: '2026-09-05T09:00:00Z',
        balances: [
          { currency: 'CNY', remaining: 20 },
          { currency: 'USD', remaining: 3.5 },
        ],
        subscription: { plan_name: 'unlimited', unlimited: true },
      },
    }] } })
    expect(populated.text()).toContain('余额 ¥20 · $3.5')
    expect(populated.text()).toContain('额度 不限量')

    const defaultRate = mount(ChannelUpstreamAssetCell, { props: { assets: [{ accountId: 6, accountName: 'unknown' }] } })
    expect(defaultRate.text()).toContain('折算倍率 x1')

    const invalidRate = mount(ChannelUpstreamAssetCell, { props: { assets: [{ accountId: 7, accountName: 'invalid', rateMultiplier: -1 }] } })
    expect(invalidRate.text()).toBe('-')
  })
})
