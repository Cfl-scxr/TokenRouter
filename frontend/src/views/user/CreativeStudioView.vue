<template>
  <AppLayout>
    <!-- 上 = 控制面板，下 = 无限画布（移动端画布约 65dvh） -->
    <div class="flex flex-col gap-4 lg:h-[calc(100dvh-10rem)] lg:flex-row">
      <!-- 左 / 上：控制面板 -->
      <div
        class="flex max-h-[60dvh] w-full flex-shrink-0 flex-col overflow-hidden rounded-xl border border-primary-900/10 bg-white dark:border-dark-600 dark:bg-dark-900 lg:h-auto lg:max-h-none lg:w-80"
      >
        <CreativeControlPanel :studio="studio" @generate="onGenerate" @uploaded="onUploaded" @clear-requested="showClearConfirm = true" />
      </div>

      <!-- 画布工作区：无限画布 + 状态胶囊 + 悬浮历史 -->
      <div class="relative h-[65dvh] w-full flex-shrink-0 lg:h-auto lg:min-w-0 lg:flex-1">
        <CreativeCanvas ref="canvasRef" class="absolute inset-0" @error="onCanvasError" />
        <CreativeRunHistory :studio="studio" />

        <!-- 生成状态胶囊：左上角浮动 -->
        <div
          v-if="pillState && !pillHidden"
          class="absolute left-3 top-3 z-10 flex max-w-[calc(100%-6rem)] items-center gap-2 rounded-full border border-primary-900/10 bg-white/90 px-3 py-1.5 text-xs shadow-md backdrop-blur dark:border-dark-600 dark:bg-dark-900/90"
          :class="pillState.toneClass"
        >
          <Icon v-if="pillState.spinning" name="refresh" size="sm" class="animate-spin" />
          <span class="whitespace-nowrap font-medium">{{ pillState.text }}</span>
          <span v-if="pillState.detail" class="truncate text-gray-500 dark:text-dark-400">{{ pillState.detail }}</span>
        </div>
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
 * - 注册画布桥接：收割成功的输出自动上板；历史点击平移到已上板的输出
 * - 画布左上角浮动状态胶囊反馈生成状态（终态非成功几秒后自动消隐，成功保持到下次生成）
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
// 终态（非成功）状态胶囊几秒后自动消隐
let pillHideTimer: ReturnType<typeof setTimeout> | null = null
const pillHidden = ref(false)

onMounted(() => {
  // 画布桥接：收割上板 + 历史平移；桥接方法自身保证异常不外溢
  studio.registerCanvasBridge({
    placeOutput: (asset) => {
      void canvasRef.value?.placeOutput(asset)
    },
    panToRunOutput: (runId, outputIndex) => canvasRef.value?.panToRunOutput(runId, outputIndex) ?? false,
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
