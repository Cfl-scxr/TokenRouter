<template>
  <div>
    <div class="mb-3 flex items-center gap-2">
      <h3 class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-400">{{ t('creative.history.title') }}</h3>
      <button type="button" class="ml-auto text-gray-400 transition-colors hover:text-gray-600 dark:hover:text-gray-200" :title="t('common.refresh')" @click="refresh">
        <Icon name="refresh" size="sm" :class="studio.loadingHistory.value && 'animate-spin'" />
      </button>
    </div>

    <div v-if="studio.runHistory.value.length" class="space-y-2">
      <div
        v-for="run in studio.runHistory.value"
        :key="run.id"
        class="rounded-lg border border-primary-900/10 dark:border-dark-600"
        :class="studio.currentRun.value?.id === run.id && 'border-primary-500 dark:border-primary-500'"
      >
        <!-- run 摘要行 -->
        <div class="flex cursor-pointer items-center gap-2 px-3 py-2" @click="toggleExpand(run.id)">
          <Icon :name="expandedIds.has(run.id) ? 'chevronDown' : 'chevronRight'" size="sm" class="flex-shrink-0 text-gray-400" />
          <span class="status-badge flex-shrink-0" :class="`status-${run.status}`">{{ t(`creative.status.${run.status}`, run.status) }}</span>
          <span class="min-w-0 flex-1 truncate text-xs text-gray-600 dark:text-gray-300">{{ run.model }}</span>
          <span class="flex-shrink-0 text-[11px] text-gray-400 dark:text-dark-400">{{ progressText(run) }}</span>
          <button
            v-if="isActive(run)"
            type="button"
            class="flex-shrink-0 text-gray-400 transition-colors hover:text-red-500"
            :title="t('creative.history.cancel')"
            @click.stop="cancel(run.id)"
          >
            <Icon name="xCircle" size="sm" />
          </button>
        </div>
        <div class="flex items-center gap-2 px-3 pb-2 pl-9 text-[11px] text-gray-400 dark:text-dark-400">
          <span>{{ formatRunTime(run.created_at) }}</span>
          <span v-if="run.actual_cost != null" class="ml-auto">{{ t('creative.result.actualCost', { cost: run.actual_cost }) }}</span>
        </div>

        <!-- 展开：outputs 简版网格 -->
        <div v-if="expandedIds.has(run.id)" class="border-t border-primary-900/10 px-3 py-2 dark:border-dark-600">
          <div v-if="run.outputs?.length" class="grid grid-cols-4 gap-2">
            <div v-for="output in run.outputs" :key="output.output_index" class="relative aspect-square overflow-hidden rounded-md bg-gray-50 dark:bg-dark-950">
              <img
                v-if="assetFor(run.id, output.output_index)"
                :src="urlFor(assetFor(run.id, output.output_index)!.key, assetFor(run.id, output.output_index)!.blob)"
                :alt="`output-${output.output_index}`"
                class="h-full w-full object-cover"
              />
              <div v-else class="flex h-full w-full flex-col items-center justify-center gap-0.5 text-gray-300 dark:text-dark-600">
                <!-- 服务端确认成功（含客户端已 ack）但本地无素材时展示缺失标记 -->
                <Icon :name="isOutputPresent(output) ? 'exclamationTriangle' : 'modalityImage'" size="sm" />
                <span v-if="isOutputPresent(output)" class="scale-90 text-[10px]">{{ t('creative.result.missing') }}</span>
              </div>
              <button
                v-if="assetFor(run.id, output.output_index)"
                type="button"
                class="absolute bottom-0.5 right-0.5 flex h-5 w-5 items-center justify-center rounded bg-black/60 text-white opacity-90 hover:opacity-100"
                :title="t('creative.result.download')"
                @click="download(run.id, output)"
              >
                <Icon name="download" size="sm" />
              </button>
            </div>
          </div>
          <p v-else class="py-1 text-[11px] text-gray-400 dark:text-dark-400">{{ t('creative.history.noOutputs') }}</p>
        </div>
      </div>
    </div>

    <p v-else-if="!studio.loadingHistory.value" class="py-4 text-center text-xs text-gray-400 dark:text-dark-400">
      {{ t('creative.history.empty') }}
    </p>
  </div>
</template>

<script setup lang="ts">
/**
 * 创作 run 历史：状态 / 进度 / 展开 outputs（本地 blob 直接展示，缺失显示占位）。
 * 进行中的 run 可取消；点击摘要行设为当前 run 并展开。
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import Icon from '@/components/icons/Icon.vue'
import { useBlobUrlMap } from './useBlobUrlMap'
import { cancelCreativeRun, type CreativeRun, type CreativeRunOutput } from '@/api/creative'
import { outputAssetKey, type LocalAsset } from '@/utils/creativeLocalStore'
import { formatDateTime } from '@/utils/format'
import type { useCreativeStudio } from '@/composables/useCreativeStudio'

type Studio = ReturnType<typeof useCreativeStudio>

interface Props {
  studio: Studio
}

const props = defineProps<Props>()
// 本地别名：studio 为 props 传入的共享状态机，子组件经它读写
const studio = props.studio
const { t } = useI18n()
const { urlFor } = useBlobUrlMap()

const expandedIds = ref<Set<string>>(new Set())

function toggleExpand(runId: string): void {
  const next = new Set(expandedIds.value)
  if (next.has(runId)) {
    next.delete(runId)
  } else {
    next.add(runId)
  }
  expandedIds.value = next
  // 点击历史 run 设为当前 run，结果网格同步展示
  const run = studio.runHistory.value.find((r) => r.id === runId)
  if (run) studio.currentRun.value = run
}

function isActive(run: CreativeRun): boolean {
  return run.status === 'queued' || run.status === 'running'
}

// 输出在服务端已确认成功（succeeded）或客户端已确认接收（acked）
function isOutputPresent(output: CreativeRunOutput): boolean {
  return output.status === 'succeeded' || output.status === 'acked'
}

function progressText(run: CreativeRun): string {
  // succeeded 与 acked（客户端已确认接收）都计入完成进度
  const succeeded = (run.outputs ?? []).filter((o) => o.status === 'succeeded' || o.status === 'acked').length
  return `${succeeded}/${run.requested_output_count ?? '?'}`
}

// 后端时间戳兼容秒 / 毫秒
function formatRunTime(timestamp: number | undefined): string {
  if (!timestamp) return ''
  const ms = timestamp < 1e12 ? timestamp * 1000 : timestamp
  return formatDateTime(new Date(ms))
}

function assetFor(runId: string, outputIndex: number): LocalAsset | null {
  return studio.outputAssetMap.value.get(outputAssetKey(runId, outputIndex)) ?? null
}

function download(runId: string, output: CreativeRunOutput): void {
  const asset = assetFor(runId, output.output_index)
  if (!asset) return
  const extension = output.mime_type?.split('/')[1] || 'png'
  saveAs(asset.blob, `creative-${runId}-${output.output_index}.${extension}`)
}

async function cancel(runId: string): Promise<void> {
  try {
    await cancelCreativeRun(runId)
  } catch (error) {
    console.error('Failed to cancel creative run:', error)
  } finally {
    await studio.refreshHistory()
  }
}

function refresh(): void {
  void studio.refreshHistory()
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
</style>
