import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserDashboardHeatmap from '../UserDashboardHeatmap.vue'
import { usageAPI } from '@/api/usage'
import { formatDateLocalInput } from '@/utils/format'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'en' },
    }),
  }
})

vi.mock('@/composables/useBalanceDisplay', () => ({
  useBalanceDisplay: () => ({
    formatBalanceAmount: (value: number) => String(value),
  }),
}))

vi.mock('@/api/usage', () => ({
  usageAPI: {
    getDashboardTrend: vi.fn(),
  },
}))

// 与组件内部保持一致的日期范围：今天向前 364 天再对齐到周日
const expectedRange = () => {
  const end = new Date()
  const start = new Date(end)
  start.setDate(start.getDate() - 364)
  start.setDate(start.getDate() - start.getDay())
  return { start, end }
}

// 真实天数按日历日计算：364 天恰好 52 周，对齐周日回溯的天数等于今天的星期数
// 不能用毫秒差计算天数，避免夏令时切换导致少一天
const expectedRealDays = (end: Date) => 365 + end.getDay()

const mountHeatmap = async () => {
  const wrapper = mount(UserDashboardHeatmap, {
    global: {
      stubs: { LoadingSpinner: true },
    },
  })
  await flushPromises()
  return wrapper
}

describe('UserDashboardHeatmap', () => {
  beforeEach(() => {
    vi.mocked(usageAPI.getDashboardTrend).mockReset()
  })

  it('按近一年整周范围请求按日趋势数据', async () => {
    vi.mocked(usageAPI.getDashboardTrend).mockResolvedValue({
      trend: [],
      start_date: '',
      end_date: '',
      granularity: 'day',
    })

    await mountHeatmap()

    const { start, end } = expectedRange()
    expect(usageAPI.getDashboardTrend).toHaveBeenCalledWith({
      start_date: formatDateLocalInput(start),
      end_date: formatDateLocalInput(end),
      granularity: 'day',
    })
  })

  it('渲染整周网格，接口未返回的日期补零为无用量档', async () => {
    const { end } = expectedRange()
    const today = formatDateLocalInput(end)
    vi.mocked(usageAPI.getDashboardTrend).mockResolvedValue({
      trend: [
        {
          date: today,
          requests: 3,
          input_tokens: 10,
          output_tokens: 20,
          cache_creation_tokens: 0,
          cache_read_tokens: 0,
          total_tokens: 30,
          cost: 0.1,
          actual_cost: 0.2,
        },
      ],
      start_date: '',
      end_date: '',
      granularity: 'day',
    })

    const wrapper = await mountHeatmap()

    // 格子总数在最后一周用不可见占位格补齐到整周
    const realDays = expectedRealDays(end)
    const cells = wrapper.findAll('[data-testid="heatmap-cell"]')
    expect(cells.length % 7).toBe(0)
    expect(cells.length).toBeGreaterThanOrEqual(realDays)
    expect(cells.length).toBeLessThan(realDays + 7)

    // 有用量的今天为最高档，其余真实日期补零为 0 档，占位格不可见
    expect(cells[realDays - 1].classes()).toContain('bg-green-700')
    expect(cells.filter((c) => c.classes().includes('bg-gray-100')).length).toBe(realDays - 1)
    expect(cells.filter((c) => c.classes().includes('invisible')).length).toBe(cells.length - realDays)
  })

  it('悬停格子时显示当天用量，无用量日期显示无用量', async () => {
    const { end } = expectedRange()
    const today = formatDateLocalInput(end)
    vi.mocked(usageAPI.getDashboardTrend).mockResolvedValue({
      trend: [
        {
          date: today,
          requests: 3,
          input_tokens: 10,
          output_tokens: 20,
          cache_creation_tokens: 0,
          cache_read_tokens: 0,
          total_tokens: 30,
          cost: 0.1,
          actual_cost: 0.2,
        },
      ],
      start_date: '',
      end_date: '',
      granularity: 'day',
    })

    const wrapper = await mountHeatmap()
    const cells = wrapper.findAll('[data-testid="heatmap-cell"]')
    const realDays = expectedRealDays(end)

    // 今天为最后一个真实格子；前一天无用量
    await cells[realDays - 1].trigger('mouseenter')
    const tooltip = wrapper.get('[data-testid="heatmap-tooltip"]')
    expect(tooltip.text()).toContain('dashboard.requests')
    expect(tooltip.text()).toContain('dashboard.tokens')
    expect(tooltip.text()).toContain('dashboard.heatmapCost')
    expect(tooltip.text()).toContain('0.2')

    await cells[realDays - 1].trigger('mouseleave')
    expect(wrapper.find('[data-testid="heatmap-tooltip"]').exists()).toBe(false)

    await cells[realDays - 2].trigger('mouseenter')
    expect(wrapper.get('[data-testid="heatmap-tooltip"]').text()).toContain('dashboard.heatmapNoUsage')
  })

  it('左侧渲染周一/周三/周五的星期标签', async () => {
    vi.mocked(usageAPI.getDashboardTrend).mockResolvedValue({
      trend: [],
      start_date: '',
      end_date: '',
      granularity: 'day',
    })

    const wrapper = await mountHeatmap()
    // 周一/周三/周五用窄格式渲染（英文为 M/W/F），放在网格第 1 列的第 2/4/6 行
    const labels = [1, 3, 5].map((d) =>
      new Date(2024, 0, d).toLocaleDateString('en', { weekday: 'narrow' })
    )
    for (const [i, dayOfWeek] of [1, 3, 5].entries()) {
      const el = wrapper.find(`[style*="grid-column: 1"][style*="grid-row: ${dayOfWeek + 2}"]`)
      expect(el.exists()).toBe(true)
      expect(el.text()).toBe(labels[i])
    }
  })

  it('reload 会重新请求数据', async () => {
    vi.mocked(usageAPI.getDashboardTrend).mockResolvedValue({
      trend: [],
      start_date: '',
      end_date: '',
      granularity: 'day',
    })

    const wrapper = await mountHeatmap()
    expect(usageAPI.getDashboardTrend).toHaveBeenCalledTimes(1)

    await (wrapper.vm as unknown as { reload: () => Promise<void> }).reload()
    await flushPromises()
    expect(usageAPI.getDashboardTrend).toHaveBeenCalledTimes(2)
  })
})
