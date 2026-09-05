import type { UpstreamUsageQueryResult } from '@/types'

export interface ChannelUpstreamAsset {
  accountId: number
  accountName: string
  rateMultiplier?: number
  groupRateMultiplier?: number
  usage?: UpstreamUsageQueryResult | null
}
