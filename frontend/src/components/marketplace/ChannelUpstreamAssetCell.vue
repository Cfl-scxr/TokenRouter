<template>
  <div class="space-y-1" data-testid="channel-upstream-asset" :title="detailTitle">
    <template v-if="visibleAssets.length > 0">
      <div v-for="asset in visibleAssets" :key="asset.accountId" class="space-y-0.5">
        <div v-if="showAccountNames" class="truncate text-[10px] text-gray-400 dark:text-dark-500">
          {{ asset.accountName }}
        </div>
        <div v-for="line in assetLines(asset)" :key="line" class="break-words leading-5 text-gray-700 dark:text-dark-200">
          {{ line }}
        </div>
      </div>
      <div v-if="hiddenAssetCount > 0" class="text-[10px] text-gray-400 dark:text-dark-500">
        {{ t('marketplace.routingHealthMoreAccounts', { count: hiddenAssetCount }) }}
      </div>
    </template>
    <span v-else class="text-gray-400 dark:text-dark-500">-</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UpstreamUsageInfo, UpstreamUsageLimit, UpstreamUsageQueryResult } from '@/types'
import type { ChannelUpstreamAsset } from './channelUpstreamAsset'

const props = withDefaults(defineProps<{
  assets?: ChannelUpstreamAsset[]
}>(), {
  assets: () => [],
})

const { t } = useI18n()

const assetsWithData = computed(() => props.assets.filter(asset => assetLines(asset).length > 0))
const visibleAssets = computed(() => assetsWithData.value.slice(0, 2))
const hiddenAssetCount = computed(() => Math.max(assetsWithData.value.length - visibleAssets.value.length, 0))
const showAccountNames = computed(() => assetsWithData.value.length > 1)

function normalizedUsage(result?: UpstreamUsageQueryResult | null): UpstreamUsageInfo | null {
  if (!result) return null
  return result.usage ?? {
    provider: result.provider || '',
    mode: result.mode || '',
    unit: result.unit,
    balance: result.balance,
    balances: result.balances,
    available: result.available,
    limits: result.limits,
    subscription: result.subscription,
    expires_at: result.expires_at,
  }
}

function compactNumber(value: number): string {
  const absolute = Math.abs(value)
  if (absolute >= 1_000_000_000) return `${trimFixed(value / 1_000_000_000, 2)}B`
  if (absolute >= 1_000_000) return `${trimFixed(value / 1_000_000, 2)}M`
  if (absolute >= 1_000) return `${trimFixed(value / 1_000, 2)}K`
  return new Intl.NumberFormat(undefined, {
    maximumFractionDigits: absolute > 0 && absolute < 0.01 ? 4 : 2,
  }).format(value)
}

function trimFixed(value: number, digits: number): string {
  return value.toFixed(digits).replace(/\.?0+$/, '')
}

function formatAmount(value: number, unit?: string): string {
  if (unit === 'USD') return `$${compactNumber(value)}`
  if (unit === 'CNY') return `¥${compactNumber(value)}`
  if (unit === 'PERCENT') return `${compactNumber(value)}%`
  if (unit === 'TOKENS') return `${compactNumber(value)} Token`
  return unit ? `${compactNumber(value)} ${unit}` : compactNumber(value)
}

function balanceLine(usage: UpstreamUsageInfo | null): string {
  if (!usage) return ''
  if (usage.balances?.length) {
    return t('marketplace.routingHealthUpstreamBalance', {
      value: usage.balances
        .slice(0, 2)
        .map(entry => formatAmount(entry.remaining, entry.currency))
        .join(' · '),
    })
  }
  if (typeof usage.balance?.remaining !== 'number' || !Number.isFinite(usage.balance.remaining)) return ''
  return t('marketplace.routingHealthUpstreamBalance', {
    value: formatAmount(usage.balance.remaining, usage.unit),
  })
}

function firstLimit(usage: UpstreamUsageInfo | null): UpstreamUsageLimit | null {
  return usage?.limits?.[0] ?? usage?.subscription?.limits?.[0] ?? null
}

function quotaLine(usage: UpstreamUsageInfo | null): string {
  if (!usage) return ''
  if (usage.subscription?.unlimited) return t('marketplace.routingHealthUpstreamUnlimited')

  const limit = firstLimit(usage)
  if (limit) {
    const remaining = typeof limit.remaining === 'number'
      ? limit.remaining
      : typeof limit.limit === 'number' && typeof limit.used === 'number'
        ? limit.limit - limit.used
        : null
    if (remaining != null && Number.isFinite(remaining) && typeof limit.limit === 'number' && Number.isFinite(limit.limit) && limit.limit > 0) {
      return t('marketplace.routingHealthUpstreamQuotaPercent', {
        percent: trimFixed(Math.min(100, Math.max(0, remaining / limit.limit * 100)), 1),
      })
    }
    if (remaining != null && Number.isFinite(remaining)) {
      return t('marketplace.routingHealthUpstreamQuotaRemaining', {
        value: formatAmount(remaining, usage.unit),
      })
    }
  }

  const remaining = usage.subscription?.remaining
  if (typeof remaining === 'number' && Number.isFinite(remaining)) {
    return t('marketplace.routingHealthUpstreamQuotaRemaining', {
      value: formatAmount(remaining, usage.unit),
    })
  }
  return ''
}

function multiplierLine(asset: ChannelUpstreamAsset): string {
  const accountMultiplier = asset.rateMultiplier == null ? 1 : asset.rateMultiplier
  const groupMultiplier = asset.groupRateMultiplier == null ? 1 : asset.groupRateMultiplier
  if (!Number.isFinite(accountMultiplier) || accountMultiplier < 0 || !Number.isFinite(groupMultiplier) || groupMultiplier < 0) return ''
  const multiplier = accountMultiplier * groupMultiplier
  return t('marketplace.routingHealthEffectiveMultiplier', {
    multiplier: `x${trimFixed(multiplier, 2)}`,
  })
}

function assetLines(asset: ChannelUpstreamAsset): string[] {
  const usage = normalizedUsage(asset.usage)
  return [balanceLine(usage), quotaLine(usage), multiplierLine(asset)].filter(Boolean)
}

const detailTitle = computed(() => visibleAssets.value.map((asset) => {
  const observedAt = asset.usage?.observed_at
  const observedLabel = observedAt && Number.isFinite(Date.parse(observedAt))
    ? new Date(observedAt).toLocaleString()
    : ''
  return [asset.accountName, ...assetLines(asset), observedLabel].filter(Boolean).join(' · ')
}).join('\n'))
</script>
