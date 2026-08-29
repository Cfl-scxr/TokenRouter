<template>
  <AppLayout>
    <!-- 整个内容区即无限画布背景：负外边距抵消 app-main 四周内边距，画布铺满全幅（含顶部，点阵直达 header 边界） -->
    <div
      ref="stageRef"
      class="relative -mx-4 -mb-4 -mt-4 h-[calc(100dvh-3.5rem)] md:-mx-6 md:-mb-6 md:-mt-5 lg:-mx-8 lg:-mb-8 lg:-mt-4"
    >
      <CreativeCanvas ref="canvasRef" class="absolute inset-0" :operation="studio.operation.value" @error="onCanvasError" />
      <CreativeRunHistory :studio="studio" />

      <!-- 设置：左上角齿轮按钮，点击向下展开设置项 -->
      <div class="absolute left-3 top-3 z-20">
        <button
          type="button"
          class="flex h-9 w-9 items-center justify-center rounded-xl border border-primary-900/10 bg-white/90 text-gray-600 shadow-md backdrop-blur transition-colors hover:text-gray-900 dark:border-dark-600 dark:bg-dark-900/90 dark:text-gray-300 dark:hover:text-gray-100"
          :class="settingsOpen && 'text-primary-700 dark:text-primary-300'"
          :title="t('creative.canvas.settings')"
          @click="settingsOpen = !settingsOpen"
        >
          <Icon name="cog" size="md" />
        </button>
        <!-- 向下展开的设置面板 -->
        <div
          v-if="settingsOpen"
          class="absolute left-0 top-12 w-64 rounded-xl border border-primary-900/10 bg-white/95 p-3 shadow-lg backdrop-blur dark:border-dark-600 dark:bg-dark-900/95"
        >
          <button
            type="button"
            class="flex h-9 w-full items-center justify-center gap-1.5 rounded-md border border-red-200 text-xs text-red-600 transition-colors hover:bg-red-50 dark:border-red-500/30 dark:text-red-400 dark:hover:bg-red-500/10"
            @click="onClearRequested"
          >
            <Icon name="trash" size="sm" />
            {{ t('creative.history.clearData') }}
          </button>
        </div>
      </div>

      <!-- 聊天式输入框：无选中时底部居中；选中图片时跟随到图片下方（位置由 composerStyle 计算） -->
      <div ref="composerSlotRef" class="absolute z-30" :style="composerStyle">
        <CreativeComposer
          :studio="studio"
          :has-selection="hasSelection"
          @generate="onGenerate"
        />
      </div>

      <!-- 生成状态胶囊：移动端顶部居中，桌面端左下角 -->
      <div
        v-if="pillState && !pillHidden"
        class="absolute left-1/2 top-3 z-10 flex max-w-[calc(100%-6rem)] -translate-x-1/2 items-center gap-2 rounded-full border border-primary-900/10 bg-white/90 px-3 py-1.5 text-xs shadow-md backdrop-blur dark:border-dark-600 dark:bg-dark-900/90 lg:bottom-3 lg:left-3 lg:top-auto lg:max-w-[calc(100%-24rem)] lg:translate-x-0"
        :class="pillState.toneClass"
      >
        <Icon v-if="pillState.spinning" name="refresh" size="sm" class="animate-spin" />
        <span class="whitespace-nowrap font-medium">{{ pillState.text }}</span>
        <span v-if="pillState.detail" class="truncate text-gray-500 dark:text-dark-400">{{ pillState.detail }}</span>
      </div>
    </div>

    <ConfirmDialog
      :show="showClearConfirm"
      :title="t('creative.history.confirmClearTitle')"
      :message="t('creative.history.confirmClearMessage')"
      :confirm-text="t('creative.history.clearData')"
      danger
      @confirm="onClearLocalData"
      @cancel="showClearConfirm = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * 创作台主视图：全幅无限画布 + 聊天式输入框。
 * - 输入框默认底部居中；画布选中图片时跟随到图片下方（随平移 / 缩放实时跟踪，越界自动夹取）
 * - 生成时从画布收集输入：edit/inpaint 取当前选中图片的原始 blob，inpaint 另取画笔 mask 导出
 * - 注册画布桥接：收割成功的输出自动上板；历史里的输出可一键导入画布
 * - 左上角设置（清空本机创作数据）、右上角历史、顶部工具栏（上传 / 局部重绘画笔组 / 删除 / 清空）
 * 图片本体只存当前浏览器（IndexedDB），生成时才把所选素材发给模型供应商。
 */
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import CreativeComposer from '@/components/creative/CreativeComposer.vue'
import CreativeCanvas from '@/components/creative/CreativeCanvas.vue'
import CreativeRunHistory from '@/components/creative/CreativeRunHistory.vue'
import { CREATIVE_RUN_TERMINAL_STATUSES } from '@/api/creative'
import { useCreativeStudio } from '@/composables/useCreativeStudio'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()
const studio = useCreativeStudio()

const canvasRef = ref<InstanceType<typeof CreativeCanvas> | null>(null)
const stageRef = ref<HTMLDivElement | null>(null)
const composerSlotRef = ref<HTMLDivElement | null>(null)
const showClearConfirm = ref(false)
// 设置弹层开关（齿轮在画布左上角，弹层向下展开）
const settingsOpen = ref(false)
// 终态（非成功）状态胶囊几秒后自动消隐
let pillHideTimer: ReturnType<typeof setTimeout> | null = null
const pillHidden = ref(false)

// ==================== 输入框定位 ====================

// 舞台（画布容器）与输入框的实测尺寸，用于跟随定位的夹取计算
const stageSize = reactive({ width: 0, height: 0 })
const composerSize = reactive({ width: 0, height: 0 })
let stageObserver: ResizeObserver | null = null
let composerObserver: ResizeObserver | null = null

// 当前选中图片的画布视口包围盒（未选中为 null；expose 的 ref 在实例上自动解包）
const selectedRect = computed(() => canvasRef.value?.selectedRect ?? null)
const hasSelection = computed(() => selectedRect.value !== null)

// 输入框定位样式：无选中 → 底部居中；有选中 → 跟随图片下方（下方放不下时放到上方，再不行夹取在可视区内）
const composerStyle = computed<Record<string, string>>(() => {
  const rect = selectedRect.value
  if (!rect) {
    return { left: '50%', bottom: '16px', transform: 'translateX(-50%)' }
  }
  const pad = 8
  const gap = 12
  const width = composerSize.width || 600
  const height = composerSize.height || 130
  const viewWidth = stageSize.width || 800
  const viewHeight = stageSize.height || 600
  const style: Record<string, string> = {}
  style.left = `${clamp(rect.left + rect.width / 2 - width / 2, pad, Math.max(pad, viewWidth - width - pad))}px`
  let top = rect.top + rect.height + gap
  if (top + height > viewHeight - pad) {
    const above = rect.top - height - gap
    top = above >= pad ? above : Math.max(pad, viewHeight - height - pad)
  }
  style.top = `${top}px`
  return style
})

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

// ==================== 生命周期 ====================

onMounted(() => {
  // 画布桥接：收割自动上板 + 历史输出导入画布；桥接方法自身保证异常不外溢
  studio.registerCanvasBridge({
    placeOutput: (asset) => {
      void canvasRef.value?.placeOutput(asset)
    },
    importToCanvas: (blob, runId, outputIndex) => {
      void canvasRef.value?.placeOutput({ blob, runId, outputIndex })
    },
  })
  // 实测舞台与输入框尺寸（夹取用），尺寸变化实时更新
  if (stageRef.value) {
    const syncStage = () => {
      stageSize.width = stageRef.value?.clientWidth ?? 0
      stageSize.height = stageRef.value?.clientHeight ?? 0
    }
    syncStage()
    stageObserver = new ResizeObserver(syncStage)
    stageObserver.observe(stageRef.value)
  }
  if (composerSlotRef.value) {
    const syncComposer = () => {
      composerSize.width = composerSlotRef.value?.clientWidth ?? 0
      composerSize.height = composerSlotRef.value?.clientHeight ?? 0
    }
    syncComposer()
    composerObserver = new ResizeObserver(syncComposer)
    composerObserver.observe(composerSlotRef.value)
  }
  void studio.loadModels()
  void studio.refreshHistory()
})

onBeforeUnmount(() => {
  studio.registerCanvasBridge(null)
  if (pillHideTimer) clearTimeout(pillHideTimer)
  stageObserver?.disconnect()
  composerObserver?.disconnect()
})

// ==================== 生成状态胶囊 ====================

interface StatusPill {
  text: string
  detail?: string
  spinning: boolean
  toneClass: string
}

const PILL_TONES: Record<string, string> = {
  queued: 'text-blue-700 dark:text-blue-300',
  running: 'text-amber-700 dark:text-amber-300',
  succeeded: 'text-green-700 dark:text-green-300',
  failed: 'text-red-700 dark:text-red-300',
  cancelled: 'text-gray-600 dark:text-dark-300',
  result_lost: 'text-red-700 dark:text-red-300',
  submitting: 'text-blue-700 dark:text-blue-300',
}

const pillState = computed<StatusPill | null>(() => {
  if (studio.polling.value || studio.busy.value) {
    const status = studio.currentRun.value?.status
    const phase = status === 'running' ? 'running' : status === 'queued' ? 'queued' : 'submitting'
    return {
      text: t(`creative.status.${phase}`),
      spinning: true,
      toneClass: PILL_TONES[phase],
    }
  }
  const run = studio.currentRun.value
  if (!run || !CREATIVE_RUN_TERMINAL_STATUSES.includes(run.status)) return null
  return {
    text: t(`creative.status.${run.status}`, run.status),
    // 失败 / 结果丢失附带服务端原因
    detail: run.error_message,
    spinning: false,
    toneClass: PILL_TONES[run.status] ?? '',
  }
})

// 状态变化时重新显示；失败 / 取消 / 结果丢失几秒后自动消隐，成功保持到下次生成
watch(
  () => studio.currentRun.value?.status,
  (status, previous) => {
    if (!status || status === previous) return
    pillHidden.value = false
    if (pillHideTimer) clearTimeout(pillHideTimer)
    pillHideTimer = null
    if (status === 'failed' || status === 'cancelled' || status === 'result_lost') {
      pillHideTimer = setTimeout(() => {
        pillHidden.value = true
      }, 5000)
    }
  },
)

// ==================== 生成与画布输入采集 ====================

// 提交生成：edit/inpaint 用当前选中图片作源图，inpaint 另附画笔 mask
async function onGenerate(): Promise<void> {
  const operation = studio.operation.value
  let sourceBlobs: Blob[] = []
  let maskBlob: Blob | null = null
  if (operation === 'edit' || operation === 'inpaint') {
    const blob = await canvasRef.value?.getSelectedImageBlob()
    if (!blob) {
      studio.error.value = t('creative.panel.selectImageHint')
      return
    }
    sourceBlobs = [blob]
  }
  if (operation === 'inpaint') {
    try {
      maskBlob = (await canvasRef.value?.getMaskBlob()) ?? null
    } catch (error) {
      console.error('Failed to export mask:', error)
    }
    if (!maskBlob) {
      studio.error.value = t('creative.error.maskRequired')
      return
    }
  }
  await studio.createRun({ sourceBlobs, maskBlob })
}

function onCanvasError(message: string): void {
  studio.error.value = message
}

// 设置弹层里的清空入口：收起弹层并弹出确认
function onClearRequested(): void {
  settingsOpen.value = false
  showClearConfirm.value = true
}

async function onClearLocalData(): Promise<void> {
  showClearConfirm.value = false
  try {
    await studio.clearLocalData()
    canvasRef.value?.resetCanvas()
    appStore.showSuccess(t('creative.history.clearSuccess'))
  } catch {
    // 清空失败时给出明确提示，错误详情已写入 studio.error
    appStore.showError(t('creative.error.clearFailed'))
  }
}
</script>
