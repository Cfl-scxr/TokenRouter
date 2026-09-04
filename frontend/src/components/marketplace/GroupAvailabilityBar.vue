<template>
  <div class="w-full min-w-0 space-y-2" :title="tooltip">
    <div class="grid grid-cols-2 gap-1.5 sm:grid-cols-4">
      <div class="rounded-md bg-gray-50 px-2 py-1.5 dark:bg-dark-800/70">
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
      <div class="rounded-md bg-gray-50 px-2 py-1.5 dark:bg-dark-800/70">
        <div class="text-[10px] leading-4 text-gray-500 dark:text-dark-400">
          {{ t('marketplace.probeLastCheckedAt') }}
        </div>
        <div class="mt-0.5 truncate text-xs font-semibold text-gray-800 dark:text-dark-100" data-testid="probe-last-checked-at">
          {{ lastCheckedLabel }}
        </div>
      </div>
      <div class="rounded-md bg-gray-50 px-2 py-1.5 dark:bg-dark-800/70">
        <div class="text-[10px] leading-4 text-gray-500 dark:text-dark-400">
          {{ t('marketplace.probeCurrentLatency') }}
        </div>
        <div class="mt-0.5 text-xs font-semibold text-gray-800 dark:text-dark-100" data-testid="probe-current-latency">
          {{ latencyLabel }}
        </div>
      </div>
      <div class="rounded-md bg-gray-50 px-2 py-1.5 dark:bg-dark-800/70">
        <div class="text-[10px] leading-4 text-gray-500 dark:text-dark-400">
          {{ t('marketplace.probeConsecutiveFailures') }}
        </div>
        <div class="mt-0.5 text-xs font-semibold text-gray-800 dark:text-dark-100" data-testid="probe-consecutive-failures">
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
        />
      </div>
      <div class="w-[96px] shrink-0 text-left">
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
import { useI18n } from 'vue-i18n'
import type { MarketplaceGroupAvailability, MarketplaceGroupAvailabilityDay } from '@/types'

const props = defineProps<{
  availability?: MarketplaceGroupAvailability | null
}>()

const { t, locale } = useI18n()

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
const statusLabel = computed(() => {
  if (normalizedStatus.value === 'success') {
    return t('marketplace.probeStatusAvailable')
  }
  if (normalizedStatus.value === 'failed') {
    return t('marketplace.probeStatusUnavailable')
  }
  return t('marketplace.probeStatusUnknown')
})
const statusTextClass = computed(() => {
  if (normalizedStatus.value === 'success') {
    return 'text-emerald-700 dark:text-emerald-300'
  }
  if (normalizedStatus.value === 'failed') {
    return 'text-rose-700 dark:text-rose-300'
  }
  return 'text-gray-600 dark:text-dark-300'
})
const statusDotClass = computed(() => {
  if (normalizedStatus.value === 'success') {
    return 'bg-emerald-500'
  }
  if (normalizedStatus.value === 'failed') {
    return 'bg-rose-500'
  }
  return 'bg-gray-400 dark:bg-dark-500'
})
const lastCheckedLabel = computed(() => {
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
</script>
