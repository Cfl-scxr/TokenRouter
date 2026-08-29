<template>
  <AppLayout>
    <div class="flex flex-col gap-4">
      <!-- 三栏工作区：桌面横排，移动端纵向堆叠 -->
      <div class="flex flex-col gap-4 lg:h-[calc(100dvh-10rem)] lg:flex-row">
        <!-- 左：控制面板 -->
        <div class="w-full flex-shrink-0 rounded-xl border border-primary-900/10 bg-white dark:border-dark-600 dark:bg-dark-900 lg:w-80">
          <CreativeControlPanel :studio="studio" @generate="onGenerate" @load-from-canvas="onLoadFromCanvas" />
        </div>

        <!-- 中：画布 -->
        <div class="flex min-h-[480px] min-w-0 flex-1 flex-col overflow-hidden rounded-xl border border-primary-900/10 bg-white dark:border-dark-600 dark:bg-dark-900 lg:min-h-0">
          <CreativeCanvas ref="canvasRef" :aspect-ratio="studio.aspectRatio.value" @composite="onComposite" @error="onCanvasError" />
        </div>

        <!-- 右：结果与历史 -->
        <div class="flex w-full flex-shrink-0 flex-col rounded-xl border border-primary-900/10 bg-white dark:border-dark-600 dark:bg-dark-900 lg:w-96">
          <div class="min-h-0 flex-1 overflow-y-auto p-4">
            <h2 class="mb-3 text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('creative.result.title') }}</h2>
            <CreativeResultGrid :studio="studio" @use-as-source="onUseAsSource" />
            <div class="my-4 border-t border-primary-900/10 dark:border-dark-600"></div>
            <CreativeRunHistory :studio="studio" />
          </div>
          <div class="border-t border-primary-900/10 p-3 dark:border-dark-600">
            <button
              type="button"
              class="flex h-9 w-full items-center justify-center gap-1.5 rounded-md border border-red-200 text-xs text-red-600 transition-colors hover:bg-red-50 dark:border-red-500/30 dark:text-red-400 dark:hover:bg-red-500/10"
              @click="showClearConfirm = true"
            >
              <Icon name="trash" size="sm" />
              {{ t('creative.history.clearData') }}
            </button>
          </div>
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
 * 创作台主视图：三栏工作区（控制面板 / 画布 / 结果与历史）。
 * 图片本体只存当前浏览器（IndexedDB），生成时才把所选素材发给模型供应商。
 */
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import CreativeControlPanel from '@/components/creative/CreativeControlPanel.vue'
import CreativeCanvas from '@/components/creative/CreativeCanvas.vue'
import CreativeResultGrid from '@/components/creative/CreativeResultGrid.vue'
import CreativeRunHistory from '@/components/creative/CreativeRunHistory.vue'
import { useCreativeStudio } from '@/composables/useCreativeStudio'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()
const studio = useCreativeStudio()

const canvasRef = ref<InstanceType<typeof CreativeCanvas> | null>(null)
const showClearConfirm = ref(false)

onMounted(() => {
  void studio.loadModels()
  void studio.refreshHistory()
})

// 提交生成：源图取当前素材列表，mask 取画布画笔轨迹（inpaint 时）
async function onGenerate(): Promise<void> {
  let maskBlob: Blob | null = null
  if (studio.operation.value === 'inpaint') {
    try {
      maskBlob = (await canvasRef.value?.getMaskBlob()) ?? null
    } catch (error) {
      console.error('Failed to export mask:', error)
    }
  }
  const exported = {
    sourceBlobs: studio.sourceAssets.value.map((asset) => asset.blob),
    maskBlob,
  }
  await studio.createRun(exported)
}

// 画布合成图加入源图列表（仅本地，不上传）
async function onLoadFromCanvas(): Promise<void> {
  const blob = await canvasRef.value?.exportCompositeBlob()
  if (!blob) {
    studio.error.value = t('creative.error.exportCompositeFailed')
    return
  }
  await studio.addSourceAsset(blob)
}

// 生成结果作为源图二次创作
async function onUseAsSource(blob: Blob): Promise<void> {
  await studio.addSourceAsset(blob)
}

// 画布导出的合成图直接入库
async function onComposite(blob: Blob): Promise<void> {
  await studio.addSourceAsset(blob)
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
