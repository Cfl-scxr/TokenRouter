import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GroupClientProtocolSelector from '../GroupClientProtocolSelector.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

describe('GroupClientProtocolSelector', () => {
  it('locks required protocols and toggles optional protocols', async () => {
    const wrapper = mount(GroupClientProtocolSelector, {
      props: {
        platform: 'openai',
        modelValue: ['openai_responses', 'openai_chat_completions']
      },
      global: {
        stubs: { Icon: { template: '<span />' } }
      }
    })

    expect(wrapper.get('[data-protocol="openai_responses"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-protocol="openai_chat_completions"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-protocol="anthropic_messages"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([
      'anthropic_messages',
      'openai_responses',
      'openai_chat_completions'
    ])
  })

  it('allows Qoder to disable every supported protocol', async () => {
    const wrapper = mount(GroupClientProtocolSelector, {
      props: {
        platform: 'qoder',
        modelValue: ['anthropic_messages']
      },
      global: {
        stubs: { Icon: { template: '<span />' } }
      }
    })

    await wrapper.get('[data-protocol="anthropic_messages"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([])
  })
})
