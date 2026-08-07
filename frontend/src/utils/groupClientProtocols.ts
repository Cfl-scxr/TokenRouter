import type { GroupClientProtocol, GroupPlatform } from '@/types'

export const GROUP_CLIENT_PROTOCOL_ORDER: readonly GroupClientProtocol[] = [
  'anthropic_messages',
  'openai_responses',
  'openai_chat_completions',
  'gemini_generate_content'
]

interface GroupClientProtocolPolicy {
  supported: readonly GroupClientProtocol[]
  required: readonly GroupClientProtocol[]
  defaults: readonly GroupClientProtocol[]
  legacy: readonly GroupClientProtocol[]
}

const GROUP_CLIENT_PROTOCOL_POLICIES: Record<GroupPlatform, GroupClientProtocolPolicy> = {
  anthropic: {
    supported: ['anthropic_messages', 'openai_responses', 'openai_chat_completions'],
    required: ['anthropic_messages'],
    defaults: ['anthropic_messages'],
    legacy: ['anthropic_messages', 'openai_responses', 'openai_chat_completions']
  },
  openai: {
    supported: ['anthropic_messages', 'openai_responses', 'openai_chat_completions'],
    required: ['openai_responses', 'openai_chat_completions'],
    defaults: ['openai_responses', 'openai_chat_completions'],
    legacy: ['openai_responses', 'openai_chat_completions']
  },
  gemini: {
    supported: GROUP_CLIENT_PROTOCOL_ORDER,
    required: ['gemini_generate_content'],
    defaults: ['gemini_generate_content'],
    legacy: GROUP_CLIENT_PROTOCOL_ORDER
  },
  antigravity: {
    supported: GROUP_CLIENT_PROTOCOL_ORDER,
    required: ['anthropic_messages', 'gemini_generate_content'],
    defaults: ['anthropic_messages', 'gemini_generate_content'],
    legacy: GROUP_CLIENT_PROTOCOL_ORDER
  },
  qoder: {
    supported: ['anthropic_messages', 'openai_responses', 'openai_chat_completions'],
    required: [],
    defaults: [],
    legacy: ['anthropic_messages', 'openai_responses', 'openai_chat_completions']
  },
  grok: {
    supported: ['anthropic_messages', 'openai_responses', 'openai_chat_completions'],
    required: ['openai_responses', 'openai_chat_completions'],
    defaults: ['openai_responses', 'openai_chat_completions'],
    legacy: ['anthropic_messages', 'openai_responses', 'openai_chat_completions']
  }
}

function orderedProtocols(protocols: Iterable<GroupClientProtocol>): GroupClientProtocol[] {
  const selected = new Set(protocols)
  return GROUP_CLIENT_PROTOCOL_ORDER.filter((protocol) => selected.has(protocol))
}

export function supportedGroupClientProtocols(platform: GroupPlatform): GroupClientProtocol[] {
  return orderedProtocols(GROUP_CLIENT_PROTOCOL_POLICIES[platform].supported)
}

export function requiredGroupClientProtocols(platform: GroupPlatform): GroupClientProtocol[] {
  return orderedProtocols(GROUP_CLIENT_PROTOCOL_POLICIES[platform].required)
}

export function defaultGroupClientProtocols(platform: GroupPlatform): GroupClientProtocol[] {
  return orderedProtocols(GROUP_CLIENT_PROTOCOL_POLICIES[platform].defaults)
}

export function legacyGroupClientProtocols(
  platform: GroupPlatform,
  allowMessagesDispatch = false
): GroupClientProtocol[] {
  const protocols = [...GROUP_CLIENT_PROTOCOL_POLICIES[platform].legacy]
  if (platform === 'openai' && allowMessagesDispatch) {
    protocols.push('anthropic_messages')
  }
  return orderedProtocols(protocols)
}

// 旧快照缺少字段时按迁移矩阵恢复；显式空数组必须原样保留。
export function effectiveGroupClientProtocols(
  platform: GroupPlatform,
  protocols: GroupClientProtocol[] | null | undefined,
  allowMessagesDispatch = false
): GroupClientProtocol[] {
  if (protocols == null) {
    return legacyGroupClientProtocols(platform, allowMessagesDispatch)
  }

  const supported = new Set(GROUP_CLIENT_PROTOCOL_POLICIES[platform].supported)
  const required = GROUP_CLIENT_PROTOCOL_POLICIES[platform].required
  return orderedProtocols([
    ...protocols.filter((protocol) => supported.has(protocol)),
    ...required
  ])
}

export function hasGroupClientProtocol(
  protocols: readonly GroupClientProtocol[],
  protocol: GroupClientProtocol
): boolean {
  return protocols.includes(protocol)
}

export function setGroupClientProtocol(
  platform: GroupPlatform,
  protocols: readonly GroupClientProtocol[],
  protocol: GroupClientProtocol,
  enabled: boolean
): GroupClientProtocol[] {
  const policy = GROUP_CLIENT_PROTOCOL_POLICIES[platform]
  if (!policy.supported.includes(protocol) || (!enabled && policy.required.includes(protocol))) {
    return effectiveGroupClientProtocols(platform, [...protocols])
  }

  const next = new Set(protocols)
  if (enabled) {
    next.add(protocol)
  } else {
    next.delete(protocol)
  }
  return effectiveGroupClientProtocols(platform, [...next])
}
