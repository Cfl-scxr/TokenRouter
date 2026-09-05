<template>
  <section
    class="overflow-hidden border-y border-gray-200 bg-white/80 dark:border-dark-700 dark:bg-dark-900/70"
    data-testid="routing-health-overview"
  >
    <div class="flex flex-col gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-800 md:flex-row md:items-center md:justify-between">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
          <h2 class="text-sm font-semibold text-gray-950 dark:text-white">
            {{ t('marketplace.routingHealthTitle') }}
          </h2>
          <span class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('marketplace.routingHealthUpdatedAt') }} {{ formatDateTime(snapshot.observedAt) }}
          </span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
          {{ currentHitLabel }}
        </p>
      </div>
      <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs font-medium">
        <span class="text-emerald-700 dark:text-emerald-300">{{ t('marketplace.routingHealthHealthy') }} {{ counts.healthy }}</span>
        <span class="text-amber-700 dark:text-amber-300">{{ t('marketplace.routingHealthDegraded') }} {{ counts.degraded }}</span>
        <span class="text-rose-700 dark:text-rose-300">{{ t('marketplace.routingHealthUnavailable') }} {{ counts.unavailable }}</span>
        <span class="text-gray-600 dark:text-dark-300">{{ t('marketplace.routingHealthManualDisabled') }} {{ counts.manualDisabled }}</span>
      </div>
    </div>

    <div
      v-if="!snapshot.available"
      class="px-4 py-5 text-sm text-amber-700 dark:text-amber-300"
      data-testid="routing-health-load-state"
    >
      {{ loadStateMessage }}
    </div>

    <div v-else class="overflow-x-auto">
      <table class="min-w-[1220px] w-full table-fixed text-left text-xs">
        <thead class="bg-gray-50/90 text-gray-500 dark:bg-dark-950/70 dark:text-dark-400">
          <tr>
            <th class="w-[160px] px-4 py-2 font-medium">{{ t('marketplace.routingHealthChannel') }}</th>
            <th class="w-[100px] px-3 py-2 font-medium">{{ t('marketplace.probeCurrentStatus') }}</th>
            <th class="w-[90px] px-3 py-2 font-medium" :title="t('marketplace.routingHealthScoreHint')">{{ t('marketplace.routingHealthScore') }}</th>
            <th class="w-[115px] px-3 py-2 font-medium">{{ t('marketplace.routingHealthProbe24h') }}</th>
            <th class="w-[145px] px-3 py-2 font-medium">{{ t('marketplace.routingHealthBusinessSuccess') }}</th>
            <th class="w-[105px] px-3 py-2 font-medium">{{ t('marketplace.probeCurrentLatency') }}</th>
            <th class="w-[90px] px-3 py-2 font-medium">{{ t('marketplace.probeConsecutiveFailures') }}</th>
            <th class="w-[110px] px-3 py-2 font-medium">{{ t('marketplace.routingHealthRouteState') }}</th>
            <th class="w-[145px] px-3 py-2 font-medium">{{ t('marketplace.routingHealthNextProbe') }}</th>
            <th class="w-[145px] px-3 py-2 font-medium">{{ t('marketplace.probeLastCheckedAt') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
          <tr
            v-for="provider in snapshot.providers"
            :key="provider.names.group"
            class="text-gray-800 dark:text-dark-100"
            data-testid="routing-health-provider"
          >
            <td class="px-4 py-2.5">
              <div class="truncate font-semibold text-gray-950 dark:text-white">{{ provider.names.group }}</div>
              <div class="truncate text-[11px] text-gray-500 dark:text-dark-400">{{ provider.supplierName }}</div>
            </td>
            <td class="px-3 py-2.5">
              <span class="inline-flex items-center gap-1.5 font-semibold" :class="healthTextClass(currentStatusKey(provider))" :title="healthStatusHint(provider)">
                <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="healthDotClass(currentStatusKey(provider))" />
                {{ healthLabel(currentStatusKey(provider)) }}
              </span>
            </td>
            <td class="px-3 py-2.5 font-semibold tabular-nums" :title="scoreHint(provider)">{{ formatScore(provider.healthScore) }}</td>
            <td class="px-3 py-2.5 tabular-nums">{{ formatProbeSuccess24h(provider) }}</td>
            <td class="px-3 py-2.5 tabular-nums">{{ formatBusinessSuccess(provider) }}</td>
            <td class="px-3 py-2.5 tabular-nums">{{ formatLatency(provider) }}</td>
            <td class="px-3 py-2.5 tabular-nums">{{ Math.max(provider.health.consecutiveFailures || 0, 0) }}</td>
            <td class="px-3 py-2.5 font-medium" :class="routeTextClass(provider.routeState)">{{ routeLabel(provider.routeState) }}</td>
            <td class="px-3 py-2.5 tabular-nums">{{ nextProbeLabel(provider) }}</td>
            <td class="px-3 py-2.5 tabular-nums">{{ formatDateTime(lastProbeAt(provider)) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MarketplaceRoutingHealthProvider, MarketplaceRoutingHealthSnapshot } from '@/types'

const props = defineProps<{
  snapshot: MarketplaceRoutingHealthSnapshot
  loadState?: 'ready' | 'source_unavailable' | 'auth_required' | 'forbidden' | 'network_error' | 'unknown_error'
}>()

const { t, locale } = useI18n()

const loadStateMessage = computed(() => {
  const messages = {
    auth_required: 'marketplace.routingHealthAuthRequired',
    forbidden: 'marketplace.routingHealthForbidden',
    network_error: 'marketplace.routingHealthNetworkError',
    unknown_error: 'marketplace.routingHealthUnknownError',
    source_unavailable: 'marketplace.routingHealthSourceUnavailable',
    ready: 'marketplace.routingHealthSourceUnavailable',
  } as const
  return t(messages[props.loadState ?? 'source_unavailable'])
})

const counts = computed(() => {
  const result = { healthy: 0, degraded: 0, unavailable: 0, manualDisabled: 0 }
  for (const provider of props.snapshot.providers) {
    const status = currentStatusKey(provider)
    if (status === 'healthy') result.healthy += 1
    else if (status === 'degraded' || status === 'recovering') result.degraded += 1
    else if (status === 'manual_disabled') result.manualDisabled += 1
    else result.unavailable += 1
  }
  return result
})

const currentHitLabel = computed(() => {
  const hit = props.snapshot.currentHit
  if (!hit?.supplierName) return t('marketplace.routingHealthNoCurrentHit')
  return t('marketplace.routingHealthCurrentHit', { supplier: hit.supplierName, model: hit.model || '-' })
})

function currentStatusKey(provider: MarketplaceRoutingHealthProvider): string {
  if (!provider.manual.enabled || provider.routeState === 'manual_disabled') return 'manual_disabled'
  if (provider.routeState === 'unavailable' || provider.routeState === 'needs_action') return 'unavailable'
  if (provider.routeState === 'cooldown') {
    return provider.healthLevel === 'needs_action' || provider.health.consecutiveFailures >= 2
      ? 'unavailable'
      : 'degraded'
  }
  if (provider.routeState === 'warming') return 'recovering'
  if (provider.healthLevel === 'needs_action') return 'unavailable'
  if (provider.healthLevel === 'recovering') return 'recovering'
  if (provider.healthLevel === 'degraded') return 'degraded'
  if (provider.healthLevel === 'healthy') return 'healthy'
  return 'unknown'
}

function healthLabel(level: string): string {
  const labels: Record<string, string> = {
    healthy: t('marketplace.routingHealthHealthy'),
    degraded: t('marketplace.routingHealthDegraded'),
    recovering: t('marketplace.routingHealthRecovering'),
    unavailable: t('marketplace.routingHealthUnavailable'),
    manual_disabled: t('marketplace.routingHealthManualDisabled'),
    unknown: t('marketplace.probeStatusUnknown'),
  }
  return labels[level] ?? t('marketplace.probeStatusUnknown')
}

function healthStatusHint(provider: MarketplaceRoutingHealthProvider): string {
  const status = currentStatusKey(provider)
  if (status === 'unknown') return t('marketplace.probeStatusUnknownHint')
  if (status === 'unavailable') return t('marketplace.routingHealthRouteUnavailableHint')
  if (provider.routeState === 'cooldown') return t('marketplace.routingHealthRouteCooldownHint')
  if (provider.routeState === 'warming') return t('marketplace.routingHealthRouteWarmingHint')
  return healthLabel(status)
}

function healthTextClass(level: string): string {
  if (level === 'healthy') return 'text-emerald-700 dark:text-emerald-300'
  if (level === 'degraded' || level === 'recovering') return 'text-amber-700 dark:text-amber-300'
  if (level === 'manual_disabled') return 'text-gray-600 dark:text-dark-300'
  if (level === 'unknown') return 'text-gray-500 dark:text-dark-400'
  return 'text-rose-700 dark:text-rose-300'
}

function healthDotClass(level: string): string {
  if (level === 'healthy') return 'bg-emerald-500'
  if (level === 'degraded' || level === 'recovering') return 'bg-amber-400'
  if (level === 'manual_disabled') return 'bg-gray-400 dark:bg-dark-500'
  if (level === 'unknown') return 'bg-gray-400 dark:bg-dark-500'
  return 'bg-rose-500'
}

function routeLabel(state: string): string {
  const labels: Record<string, string> = {
    available: t('marketplace.routingHealthRouteAvailable'),
    cooldown: t('marketplace.routingHealthRouteCooldown'),
    warming: t('marketplace.routingHealthRouteWarming'),
    unavailable: t('marketplace.routingHealthRouteUnavailable'),
    manual_disabled: t('marketplace.routingHealthManualDisabled'),
  }
  return labels[state] ?? t('marketplace.probeStatusUnknown')
}

function routeTextClass(state: string): string {
  if (state === 'available') return 'text-emerald-700 dark:text-emerald-300'
  if (state === 'cooldown' || state === 'warming') return 'text-amber-700 dark:text-amber-300'
  if (state === 'manual_disabled') return 'text-gray-600 dark:text-dark-300'
  return 'text-rose-700 dark:text-rose-300'
}

function formatScore(score?: number | null): string {
  return typeof score === 'number' ? `${Math.round(score)}` : '-'
}

function scoreHint(provider: MarketplaceRoutingHealthProvider): string {
  if (typeof provider.healthScore !== 'number') return t('marketplace.routingHealthScoreNoData')
  return t('marketplace.routingHealthScoreHint')
}

function formatBusinessSuccess(provider: MarketplaceRoutingHealthProvider): string {
  const rate = provider.business.successRate
  if (typeof rate !== 'number') return t('marketplace.routingHealthNoBusinessSample')
  return `${(rate * 100).toFixed(1)}% (${provider.business.success}/${provider.business.total})`
}

function formatProbeSuccess24h(provider: MarketplaceRoutingHealthProvider): string {
  const rate = provider.scheduledTest?.successRate24h
  const samples = provider.scheduledTest?.sampleCount24h ?? 0
  if (typeof rate !== 'number' || samples <= 0) return '-'
  return `${(rate * 100).toFixed(1)}% (${samples})`
}

function formatLatency(provider: MarketplaceRoutingHealthProvider): string {
  const latency = provider.health.lastLatencyMs ?? provider.scheduledTest?.latencyMs
  if (typeof latency !== 'number' || latency < 0) return '-'
  return latency >= 1000 ? `${(latency / 1000).toFixed(2)} s` : `${Math.round(latency)} ms`
}

function lastProbeAt(provider: MarketplaceRoutingHealthProvider): string | null | undefined {
  const candidates = [provider.availabilityProbe?.observedAt, provider.scheduledTest?.observedAt]
    .filter((value): value is string => Boolean(value))
    .sort((left, right) => new Date(right).getTime() - new Date(left).getTime())
  return candidates[0]
}

function nextProbeLabel(provider: MarketplaceRoutingHealthProvider): string {
  if (!provider.manual.enabled || provider.routeState === 'manual_disabled') {
    return t('marketplace.routingHealthNoAutoReturn')
  }
  return formatDateTime(provider.availabilityProbe?.nextProbeAt)
}

function formatDateTime(raw?: string | null): string {
  if (!raw) return '-'
  const value = new Date(raw)
  if (Number.isNaN(value.getTime())) return '-'
  return value.toLocaleString(locale.value, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
}
</script>
