<template>
  <!-- 历史入口：画布区域右上角的手写 SVG 图标按钮 -->
  <button
    type="button"
    class="absolute right-3 top-3 z-20 flex h-9 w-9 items-center justify-center rounded-xl border border-primary-900/10 bg-white/90 text-gray-600 shadow-md backdrop-blur transition-colors hover:text-gray-900 dark:border-dark-600 dark:bg-dark-900/90 dark:text-gray-300 dark:hover:text-gray-100"
    :class="open && 'text-primary-700 dark:text-primary-300'"
    :title="t('creative.history.toggle')"
    @click="open = !open"
  >
    <HistoryIcon />
  </button>

  <!-- 悬浮历史列表：点击展开 / 收起，选择行后不自动收起 -->
  <div
    v-if="open"
    class="absolute right-3 top-14 z-20 flex max-h-[70%] w-80 flex-col overflow-hidden rounded-xl border border-primary-900/10 bg-white/95 shadow-lg backdrop-blur dark:border-dark-600 dark:bg-dark-900/95"
  >
    <div class="flex items-center gap-2 border-b border-primary-900/10 px-3 py-2 dark:border-dark-600">
      <h3 class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-400">
        {{ t('creative.history.title') }}
      </h3>
      <button
        type="button"
        class="ml-auto text-gray-400 transition-colors hover:text-gray-600 dark:hover:text-gray-200"
        :title="t('common.refresh')"
        @click="refresh"
      >
        <Icon name="refresh" size="sm" :class="studio.loadingHistory.value && 'animate-spin'" />
      </button>
      <button
        type="button"
        class="text-gray-400 transition-colors hover:text-gray-600 dark:hover:text-gray-200"
        :title="t('common.close')"
        @click="open = false"
      >
        <Icon name="x" size="sm" />
      </button>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto p-2">
      <div v-if="studio.runHistory.value.length" class="space-y-1.5">
        <div
          v-for="run in studio.runHistory.value"
          :key="run.id"
          class="cursor-pointer rounded-lg border border-primary-900/10 px-3 py-2 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-800"
          :class="studio.currentRun.value?.id === run.id && 'border-primary-500 dark:border-primary-500'"
          @click="locateRun(run)"
        >
          <div class="flex items-center gap-2">
            <span class="status-badge flex-shrink-0" :class="`status-${run.status}`">
              {{ t(`creative.status.${run.status}`, run.status) }}
            </span>
            <span class="min-w-0 flex-1 truncate text-xs text-gray-600 dark:text-gray-300">{{ run.model }}</span>
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
          <div class="mt-1 flex items-center gap-2 text-[11px] text-gray-400 dark:text-dark-400">
            <span>{{ formatRunTime(run.created_at) }}</span>
            <span v-if="run.actual_cost != null" class="ml-auto">{{ t('creative.result.actualCost', { cost: run.actual_cost }) }}</span>
          </div>
        </div>
      </div>
      <p v-else-if="!studio.loadingHistory.value" class="py-6 text-center text-xs text-gray-400 dark:text-dark-400">
        {{ t('creative.history.empty') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * 创作 run 历史（悬浮层）：
 * - 画布右上角图标按钮展开 / 收起；列表每行 = 状态徽章 + 模型名 + 时间（+ 实际费用）
 * - 点击行：该任务的输出已在本画布上时视角平移过去（按 runId + outputIndex 匹配），不在画布则不动；点击行不自动收起
 * - 进行中的任务行内提供取消按钮（可取消历史里的任意进行中任务）
 */
import { h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { CreativeRun } from '@/api/creative'
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

// 历史面板展开状态：默认折叠
const open = ref(false)

// 历史（回旋时钟）图标：手写 SVG，仿 🕘 样式，风格对齐 AppSidebar 内手写图标
const HistoryIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5', class: 'h-5 w-5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8',
        }),
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M3 3v5h5',
        }),
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M12 7v5l4 2',
        }),
      ],
    ),
}

function isActive(run: CreativeRun): boolean {
  return run.status === 'queued' || run.status === 'running'
}

// 后端时间戳兼容秒 / 毫秒
function formatRunTime(timestamp: number | undefined): string {
  if (!timestamp) return ''
  const ms = timestamp < 1e12 ? timestamp * 1000 : timestamp
  return formatDateTime(new Date(ms))
}

// 点击历史行：依次尝试该任务各输出，已在画布上的直接平移过去
function locateRun(run: CreativeRun): void {
  const outputs = run.outputs?.length
    ? run.outputs.map((output) => output.output_index)
    : Array.from({ length: run.requested_output_count ?? 1 }, (_, index) => index)
  for (const outputIndex of outputs) {
    if (studio.panToRunOutput(run.id, outputIndex)) return
  }
}

async function cancel(runId: string): Promise<void> {
  await studio.cancelRun(runId)
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
