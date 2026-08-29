<template>
  <AppLayout>
    <!-- 整个内容区即无限画布背景：负外边距抵消 app-main 四周内边距，画布铺满全幅（含顶部，点阵直达 header 边界） -->
    <div
      class="relative -mx-4 -mb-4 -mt-4 h-[calc(100dvh-3.5rem)] md:-mx-6 md:-mb-6 md:-mt-5 lg:-mx-8 lg:-mb-8 lg:-mt-4"
    >
      <CreativeCanvas ref="canvasRef" class="absolute inset-0" @error="onCanvasError" />
      <CreativeRunHistory :studio="studio" />

      <!-- 控制面板：桌面端左侧浮动卡片；移动端顶部浮层，均压在画布之上 -->
      <div
        class="absolute inset-x-3 top-3 z-10 flex max-h-[58dvh] flex-col overflow-y-auto rounded-xl border border-primary-900/10 bg-white/95 shadow-lg backdrop-blur dark:border-dark-600 dark:bg-dark-900/95 lg:bottom-3 lg:left-3 lg:right-auto lg:max-h-none lg:w-80"
      >
        <CreativeControlPanel :studio="studio" @generate="onGenerate" @uploaded="onUploaded" />
      </div>

      <!-- 设置：右下角齿轮按钮，样式与历史按钮一致；点击向上展开设置项 -->
      <div class="absolute bottom-3 right-3 z-20">
        <button
          type="button"
          class="flex h-9 w-9 items-center justify-center rounded-xl border border-primary-900/10 bg-white/90 text-gray-600 shadow-md backdrop-blur transition-colors hover:text-gray-900 dark:border-dark-600 dark:bg-dark-900/90 dark:text-gray-300 dark:hover:text-gray-100"
          :class="settingsOpen && 'text-primary-700 dark:text-primary-300'"
          :title="t('creative.canvas.settings')"
          @click="settingsOpen = !settingsOpen"
        >
          <Icon name="cog" size="md" />
        </button>
        <!-- 向上展开的设置面板 -->
        <div
          v-if="settingsOpen"
          class="absolute bottom-12 right-0 w-64 rounded-xl border border-primary-900/10 bg-white/95 p-3 shadow-lg backdrop-blur dark:border-dark-600 dark:bg-dark-900/95"
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

      <!-- 生成状态胶囊：移动端左下（工具栏上方），桌面端底部居中（避开左右角标） -->
      <div
        v-if="pillState && !pillHidden"
        class="absolute bottom-14 left-3 z-10 flex max-w-[calc(100%-6rem)] items-center gap-2 rounded-full border border-primary-900/10 bg-white/90 px-3 py-1.5 text-xs shadow-md backdrop-blur dark:border-dark-600 dark:bg-dark-900/90 lg:bottom-3 lg:left-1/2 lg:right-auto lg:-translate-x-1/2"
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
 * 创作台主视图：左侧控制面板 + 无限画布工作台。
 * - 生成时从画布收集输入：edit/inpaint 取当前选中图片的原始 blob，inpaint 另取画笔 mask 导出
 * - 注册画布桥接：收割成功的输出自动上板；历史里的输出可一键导入画布
 * - 右下角设置按钮（齿轮）向上展开设置项，收纳"清空本机创作数据"
 * - 画布浮动状态胶囊反馈生成状态（终态非成功几秒后自动消隐，成功保持到下次生成）
 * 图片本体只存当前浏览器（IndexedDB），生成时才把所选素材发给模型供应商。
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import CreativeControlPanel from '@/components/creative/CreativeControlPanel.vue'
import CreativeCanvas from '@/components/creative/CreativeCanvas.vue'
import CreativeRunHistory from '@/components/creative/CreativeRunHistory.vue'
import { CREATIVE_RUN_TERMINAL_STATUSES } from '@/api/creative'
import { useCreativeStudio } from '@/composables/useCreativeStudio'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()
const studio = useCreativeStudio()

const canvasRef = ref<InstanceType<typeof CreativeCanvas> | null>(null)
const showClearConfirm = ref(false)
// 右下角设置弹层
const settingsOpen = ref(false)
// 终态（非成功）状态胶囊几秒后自动消隐
let pillHideTimer: ReturnType<typeof setTimeout> | null = null
const pillHidden = ref(false)

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
  void studio.loadModels()
  void studio.refreshHistory()
})

onBeforeUnmount(() => {
  studio.registerCanvasBridge(null)
  if (pillHideTimer) clearTimeout(pillHideTimer)
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

// 上传裁剪结果：直接放上画布当前视角中心
function onUploaded(blob: Blob): void {
  void canvasRef.value?.addUploadedImage(blob)
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
