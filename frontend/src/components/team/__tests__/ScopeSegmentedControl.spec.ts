import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ScopeSegmentedControl from '../ScopeSegmentedControl.vue'

const mocks = vi.hoisted(() => ({ replace: vi.fn() }))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-router', () => ({
  useRoute: () => ({ query: { retained: 'yes', scope: 'personal' } }),
  useRouter: () => ({ replace: mocks.replace })
}))

describe('ScopeSegmentedControl', () => {
  it('切换作用域时更新模型并保留其他 URL 参数', async () => {
    mocks.replace.mockResolvedValue(undefined)
    const wrapper = mount(ScopeSegmentedControl, { props: { modelValue: 'personal' } })

    await wrapper.findAll('button')[1].trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([['team']])
    expect(mocks.replace).toHaveBeenCalledWith({ query: { retained: 'yes', scope: 'team' } })
    expect(wrapper.emitted('change')).toEqual([['team']])
  })
})
