<template>
  <div class="flex h-full flex-col overflow-y-auto p-4">
    <h2 class="mb-4 text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('creative.panel.title') }}</h2>

    <!-- 模型 + 分组选择 -->
    <label class="panel-label">{{ t('creative.panel.model') }}</label>
    <Select
      :model-value="studio.selectedOptionKey.value"
      :options="modelSelectOptions"
      :placeholder="t('creative.panel.selectModelPlaceholder')"
      :disabled="studio.loadingModels.value"
      @update:model-value="onModelChange"
    />
    <!-- 模型目录空态：区分功能被管理员关闭与分组未配置图片生成 -->
    <p
      v-if="showModelsEmptyHint"
      class="mt-2 rounded-md border border-primary-900/10 bg-primary-900/5 px-3 py-2 text-xs text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-400"
    >
      {{ modelsEmptyHintText }}
    </p>

    <!-- 操作选择 -->
    <label class="panel-label mt-4">{{ t('creative.panel.operation') }}</label>
    <Select
      :model-value="studio.operation.value"
      :options="operationSelectOptions"
      :placeholder="t('creative.panel.selectOperation')"
      :disabled="!studio.selectedOption.value"
      @update:model-value="onOperationChange"
    />
    <!-- 图生图 / 局部重绘需要先在画布中选中一张源图 -->
    <p v-if="studio.operation.value === 'edit'" class="mt-2 rounded-md border border-primary-900/10 bg-primary-900/5 px-3 py-2 text-xs text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-400">
      {{ t('creative.panel.selectImageHint') }}
    </p>
    <!-- 局部重绘额外需要画笔 mask -->
    <p v-else-if="studio.operation.value === 'inpaint'" class="mt-2 rounded-md border border-primary-900/10 bg-primary-900/5 px-3 py-2 text-xs text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-400">
      {{ t('creative.panel.maskHint') }}
    </p>

    <!-- prompt -->
    <label class="panel-label mt-4">{{ t('creative.panel.prompt') }}</label>
    <TextArea
      class="creative-prompt"
      :model-value="studio.prompt.value"
      :placeholder="t('creative.panel.promptPlaceholder')"
      :rows="5"
      @update:model-value="onPromptChange"
    />
    <p class="mt-1 text-right text-xs" :class="promptLength > PROMPT_MAX ? 'text-red-500' : 'text-gray-400 dark:text-dark-400'">
      {{ promptLength }}/{{ PROMPT_MAX }}
    </p>

    <!-- 上传图片：裁剪确认后直接放上画布当前视角中心 -->
    <label class="panel-label">{{ t('creative.panel.sourceImages') }}</label>
    <div class="flex flex-wrap items-center gap-2">
      <button type="button" class="panel-upload-btn" @click="fileInputRef?.click()">
        <Icon name="upload" size="sm" />
        {{ t('creative.panel.uploadSource') }}
      </button>
      <input
        ref="fileInputRef"
        type="file"
        accept="image/png,image/jpeg,image/webp"
        multiple
        class="hidden"
        @change="onFilesPicked"
      />
    </div>
    <p class="mt-1 text-xs text-gray-400 dark:text-dark-400">{{ t('creative.panel.uploadHint') }}</p>

    <!-- 尺寸 / 比例 / 数量 -->
    <div class="mt-4 grid grid-cols-2 gap-3">
      <div>
        <label class="panel-label">{{ t('creative.panel.imageSize') }}</label>
        <Select
          :model-value="studio.imageSize.value"
          :options="imageSizeOptions"
          :disabled="!studio.imageSizeOptions.value.length"
          @update:model-value="onImageSizeChange"
        />
      </div>
      <div>
        <label class="panel-label">{{ t('creative.panel.aspectRatio') }}</label>
        <Select :model-value="studio.aspectRatio.value" :options="aspectRatioOptions" @update:model-value="onAspectRatioChange" />
      </div>
      <div>
        <label class="panel-label">{{ t('creative.panel.outputCount') }}</label>
        <Select :model-value="studio.outputCount.value" :options="outputCountOptions" @update:model-value="onOutputCountChange" />
      </div>
    </div>

    <!-- 生成按钮（主操作，大尺寸突出；shrink-0 防止面板溢出时 flex 压缩高度） -->
    <button
      type="button"
      style="height: 44px"
      class="mt-6 flex w-full shrink-0 items-center justify-center gap-2 rounded-control bg-primary-600 text-base font-semibold text-white transition-colors hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-60"
      :disabled="!studio.canGenerate.value"
      @click="emit('generate')"
    >
      <Icon v-if="studio.busy.value" name="refresh" size="md" class="animate-spin" />
      <Icon v-else name="sparkles" size="md" />
      {{ studio.busy.value ? t('creative.panel.generating') : t('creative.panel.generate') }}
    </button>
    <p v-if="studio.estimatedCost.value !== null" class="mt-2 text-center text-xs text-gray-400 dark:text-dark-400">
      {{ t('creative.panel.estimatedCost', { cost: formatBalanceAmount(studio.estimatedCost.value, { fractionDigits: 3 }) }) }}
    </p>
    <p v-if="studio.error.value" class="mt-2 rounded-md bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-500/10 dark:text-red-400">
      {{ studio.error.value }}
    </p>

    <!-- 裁剪弹窗队列：每张图片依次进入，确认/跳过后直接放上画布 -->
    <CropperModal :show="cropQueue.length > 0" :blob="cropQueue[0] ?? null" @confirm="onCropConfirm" @skip="onCropConfirm" @cancel="onCropCancel" />
  </div>
</template>

<script setup lang="ts">
/**
 * 创作台左侧面板：模型/操作/prompt/上传（裁剪流程）/尺寸/生成/清空本机数据。
 * 上传不再维护源图缩略图列表：裁剪确认后通过 uploaded 事件交给画布放到当前视角中心。
 * 状态全部经由 props 传入的 studio（useCreativeStudio 返回值）读写。
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import Icon from '@/components/icons/Icon.vue'
import CropperModal from './CropperModal.vue'
import { useAppStore } from '@/stores/app'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import type { useCreativeStudio } from '@/composables/useCreativeStudio'
import { creativeOptionKey } from '@/composables/useCreativeStudio'

type Studio = ReturnType<typeof useCreativeStudio>

interface Props {
  studio: Studio
}

interface Emits {
  (e: 'generate'): void
  (e: 'uploaded', blob: Blob): void
}

const props = defineProps<Props>()
// 本地别名：studio 为 props 传入的共享状态机，子组件经它读写
const studio = props.studio
const emit = defineEmits<Emits>()
const { t } = useI18n()
const appStore = useAppStore()
const { formatBalanceAmount } = useBalanceDisplay()

const PROMPT_MAX = 8000

const fileInputRef = ref<HTMLInputElement | null>(null)
// 待裁剪队列：确认/跳过一张后自动出队下一张
const cropQueue = ref<Blob[]>([])

const promptLength = computed(() => studio.prompt.value.length)

// 模型目录加载完成且为空时展示空态提示（加载失败时 models 同样为空，伴随 error 红条展示）
const showModelsEmptyHint = computed(
  () => !studio.loadingModels.value && studio.models.value.length === 0,
)

// 功能被管理员关闭时提示联系管理员开启，否则提示分组未配置图片生成
const modelsEmptyHintText = computed(() =>
  appStore.cachedPublicSettings?.creative_enabled === false
    ? t('creative.panel.studioDisabled')
    : t('creative.panel.noModelsAvailable'),
)

// "group_name — model" 合成选项
const modelSelectOptions = computed(() =>
  studio.models.value.map((option) => ({
    value: creativeOptionKey(option),
    label: `${option.group_name} — ${option.model}`,
  })),
)

const operationSelectOptions = computed(() =>
  studio.operationOptions.value.map((operation) => ({
    value: operation,
    label: t(`creative.operations.${operation}`, operation),
  })),
)

const imageSizeOptions = computed(() =>
  studio.imageSizeOptions.value.map((size) => ({ value: size, label: size })),
)

const ASPECT_RATIOS = ['1:1', '4:3', '3:4', '16:9', '9:16']
const aspectRatioOptions = ASPECT_RATIOS.map((ratio) => ({
  value: ratio,
  label: t(`creative.aspects.${ratio.replace(':', 'x')}`),
}))

const outputCountOptions = [1, 2, 3, 4].map((count) => ({
  value: count,
  label: t('creative.panel.outputCountOption', { n: count }),
}))

function onModelChange(value: string | number | boolean | null): void {
  studio.selectOption(String(value ?? ''))
}

function onOperationChange(value: string | number | boolean | null): void {
  studio.operation.value = String(value ?? 'generate')
}

function onPromptChange(value: string): void {
  studio.prompt.value = value
}

function onImageSizeChange(value: string | number | boolean | null): void {
  studio.imageSize.value = String(value ?? '')
}

function onAspectRatioChange(value: string | number | boolean | null): void {
  studio.aspectRatio.value = String(value ?? '1:1')
}

function onOutputCountChange(value: string | number | boolean | null): void {
  studio.outputCount.value = Number(value ?? 1)
}

// 选择文件后全部进入裁剪队列
function onFilesPicked(event: Event): void {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files ?? []).filter((file) => file.type.startsWith('image/'))
  input.value = ''
  if (!files.length) return
  cropQueue.value = [...cropQueue.value, ...files]
}

// 裁剪确认（跳过也走这里，直接保留原图）：交给画布放到当前视角中心
function onCropConfirm(blob: Blob): void {
  emit('uploaded', blob)
  cropQueue.value = cropQueue.value.slice(1)
}

// 取消裁剪：丢弃剩余队列
function onCropCancel(): void {
  cropQueue.value = []
}
</script>

<style scoped>
.panel-label {
  @apply mb-1.5 block text-xs font-medium text-gray-500 dark:text-dark-400;
}

/* 拖拽调整 prompt 框大小时禁用过渡并限制最大高度，避免拖拽卡顿 */
:deep(.creative-prompt textarea) {
  max-height: 40vh;
  transition: none;
}

.panel-upload-btn {
  @apply inline-flex shrink-0 items-center gap-1.5 rounded-control border border-primary-900/10 bg-white px-3 text-xs text-gray-600 transition-colors;
  /* 内联高度不依赖 Tailwind 规则，避免样式表不同步时塌缩；shrink-0 防止面板溢出时被 flex 压缩 */
  height: 44px;
  @apply hover:border-black/20 hover:text-gray-900;
  @apply dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-600 dark:hover:text-gray-100;
}
</style>
