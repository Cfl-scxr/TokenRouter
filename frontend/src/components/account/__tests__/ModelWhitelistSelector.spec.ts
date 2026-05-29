import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

const {
  syncUpstreamModels,
  syncUpstreamModelsPreview,
  showError,
  showInfo,
  showSuccess
} = vi.hoisted(() => ({
  syncUpstreamModels: vi.fn(),
  syncUpstreamModelsPreview: vi.fn(),
  showError: vi.fn(),
  showInfo: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin/accounts', () => ({
  accountsAPI: {
    syncUpstreamModels,
    syncUpstreamModelsPreview
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showInfo,
    showSuccess
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

function mountSelector(props: Record<string, unknown> = {}) {
  return mount(ModelWhitelistSelector, {
    props: {
      modelValue: [],
      platform: 'openai',
      ...props
    },
    global: {
      stubs: {
        ModelIcon: true,
        Icon: true
      }
    }
  })
}

describe('ModelWhitelistSelector', () => {
  beforeEach(() => {
    syncUpstreamModels.mockReset()
    syncUpstreamModelsPreview.mockReset()
    showError.mockReset()
    showInfo.mockReset()
    showSuccess.mockReset()
  })

  it('创建账号时使用临时凭证同步上游模型', async () => {
    syncUpstreamModelsPreview.mockResolvedValue({ models: ['gpt-5.1', 'o3', 'gpt-5.1'] })
    const syncCredentials = {
      platform: 'openai',
      type: 'apikey',
      base_url: 'https://openai.example.com/v1',
      api_key: 'openai-key'
    }
    const wrapper = mountSelector({ syncCredentials })

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.accounts.syncUpstreamModels'))
    expect(button).toBeTruthy()
    await button!.trigger('click')

    expect(syncUpstreamModelsPreview).toHaveBeenCalledWith(syncCredentials)
    expect(syncUpstreamModels).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual(['gpt-5.1', 'o3'])
    expect(showSuccess).toHaveBeenCalled()
  })

  it('编辑账号时仍使用账号 ID 同步上游模型', async () => {
    syncUpstreamModels.mockResolvedValue({ models: ['claude-sonnet-4-5'] })
    const wrapper = mountSelector({
      platform: 'anthropic',
      accountId: 7,
      syncCredentials: {
        platform: 'anthropic',
        type: 'apikey',
        api_key: 'should-not-use'
      }
    })

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.accounts.syncUpstreamModels'))
    expect(button).toBeTruthy()
    await button!.trigger('click')

    expect(syncUpstreamModels).toHaveBeenCalledWith(7)
    expect(syncUpstreamModelsPreview).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual(['claude-sonnet-4-5'])
  })
})
