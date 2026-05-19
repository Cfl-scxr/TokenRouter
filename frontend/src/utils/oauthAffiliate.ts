const OAUTH_AFFILIATE_REFERRAL_KEY = 'oauth_affiliate_referral_code'
const EMAIL_OAUTH_PENDING_REFERRAL_KEY = 'email_oauth_referral_code'

// OAuth 跳转会离开前端页面，返利码需要暂存在 sessionStorage 中跨回调读取。
export function firstOAuthAffiliateQueryValue(value: unknown): string {
  if (typeof value === 'string') {
    return value.trim()
  }
  if (Array.isArray(value)) {
    const item = value.find((entry) => typeof entry === 'string' && entry.trim())
    return typeof item === 'string' ? item.trim() : ''
  }
  return ''
}

export function resolveAffiliateReferralCode(...values: unknown[]): string {
  for (const value of values) {
    const code = firstOAuthAffiliateQueryValue(value)
    if (code) {
      return code
    }
  }
  return ''
}

export function storeOAuthAffiliateCode(code: string): void {
  if (typeof window === 'undefined') {
    return
  }

  const normalized = code.trim()
  if (normalized) {
    window.sessionStorage.setItem(OAUTH_AFFILIATE_REFERRAL_KEY, normalized)
    return
  }
  window.sessionStorage.removeItem(OAUTH_AFFILIATE_REFERRAL_KEY)
}

export function loadOAuthAffiliateCode(): string {
  if (typeof window === 'undefined') {
    return ''
  }
  return window.sessionStorage.getItem(OAUTH_AFFILIATE_REFERRAL_KEY)?.trim() || ''
}

export function oauthAffiliatePayload(code: string): { aff_code?: string } {
  const normalized = code.trim()
  return normalized ? { aff_code: normalized } : {}
}

export function clearAllAffiliateReferralCodes(): void {
  if (typeof window === 'undefined') {
    return
  }
  window.sessionStorage.removeItem(OAUTH_AFFILIATE_REFERRAL_KEY)
  window.sessionStorage.removeItem(EMAIL_OAUTH_PENDING_REFERRAL_KEY)
}
