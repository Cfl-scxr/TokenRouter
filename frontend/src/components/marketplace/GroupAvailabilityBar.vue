<template>
  <div
    class="flex w-full min-w-0 items-center gap-3 xl:min-w-[520px]"
    :title="tooltip"
  >
    <div
      class="flex h-8 min-w-0 flex-1 items-center gap-[3px]"
      role="img"
      :aria-label="ariaLabel"
    >
      <span
        v-for="(day, index) in normalizedDays"
        :key="day.date || index"
        :class="[
          'h-4 min-w-[3px] flex-1 rounded-[2px]',
          dayClass(day.availability_rate, day.total_count),
        ]"
      />
    </div>
    <div class="w-[86px] shrink-0 text-right">
      <div class="font-mono text-sm font-semibold text-gray-900 dark:text-white">
        {{ rateLabel }}
      </div>
      <div class="mt-0.5 text-[11px] font-medium text-gray-500 dark:text-dark-400">
        {{ t('marketplace.availability30d') }}
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

const { t } = useI18n()

const windowDays = computed(() => Math.max(props.availability?.window_days ?? 30, 1))

const normalizedDays = computed<MarketplaceGroupAvailabilityDay[]>(() => {
  const days = props.availability?.days ?? []
  const target = windowDays.value
  if (days.length >= target) {
    return days.slice(days.length - target)
  }
  return [
    ...Array.from({ length: target - days.length }, () => ({
      date: '',
      success_count: 0,
      total_count: 0,
      availability_rate: null,
    })),
    ...days,
  ]
})

const rateLabel = computed(() => {
  const rate = props.availability?.availability_rate
  if (typeof rate !== 'number') {
    return t('marketplace.availabilityNoData')
  }
  return `${(rate * 100).toFixed(2)}%`
})

const tooltip = computed(() => {
  const availability = props.availability
  if (!availability || typeof availability.availability_rate !== 'number') {
    return t('marketplace.availabilityHintNoData')
  }
  return t('marketplace.availabilityHint', {
    rate: rateLabel.value,
    success: availability.success_count,
    total: availability.total_count,
  })
})

const ariaLabel = computed(() => `${t('marketplace.availability30d')}: ${rateLabel.value}`)

function dayClass(rate?: number | null, totalCount?: number): string {
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
