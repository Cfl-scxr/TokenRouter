<template>
  <div class="space-y-1" data-testid="account-upstream-usage">
    <div v-if="!queryEnabled" class="flex min-h-5 items-center justify-end gap-1">
      <span class="text-[9px] text-gray-400 dark:text-gray-500">
        {{ t('admin.accounts.upstreamUsage.disabled') }}
      </span>
    </div>
    <div v-else-if="loading || error || result" class="flex items-center justify-between gap-2">
      <span class="text-[9px] font-medium uppercase text-sky-600 dark:text-sky-400">
        {{ t('admin.accounts.upstreamUsage.source') }}
      </span>
      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[9px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="loading || !queryEnabled"
        :title="t('admin.accounts.upstreamUsage.query')"
        :aria-label="t('admin.accounts.upstreamUsage.query')"
        @click="query(true)"
      >
        <Icon name="refresh" size="xs" :class="{ 'animate-spin': loading }" :stroke-width="2" />
      </button>
    </div>
    <div v-else class="flex min-h-5 items-center justify-end gap-1">
      <button
        type="button"
        class="inline-flex items-center rounded px-1 py-0.5 text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :title="t('admin.accounts.upstreamUsage.query')"
        :aria-label="t('admin.accounts.upstreamUsage.query')"
        @click="query(true)"
      >
        <Icon name="refresh" size="xs" :stroke-width="2" />
      </button>
    </div>

    <div v-if="queryEnabled && loading" class="space-y-1">
      <div class="h-3 w-28 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-36 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    </div>
    <div v-else-if="queryEnabled && error" class="flex items-center gap-1 text-[10px] text-amber-600 dark:text-amber-400">
      <span class="truncate" :title="error.message || error.code || ''">
        {{ errorLabel }}
      </span>
    </div>
    <div v-else-if="queryEnabled && normalizedUsage" class="space-y-1">
      <div v-if="balanceLabel" class="text-[10px] text-gray-600 dark:text-gray-300">
        {{ balanceLabel }}
      </div>
      <div v-for="limit in visibleLimits" :key="limit.name" class="space-y-0.5">
        <UsageProgressBar
          :label="limit.name"
          :utilization="limit.utilization"
          :resets-at="limit.resetAt"
          color="indigo"
        />
        <div v-if="limit.hasAmount" class="pl-9 text-[9px] text-gray-400 dark:text-gray-500">
          {{ formatAmount(limit.remaining, normalizedUsage?.unit) }} /
          {{ formatAmount(limit.limit, normalizedUsage?.unit) }}
        </div>
      </div>
      <div v-if="subscriptionLabel || subscriptionExpiry" class="flex flex-wrap items-center gap-1 text-[10px] text-gray-500 dark:text-gray-400">
        <span v-if="subscriptionLabel" class="rounded bg-sky-50 px-1.5 py-0.5 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300">
          {{ subscriptionLabel }}
        </span>
        <span v-if="subscriptionRemainingLabel">{{ subscriptionRemainingLabel }}</span>
        <span v-if="subscriptionExpiry">{{ subscriptionExpiry }}</span>
      </div>
      <div class="text-[9px] text-gray-400 dark:text-gray-500" :title="result?.observed_at || ''">
        {{ t('admin.accounts.upstreamUsage.observedAt') }} {{ formatObservedAt(result?.observed_at || '') }}
      </div>
    </div>
    <!-- 无查询结果时由外层账号用量单元格提供统一占位符。 -->
    <div v-else class="h-3" aria-hidden="true"></div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, UpstreamUsageQueryError, UpstreamUsageQueryResult } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import UsageProgressBar from './UsageProgressBar.vue'

const props = withDefaults(defineProps<{
  account: Account
  result?: UpstreamUsageQueryResult | null
  error?: UpstreamUsageQueryError | null
  loading?: boolean
  request?: ((account: Account, options?: { force?: boolean }) => void) | null
}>(), {
  result: null,
  error: null,
  loading: false,
  request: null
})

const { t } = useI18n()

// 查询按钮只负责发出管理员显式操作，组件挂载和滚动不会触发请求。
const queryEnabled = computed(() => {
  const config = props.account.extra?.upstream_usage_query as Record<string, unknown> | undefined
  return config?.enabled !== false
})

const formatNumber = (value: number | undefined) => {
  if (value == null || !Number.isFinite(value)) return '-'
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 4 }).format(value)
}

const formatAmount = (value: number | undefined, unit?: string) => {
  if (value == null) return '-'
  const suffix = unit ? ` ${unit}` : ''
  return `${formatNumber(value)}${suffix}`
}

const normalizedUsage = computed(() => {
  const result = props.result
  if (!result) return null
  return result.usage ?? {
    provider: result.provider || '',
    mode: result.mode || '',
    unit: result.unit,
    balance: result.balance,
    limits: result.limits,
    subscription: result.subscription,
    expires_at: result.expires_at
  }
})

const balanceLabel = computed(() => {
  const usage = normalizedUsage.value
  if (!usage?.balance) return ''
  const balance = usage.balance
  const unit = usage.unit
  if (balance.total != null || balance.used != null) {
    return t('admin.accounts.upstreamUsage.balanceLine', {
      remaining: formatAmount(balance.remaining, unit),
      used: formatAmount(balance.used, unit),
      total: formatAmount(balance.total, unit)
    })
  }
  return t('admin.accounts.upstreamUsage.remainingLine', {
    remaining: formatAmount(balance.remaining, unit)
  })
})

const allLimits = computed(() => {
  const usage = normalizedUsage.value
  const limits = [...(usage?.limits ?? [])]
  if (usage?.subscription?.limits) limits.push(...usage.subscription.limits)
  return limits
})

const visibleLimits = computed(() => allLimits.value.map(limit => ({
  name: limit.name,
  used: limit.used,
  limit: limit.limit,
  remaining: limit.remaining,
  hasAmount: limit.used != null || limit.limit != null || limit.remaining != null,
  utilization: limit.limit && limit.limit > 0 && limit.used != null
    ? Math.min(100, Math.max(0, limit.used / limit.limit * 100))
    : 0,
  resetAt: limit.reset_at ?? null
})))

const subscriptionLabel = computed(() => {
  const subscription = normalizedUsage.value?.subscription
  if (!subscription) return ''
  if (subscription.unlimited) return t('admin.accounts.upstreamUsage.unlimited', { plan: subscription.plan_name })
  return subscription.plan_name
})

const subscriptionRemainingLabel = computed(() => {
  const subscription = normalizedUsage.value?.subscription
  if (!subscription || subscription.unlimited || subscription.remaining == null) return ''
  return t('admin.accounts.upstreamUsage.subscriptionRemaining', {
    remaining: formatAmount(subscription.remaining, normalizedUsage.value?.unit)
  })
})

const subscriptionExpiry = computed(() => {
  const expiresAt = normalizedUsage.value?.subscription?.expires_at || normalizedUsage.value?.expires_at
  if (!expiresAt) return ''
  return t('admin.accounts.upstreamUsage.expiresAt', { time: formatObservedAt(expiresAt) })
})

const errorLabel = computed(() => {
  const code = props.error?.code
  if (code) return t(`admin.accounts.upstreamUsage.errors.${code}`, code)
  return props.error?.message || t('admin.accounts.upstreamUsage.queryFailed')
})

const formatObservedAt = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

const query = (force: boolean) => {
  props.request?.(props.account, { force })
}
</script>
