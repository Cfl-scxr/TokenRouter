import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('@/composables/useBalanceDisplay', () => ({
  useBalanceDisplay: () => ({
    formatBalanceAmount: (value: number | null | undefined) => String(value ?? ''),
  }),
}))

import SubscriptionPlanCard from '../SubscriptionPlanCard.vue'

const mountPlanCard = (groupPlatform: string) =>
  mount(SubscriptionPlanCard, {
    props: {
      plan: {
        id: 1,
        group_id: 10,
        group_platform: groupPlatform,
        name: 'Pro',
        description: '',
        price: 10,
        validity_days: 30,
        validity_unit: 'day',
        features: [],
        for_sale: true,
        sort_order: 1,
        supported_model_scopes: ['claude', 'gemini_text', 'gemini_image'],
      },
    },
  })

describe('SubscriptionPlanCard', () => {
  it('does not show Antigravity model scopes for OpenAI plans', () => {
    const text = mountPlanCard('openai').text()

    expect(text).not.toContain('Claude')
    expect(text).not.toContain('Gemini')
    expect(text).not.toContain('Imagen')
  })

  it('shows model scopes for Antigravity plans', () => {
    const text = mountPlanCard('antigravity').text()

    expect(text).toContain('Claude')
    expect(text).toContain('Gemini')
    expect(text).toContain('Imagen')
  })
})
