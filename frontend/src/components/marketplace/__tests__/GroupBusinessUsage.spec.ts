import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import GroupBusinessUsage from '../GroupBusinessUsage.vue'
import type { AdminUsageStatsResponse } from '@/api/admin/usage'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: { value: 'en' }, t: (key: string) => key }) }))
const stats = (values = {}) => ({ total_requests: 2, total_input_tokens: 100, total_output_tokens: 50, total_cache_read_tokens: 300, total_cache_creation_tokens: 100, ...values }) as AdminUsageStatsResponse

describe('GroupBusinessUsage', () => {
  it('缓存比例包含写入和未缓存输入，不包含输出', () => {
    const wrapper = mount(GroupBusinessUsage, { props: { stats: stats() } })
    expect(wrapper.text()).toContain('60.0%')
  })
  it('无调用保留真实零值，比例未知', () => {
    const wrapper = mount(GroupBusinessUsage, { props: { stats: stats({ total_requests: 0, total_input_tokens: 0, total_cache_read_tokens: 0, total_cache_creation_tokens: 0 }) } })
    expect(wrapper.text()).toContain('marketplace.businessUsageEmpty')
    expect(wrapper.findAll('dd').map(node => node.text())).toEqual(['0', '0', '50', '0', '0', '-'])
  })
  it('字段缺失不冒充零或有效命中率，失败时注明历史数据', () => {
    const wrapper = mount(GroupBusinessUsage, { props: { stats: stats({ total_cache_creation_tokens: undefined }), error: true } })
    expect(wrapper.findAll('dd').slice(-2).map(node => node.text())).toEqual(['-', '-'])
    expect(wrapper.text()).toContain('marketplace.businessUsageStale')
  })
})
