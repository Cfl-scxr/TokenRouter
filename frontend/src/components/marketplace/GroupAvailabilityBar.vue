<template>
  <div class="probe-overview w-full min-w-0 space-y-3">
    <div class="probe-fields grid grid-cols-2 gap-x-4 gap-y-3 border-b border-gray-100 pb-3 dark:border-dark-700">
      <div class="min-w-0">
        <div class="text-[10px] leading-4 text-gray-500 dark:text-dark-400">
          {{ t('marketplace.probeCurrentStatus') }}
        </div>
        <div
          class="mt-0.5 flex items-center gap-1.5 text-xs font-semibold"
          :class="statusTextClass"
          data-testid="probe-current-status"
        >
          <span class="h-1.5 w-1.5 rounded-full" :class="statusDotClass" />
          {{ statusLabel }}
        </div>
      </div>
      <div class="min-w-0">
        <div class="text-[10px] leading-4 text-gray-500 dark:text-dark-400">
          {{ t('marketplace.probeLastCheckedAt') }}
        </div>
        <div class="mt-0.5 break-words text-xs font-semibold text-gray-800 dark:text-dark-100" :title="lastCheckedAbsolute" data-testid="probe-last-checked-at">
          {{ lastCheckedLabel }}
        </div>
      </div>
      <div class="min-w-0">
        <div class="text-[10px] leading-4 text-gray-500 dark:text-dark-400">
          {{ t('marketplace.probeCurrentLatency') }}
        </div>
        <div class="mt-0.5 text-xs font-semibold tabular-nums" :class="isSlowResponse ? 'text-amber-700 dark:text-amber-300' : 'text-gray-800 dark:text-dark-100'" data-testid="probe-current-latency">
          {{ latencyLabel }}
        </div>
      </div>
      <div class="min-w-0">
        <div class="text-[10px] leading-4 text-gray-500 dark:text-dark-400">
          {{ t('marketplace.probeConsecutiveFailures') }}
        </div>
        <div class="mt-0.5 text-xs font-semibold tabular-nums" :class="Number(consecutiveFailuresLabel) >= 2 ? 'text-rose-700 dark:text-rose-300' : Number(consecutiveFailuresLabel) > 0 ? 'text-amber-700 dark:text-amber-300' : 'text-gray-500 dark:text-dark-400'" data-testid="probe-consecutive-failures">
          {{ consecutiveFailuresLabel }}
        </div>
      </div>
    </div>

    <div class="flex min-w-0 items-center gap-2">
      <div
        class="grid h-7 min-w-0 flex-1 items-center overflow-hidden"
        :style="barGridStyle"
        role="img"
        :aria-label="ariaLabel"
        data-testid="probe-history-bar"
      >
        <span
          v-for="(bucket, index) in normalizedBuckets"
          :key="`${bucket.date || 'empty'}-${index}`"
          :class="[
            'h-5 max-w-full justify-self-center rounded-[2px]',
            bucketClass(bucket.availability_rate, bucket.total_count),
          ]"
          :style="{ width: bucketWidth }"
          :title="bucketTitle(bucket)"
        />
      </div>
      <div class="w-[76px] shrink-0 text-right" :title="tooltip">
        <div class="text-sm font-semibold leading-5 text-gray-900 dark:text-white">
          {{ rateLabel }}
        </div>
        <div class="text-[10px] leading-4 text-gray-500 dark:text-dark-400">
          {{ t('marketplace.availabilityWindowHours', { hours: windowHours }) }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useNow } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import type { MarketplaceGroupAvailability, MarketplaceGroupAvailabilityDay } from '@/types'

const props = defineProps<{
  availability?: MarketplaceGroupAvailability | null
  currentStatus?: string
}>()

const { t, locale } = useI18n()
const now = useNow({ interval: 30_000 })
const slowResponseThresholdMs = 8_000

const windowDays = computed(() => Math.max(props.availability?.window_days ?? 1, 1))
const windowHours = computed(() => windowDays.value * 24)
const bucketMinutes = computed(() => Math.max(props.availability?.bucket_minutes ?? 15, 1))
const targetBucketCount = computed(() =>
  Math.max(Math.ceil((windowDays.value * 24 * 60) / bucketMinutes.value), 1),
)

const normalizedBuckets = computed<MarketplaceGroupAvailabilityDay[]>(() => {
  const buckets = props.availability?.days ?? []
  const target = targetBucketCount.value
  if (buckets.length >= target) {
    return buckets.slice(buckets.length - target)
  }
  return [
    ...Array.from({ length: target - buckets.length }, () => ({
      date: '',
      success_count: 0,
      total_count: 0,
      availability_rate: null,
    })),
    ...buckets,
  ]
})

const barGridStyle = computed(() => ({
  gap:
    normalizedBuckets.value.length > 360
      ? '0'
      : normalizedBuckets.value.length > 180
        ? '1px'
        : '2px',
  gridTemplateColumns: `repeat(${normalizedBuckets.value.length}, minmax(0, 1fr))`,
}))

const bucketWidth = computed(() => {
  const count = normalizedBuckets.value.length
  if (count <= 30) {
    return '8px'
  }
  if (count <= 90) {
    return '5px'
  }
  if (count <= 180) {
    return '4px'
  }
  return '100%'
})

const rateLabel = computed(() => {
  const rate = props.availability?.availability_rate
  if (typeof rate !== 'number') {
    return t('marketplace.availabilityNoData')
  }
  return `${(rate * 100).toFixed(2)}%`
})

const normalizedStatus = computed(() => props.availability?.last_status?.trim().toLowerCase() ?? '')
const isSlowResponse = computed(() =>
  normalizedStatus.value === 'success'
  && typeof props.availability?.last_latency_ms === 'number'
  && props.availability.last_latency_ms > slowResponseThresholdMs,
)
const statusLabel = computed(() => {
  const labels: Record<string, string> = { available: 'probeStatusAvailable', slow: 'probeStatusSlow', fluctuating: 'routingHealthDegraded', interrupted: 'probeStatusInterrupted', recovering: 'routingHealthRecovering', unknown: 'probeStatusUnknown', manual_disabled: 'routingHealthManualDisabled' }
  if (props.currentStatus && labels[props.currentStatus]) return t(`marketplace.${labels[props.currentStatus]}`)
  if (isSlowResponse.value) {
    return t('marketplace.probeStatusSlow')
  }
  if (normalizedStatus.value === 'success') {
    return t('marketplace.probeStatusAvailable')
  }
  if (normalizedStatus.value === 'failed') {
    return t('marketplace.probeStatusUnavailable')
  }
  return t('marketplace.probeStatusUnknown')
})
const statusTextClass = computed(() => {
  if (props.currentStatus) return props.currentStatus === 'available' ? 'text-emerald-700 dark:text-emerald-300' : props.currentStatus === 'interrupted' ? 'text-rose-700 dark:text-rose-300' : ['slow', 'fluctuating', 'recovering'].includes(props.currentStatus) ? 'text-amber-700 dark:text-amber-300' : 'text-gray-600 dark:text-dark-300'
  if (isSlowResponse.value) {
    return 'text-amber-700 dark:text-amber-300'
  }
  if (normalizedStatus.value === 'success') {
    return 'text-emerald-700 dark:text-emerald-300'
  }
  if (normalizedStatus.value === 'failed') {
    return 'text-rose-700 dark:text-rose-300'
  }
  return 'text-gray-600 dark:text-dark-300'
})
const statusDotClass = computed(() => {
  if (props.currentStatus) return props.currentStatus === 'available' ? 'bg-emerald-500' : props.currentStatus === 'interrupted' ? 'bg-rose-500' : ['slow', 'fluctuating', 'recovering'].includes(props.currentStatus) ? 'bg-amber-400' : 'bg-gray-400'
  if (isSlowResponse.value) {
    return 'bg-amber-400'
  }
  if (normalizedStatus.value === 'success') {
    return 'bg-emerald-500'
  }
  if (normalizedStatus.value === 'failed') {
    return 'bg-rose-500'
  }
  return 'bg-gray-400 dark:bg-dark-500'
})
const lastCheckedAbsolute = computed(() => {
  const raw = props.availability?.last_checked_at
  if (!raw) {
    return t('marketplace.probeNoData')
  }
  const value = new Date(raw)
  if (Number.isNaN(value.getTime())) {
    return t('marketplace.probeNoData')
  }
  return value.toLocaleString(locale.value, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
})
// 相对时间随页面存活更新，不触发任何供应商探测。
const lastCheckedLabel = computed(() => {
  const timestamp = Date.parse(props.availability?.last_checked_at ?? '')
  if (!Number.isFinite(timestamp)) return t('marketplace.probeNoData')
  const seconds = Math.round((timestamp - now.value.getTime()) / 1000)
  const formatter = new Intl.RelativeTimeFormat(locale.value, { numeric: 'auto' })
  if (Math.abs(seconds) < 60) return formatter.format(0, 'second')
  if (Math.abs(seconds) < 3600) return formatter.format(Math.round(seconds / 60), 'minute')
  if (Math.abs(seconds) < 86400) return formatter.format(Math.round(seconds / 3600), 'hour')
  return formatter.format(Math.round(seconds / 86400), 'day')
})
const latencyLabel = computed(() => {
  const latency = props.availability?.last_latency_ms
  if (typeof latency !== 'number' || latency < 0) {
    return t('marketplace.probeNoData')
  }
  if (latency >= 1000) {
    return `${(latency / 1000).toFixed(2)} s`
  }
  return `${latency} ms`
})
const consecutiveFailuresLabel = computed(() =>
  String(Math.max(props.availability?.consecutive_failures ?? 0, 0)),
)

const tooltip = computed(() => {
  const availability = props.availability
  if (!availability || typeof availability.availability_rate !== 'number') {
    return t('marketplace.availabilityHintNoData', {
      hours: windowHours.value,
    })
  }
  return t('marketplace.availabilityHint', {
    hours: windowHours.value,
    rate: rateLabel.value,
    success: availability.success_count,
    total: availability.total_count,
  })
})

const ariaLabel = computed(
  () => `${t('marketplace.availabilityWindowHours', { hours: windowHours.value })}: ${rateLabel.value}`,
)

function bucketClass(rate?: number | null, totalCount?: number): string {
  if (!totalCount || typeof rate !== 'number') {
    return 'bg-gray-200 dark:bg-dark-700'
  }
  if (rate >= 0.995) {
    return 'bg-emerald-500'
  }
  if (rate >= 0.98) {
    return 'bg-lime-500'
  }
  if (rate >= 0.9) {
    return 'bg-amber-400'
  }
  return 'bg-rose-400'
}
function bucketTitle(bucket: MarketplaceGroupAvailabilityDay): string {
  if (!bucket.total_count || bucket.availability_rate == null) return `${bucket.date || ''} ${t('marketplace.availabilityNoData')}`.trim()
  return `${bucket.date} | ${(bucket.availability_rate * 100).toFixed(1)}% | ${bucket.success_count}/${bucket.total_count}`
}
</script>

<style scoped>
.probe-overview { container-type: inline-size; }
@container (min-width: 440px) {
  .probe-fields { grid-template-columns: repeat(4, minmax(0, 1fr)); }
}
</style>
