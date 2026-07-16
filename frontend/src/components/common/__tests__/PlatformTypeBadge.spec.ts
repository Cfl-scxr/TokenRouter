import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import PlatformTypeBadge from '../PlatformTypeBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('PlatformTypeBadge', () => {
  it('renders Qoder COSY accounts as Qoder instead of Gemini', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'qoder',
        type: 'cosy'
      },
      global: {
        stubs: {
          PlatformIcon: true,
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Qoder')
    expect(wrapper.text()).toContain('COSY')
    expect(wrapper.text()).not.toContain('Gemini')
  })

  it('distinguishes Agent Identity, PAT, and OAuth accounts', async () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'openai',
        type: 'oauth',
        authMode: 'agentIdentity'
      },
      global: {
        stubs: {
          PlatformIcon: true,
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Agent Identity')

    await wrapper.setProps({ authMode: 'personalAccessToken' })
    expect(wrapper.text()).toContain('PAT')
    expect(wrapper.text()).not.toContain('Agent Identity')

    await wrapper.setProps({ authMode: undefined })
    expect(wrapper.text()).toContain('OAuth')
  })
})
