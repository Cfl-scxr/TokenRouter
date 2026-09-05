<template>
  <div class="mt-3 border-t border-gray-200 pt-3 dark:border-dark-700" data-testid="group-business-usage">
    <div class="mb-2 flex flex-wrap items-center justify-between gap-1 text-[11px] text-gray-500 dark:text-dark-400">
      <span>{{ t('marketplace.businessUsageTitle') }}</span>
      <span v-if="updatedAt" :title="new Date(updatedAt).toLocaleString(locale)">{{ new Date(updatedAt).toLocaleTimeString(locale, { hour12: false }) }}</span>
    </div>
    <p v-if="error" class="mb-2 text-xs text-amber-700 dark:text-amber-300" role="status">{{ t(stats ? 'marketplace.businessUsageStale' : 'marketplace.businessUsageError') }}</p>
    <p v-else-if="!stats" class="text-xs text-gray-500" role="status">{{ t('common.loading') }}</p>
    <template v-if="stats">
      <p v-if="stats.total_requests === 0" class="mb-2 text-xs text-gray-500">{{ t('marketplace.businessUsageEmpty') }}</p>
      <dl class="grid grid-cols-3 gap-x-3 gap-y-2 text-xs sm:grid-cols-6">
        <div v-for="metric in metrics" :key="metric.key" class="min-w-0" :title="metric.hint">
          <dt class="break-words text-[11px] text-gray-500 dark:text-dark-400">{{ t(`marketplace.${metric.key}`) }}</dt>
          <dd class="mt-1 break-words font-semibold tabular-nums text-gray-900 dark:text-white">{{ metric.value }}</dd>
        </div>
      </dl>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminUsageStatsResponse } from '@/api/admin/usage'

const props = defineProps<{ stats?: AdminUsageStatsResponse; error?: boolean; updatedAt?: string }>()
const { t, locale } = useI18n()
const valid = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value) && value >= 0
const metrics = computed(() => {
  const stats = props.stats
  if (!stats) return []
  const counts = [
    ['businessUsageRequests', stats.total_requests],
    ['businessUsageInput', stats.total_input_tokens],
    ['businessUsageOutput', stats.total_output_tokens],
    ['businessUsageCacheRead', stats.total_cache_read_tokens],
    ['businessUsageCacheWrite', stats.total_cache_creation_tokens],
  ] as const
  // 输入字段已扣除缓存，命中率以所有输入 Token 为分母，不能用请求数计算。
  const inputs = [stats.total_input_tokens, stats.total_cache_read_tokens, stats.total_cache_creation_tokens]
  const denominator = inputs.every(valid) ? inputs.reduce((sum, value) => sum + value, 0) : 0
  return [
    ...counts.map(([key, value]) => ({ key,
      value: valid(value) ? new Intl.NumberFormat(locale.value, { notation: 'compact', maximumFractionDigits: 1 }).format(value) : '-',
      hint: valid(value) ? value.toLocaleString(locale.value) : t('marketplace.businessUsageMissing'),
    })),
    { key: 'businessUsageCacheRate', value: denominator > 0 ? `${(stats.total_cache_read_tokens / denominator * 100).toFixed(1)}%` : '-', hint: t('marketplace.businessUsageCacheRateHint') },
  ]
})
</script>
