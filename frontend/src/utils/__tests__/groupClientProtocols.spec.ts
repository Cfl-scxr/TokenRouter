import { describe, expect, it } from 'vitest'
import type { GroupPlatform } from '@/types'
import {
  defaultGroupClientProtocols,
  effectiveGroupClientProtocols,
  requiredGroupClientProtocols,
  setGroupClientProtocol,
  supportedGroupClientProtocols
} from '../groupClientProtocols'

describe('groupClientProtocols', () => {
  it.each<[GroupPlatform, string[], string[], string[]]>([
    ['anthropic', ['anthropic_messages', 'openai_responses', 'openai_chat_completions'], ['anthropic_messages'], ['anthropic_messages']],
    ['openai', ['anthropic_messages', 'openai_responses', 'openai_chat_completions'], ['openai_responses', 'openai_chat_completions'], ['openai_responses', 'openai_chat_completions']],
    ['gemini', ['anthropic_messages', 'openai_responses', 'openai_chat_completions', 'gemini_generate_content'], ['gemini_generate_content'], ['gemini_generate_content']],
    ['antigravity', ['anthropic_messages', 'openai_responses', 'openai_chat_completions', 'gemini_generate_content'], ['anthropic_messages', 'gemini_generate_content'], ['anthropic_messages', 'gemini_generate_content']],
    ['qoder', ['anthropic_messages', 'openai_responses', 'openai_chat_completions'], [], []],
    ['grok', ['anthropic_messages', 'openai_responses', 'openai_chat_completions'], ['openai_responses', 'openai_chat_completions'], ['openai_responses', 'openai_chat_completions']]
  ])('returns the %s protocol policy', (platform, supported, required, defaults) => {
    expect(supportedGroupClientProtocols(platform)).toEqual(supported)
    expect(requiredGroupClientProtocols(platform)).toEqual(required)
    expect(defaultGroupClientProtocols(platform)).toEqual(defaults)
  })

  it('distinguishes a missing old snapshot from a Qoder explicit empty list', () => {
    expect(effectiveGroupClientProtocols('qoder', undefined)).toEqual([
      'anthropic_messages',
      'openai_responses',
      'openai_chat_completions'
    ])
    expect(effectiveGroupClientProtocols('qoder', [])).toEqual([])
  })

  it('derives the OpenAI Messages protocol from the legacy field only when needed', () => {
    expect(effectiveGroupClientProtocols('openai', undefined, true)).toEqual([
      'anthropic_messages',
      'openai_responses',
      'openai_chat_completions'
    ])
    expect(effectiveGroupClientProtocols('openai', ['openai_responses', 'openai_chat_completions'], true)).toEqual([
      'openai_responses',
      'openai_chat_completions'
    ])
  })

  it('keeps required protocols enabled while optional protocols can be toggled', () => {
    expect(setGroupClientProtocol('openai', ['openai_responses', 'openai_chat_completions'], 'openai_responses', false)).toEqual([
      'openai_responses',
      'openai_chat_completions'
    ])
    expect(setGroupClientProtocol('openai', ['openai_responses', 'openai_chat_completions'], 'anthropic_messages', true)).toEqual([
      'anthropic_messages',
      'openai_responses',
      'openai_chat_completions'
    ])
  })
})
