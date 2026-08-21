import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import UpstreamUsageConfigEditor from '../UpstreamUsageConfigEditor.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: '<button data-testid="select" @click="$emit(\'update:modelValue\', \'new_api\')">{{ options?.length }}</button>'
}

describe('UpstreamUsageConfigEditor', () => {
  it('使用项目 Select 并保留默认配置与禁用状态', async () => {
    const wrapper = mount(UpstreamUsageConfigEditor, {
      props: { enabled: true, adapter: 'sub2api', baseUrl: '' },
      global: {
        stubs: { Select: SelectStub },
        mocks: { $t: (key: string) => key }
      }
    })
    expect(wrapper.find('[data-testid="upstream-usage-enabled"]').element).toBeTruthy()
    expect(wrapper.find('[data-testid="upstream-usage-adapter"]').exists()).toBe(true)
    await wrapper.find('[data-testid="upstream-usage-adapter"]').trigger('click')
    expect(wrapper.emitted('update:adapter')?.[0]).toEqual(['new_api'])

    await wrapper.setProps({ enabled: false })
    expect(wrapper.find('[data-testid="select"]').exists()).toBe(false)
  })
})
