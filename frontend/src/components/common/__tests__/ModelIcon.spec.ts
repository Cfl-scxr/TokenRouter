import { mount } from '@vue/test-utils'
import ModelIcon from '../ModelIcon.vue'
import ProviderIcon from '../ProviderIcon.vue'

describe('ModelIcon', () => {
  it.each([
    ['OpenAI', 'gpt-5.6'],
    ['Grok', 'grok-4.5'],
  ])('%s 模型图标继承主题文字色', (_provider, model) => {
    const wrapper = mount(ModelIcon, {
      props: {
        model,
        size: '28px',
      },
    })

    expect(wrapper.get('svg').attributes('width')).toBe('28px')
    expect(wrapper.get('path').attributes('fill')).toBe('currentColor')
  })
})

describe('ProviderIcon', () => {
  it('xAI 徽标继承主题文字色', () => {
    const wrapper = mount(ProviderIcon, {
      props: {
        brand: 'xAI',
      },
    })

    expect(wrapper.get('path').attributes('fill')).toBe('currentColor')
  })
})
