import type { UpstreamUsageQueryResult } from '@/types'

export interface ChannelUpstreamAsset {
  accountId: number
  accountName: string
  rateMultiplier?: number
  usage?: UpstreamUsageQueryResult | null
}
