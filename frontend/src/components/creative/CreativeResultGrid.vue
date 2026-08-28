<template>
  <div>
    <!-- run 级状态与费用 -->
    <div v-if="run" class="mb-3 flex flex-wrap items-center gap-2">
      <span class="status-badge" :class="`status-${run.status}`">{{ t(`creative.status.${run.status}`, run.status) }}</span>
      <span class="text-xs text-gray-400 dark:text-dark-400">{{ run.model }}</span>
      <span v-if="run.actual_cost != null" class="ml-auto text-xs text-gray-400 dark:text-dark-400">
        {{ t('creative.result.actualCost', { cost: run.actual_cost }) }}
      </span>
      <span v-else-if="typeof run.estimated_cost === 'number'" class="ml-auto text-xs text-gray-400 dark:text-dark-400">
        {{ t('creative.result.estimatedCost', { cost: run.estimated_cost }) }}
      </span>
    </div>
    <p v-if="run && (run.status === 'failed' || run.status === 'result_lost') && run.error_message" class="mb-3 rounded-md bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-500/10 dark:text-red-400">
      {{ run.error_message }}
    </p>

    <!-- 输出网格 -->
    <div v-if="outputs.length" class="grid grid-cols-2 gap-3">
      <div v-for="output in outputs" :key="output.output_index" class="overflow-hidden rounded-lg border border-primary-900/10 dark:border-dark-600">
        <div class="relative aspect-square bg-gray-50 dark:bg-dark-950">
          <img
            v-if="assetFor(output)"
            :src="urlFor(assetFor(output)!.key, assetFor(output)!.blob)"
            :alt="`output-${output.output_index}`"
            class="h-full w-full object-cover"
          />
          <div v-else-if="isMissing(output)" class="flex h-full w-full flex-col items-center justify-center gap-1 text-gray-400 dark:text-dark-500">
            <Icon name="exclamationTriangle" size="lg" />
            <span class="px-2 text-center text-xs">{{ t('creative.result.missing') }}</span>
          </div>
          <div v-else class="flex h-full w-full items-center justify-center text-gray-300 dark:text-dark-600">
            <Icon name="modalityImage" size="xl" />
          </div>
          <span v-if="output.status !== 'succeeded'" class="status-badge absolute left-1 top-1" :class="`status-${output.status}`">
            {{ t(`creative.outputStatus.${output.status}`, output.status) }}
          </span>
        </div>
        <div v-if="assetFor(output)" class="flex items-center gap-1 p-1.5">
          <button type="button" class="result-action-btn" :title="t('creative.result.download')" @click="download(output)">
            <Icon name="download" size="sm" />
          </button>
          <button type="button" class="result-action-btn" :title="t('creative.result.useAsSource')" @click="useAsSource(output)">
            <Icon name="plus" size="sm" />
          </button>
        </div>
        <p v-else-if="isMissing(output)" class="px-2 py-1.5 text-center text-[11px] text-gray-400 dark:text-dark-400">
          {{ t('creative.result.retryHint') }}
        </p>
      </div>
    </div>

    <EmptyState v-else :title="t('creative.result.empty')" class="py-8" />
  </div>
</template>

<script setup lang="ts">
/**
 * 当前 run 的输出网格：本地 blob 直接展示，缺失的素材显示占位（绝不向服务端请求恢复）。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import Icon from '@/components/icons/Icon.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { useBlobUrlMap } from './useBlobUrlMap'
import { outputAssetKey, type LocalAsset } from '@/utils/creativeLocalStore'
import type { useCreativeStudio } from '@/composables/useCreativeStudio'
import type { CreativeRunOutput } from '@/api/creative'

type Studio = ReturnType<typeof useCreativeStudio>

interface Props {
  studio: Studio
}

interface Emits {
  (e: 'use-as-source', blob: Blob): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { t } = useI18n()
const { urlFor } = useBlobUrlMap()

const run = computed(() => props.studio.currentRun.value)
const outputs = computed(() => (run.value?.outputs?.length ? run.value.outputs : []))

function assetFor(output: CreativeRunOutput): LocalAsset | null {
  if (!run.value) return null
  return props.studio.outputAssetMap.value.get(outputAssetKey(run.value.id, output.output_index)) ?? null
}

// 服务端标记成功但本地无 blob（transient 已过期或被清空）
function isMissing(output: CreativeRunOutput): boolean {
  if (!run.value) return false
  return props.studio.missingOutputKeys.value.has(outputAssetKey(run.value.id, output.output_index))
}

function fileName(output: CreativeRunOutput): string {
  const extension = output.mime_type?.split('/')[1] || 'png'
  return `creative-${run.value?.id ?? 'run'}-${output.output_index}.${extension}`
}

function download(output: CreativeRunOutput): void {
  const asset = assetFor(output)
  if (asset) saveAs(asset.blob, fileName(output))
}

function useAsSource(output: CreativeRunOutput): void {
  const asset = assetFor(output)
  if (asset) emit('use-as-source', asset.blob)
}
</script>

<style scoped>
.status-badge {
  @apply inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium;
  @apply bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300;
}

.status-queued {
  @apply bg-blue-50 text-blue-600 dark:bg-blue-500/10 dark:text-blue-400;
}

.status-running {
  @apply bg-amber-50 text-amber-600 dark:bg-amber-500/10 dark:text-amber-400;
}

.status-succeeded {
  @apply bg-green-50 text-green-600 dark:bg-green-500/10 dark:text-green-400;
}

.status-failed,
.status-result_lost {
  @apply bg-red-50 text-red-600 dark:bg-red-500/10 dark:text-red-400;
}

.status-cancelled {
  @apply bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300;
}

.result-action-btn {
  @apply inline-flex h-7 w-7 items-center justify-center rounded text-gray-400 transition-colors;
  @apply hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200;
}
</style>
