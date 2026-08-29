<template>
  <!-- 聊天式输入框：底部居中或跟随选中图片；左下调参入口，右下费用 + 发送 -->
  <div
    ref="rootRef"
    class="relative w-[min(600px,calc(100vw-2rem))] rounded-2xl border border-primary-900/10 bg-white/95 shadow-xl backdrop-blur dark:border-dark-600 dark:bg-dark-900/95"
  >
    <!-- 提示词输入区（高度随内容自适应，上限约 6 行） -->
    <div class="relative">
      <textarea
        ref="textareaRef"
        v-model="prompt"
        rows="2"
        class="composer-textarea w-full resize-none bg-transparent px-4 pb-1.5 pt-3.5 text-sm leading-relaxed text-gray-900 outline-none placeholder:text-gray-400 dark:text-gray-100 dark:placeholder:text-dark-400"
        :class="studio.busy.value && 'opacity-60'"
        :placeholder="t('creative.panel.promptPlaceholder')"
        @input="autosize"
        @keydown="onKeydown"
      ></textarea>
      <span
        class="pointer-events-none absolute right-3 top-2 text-[10px] tabular-nums"
        :class="promptLength > PROMPT_MAX ? 'text-red-500' : 'text-gray-300 dark:text-dark-500'"
      >
        {{ promptLength }}/{{ PROMPT_MAX }}
      </span>
    </div>

    <!-- 前置提示：图生图 / 局部重绘未选中源图时引导（选中后输入框已跟随图片，无需再提示） -->
    <p v-if="operationHint" class="flex items-center gap-1.5 px-4 pb-1.5 text-xs text-amber-600 dark:text-amber-400">
      <Icon name="infoCircle" size="xs" class="flex-shrink-0" />
      {{ operationHint }}
    </p>
    <p v-if="studio.error.value" class="px-4 pb-1.5 text-xs text-red-600 dark:text-red-400">{{ studio.error.value }}</p>

    <!-- 底栏：左下 = 模型 / 参数 / 操作 三个调参入口；右下 = 预估费用 + 发送 -->
    <div class="flex items-center gap-1.5 px-3 pb-3">
      <button
        type="button"
        class="composer-chip"
        :class="openPanel === 'model' && 'composer-chip-active'"
        :title="t('creative.composer.model')"
        @click="togglePanel('model')"
      >
        <Icon name="sparkles" size="xs" class="flex-shrink-0" />
        <span class="max-w-28 truncate">{{ modelChipLabel }}</span>
        <Icon name="chevronUp" size="xs" class="flex-shrink-0 transition-transform" :class="openPanel !== 'model' && 'rotate-180'" />
      </button>
      <button
        type="button"
        class="composer-chip"
        :class="openPanel === 'params' && 'composer-chip-active'"
        :title="t('creative.composer.params')"
        @click="togglePanel('params')"
      >
        <Icon name="filter" size="xs" class="flex-shrink-0" />
        <span class="max-w-24 truncate">{{ paramsChipLabel }}</span>
        <Icon name="chevronUp" size="xs" class="flex-shrink-0 transition-transform" :class="openPanel !== 'params' && 'rotate-180'" />
      </button>
      <button
        type="button"
        class="composer-chip"
        :class="openPanel === 'operation' && 'composer-chip-active'"
        :title="t('creative.composer.operation')"
        @click="togglePanel('operation')"
      >
        <Icon name="swap" size="xs" class="flex-shrink-0" />
        <span class="max-w-24 truncate">{{ operationChipLabel }}</span>
        <Icon name="chevronUp" size="xs" class="flex-shrink-0 transition-transform" :class="openPanel !== 'operation' && 'rotate-180'" />
      </button>

      <div class="ml-auto flex items-center gap-2">
        <span v-if="studio.estimatedCost.value !== null" class="whitespace-nowrap text-[11px] text-gray-400 dark:text-dark-400">
          {{ t('creative.panel.estimatedCost', { cost: formatBalanceAmount(studio.estimatedCost.value, { fractionDigits: 3 }) }) }}
        </span>
        <button
          type="button"
          class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full bg-primary-600 text-white transition-colors hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-40"
          :disabled="!studio.canGenerate.value"
          :title="t('creative.composer.send')"
          @click="emit('generate')"
        >
          <Icon v-if="studio.busy.value" name="refresh" size="sm" class="animate-spin" />
          <Icon v-else name="arrowUp" size="sm" />
        </button>
      </div>
    </div>

    <!-- 展开面板：模型 / 参数 / 操作（点击外部自动收起） -->
    <div
      v-if="openPanel"
      class="absolute bottom-full left-3 z-30 mb-2 w-[min(320px,calc(100vw-3.5rem))] overflow-hidden rounded-xl border border-primary-900/10 bg-white/95 shadow-xl backdrop-blur dark:border-dark-600 dark:bg-dark-900/95"
    >
      <!-- 模型列表：分组 + 模型名单选 -->
      <div v-if="openPanel === 'model'" class="max-h-72 overflow-y-auto p-1.5">
        <p v-if="showModelsEmptyHint" class="rounded-md bg-primary-900/5 px-3 py-2 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-400">
          {{ modelsEmptyHintText }}
        </p>
        <button
          v-for="option in studio.models.value"
          :key="creativeOptionKey(option)"
          type="button"
          class="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
          :class="studio.selectedOptionKey.value === creativeOptionKey(option) && 'bg-primary-600/5 dark:bg-primary-900/20'"
          @click="selectModel(option)"
        >
          <span class="min-w-0 flex-1">
            <span class="block truncate text-xs font-medium text-gray-800 dark:text-gray-100">{{ option.model }}</span>
            <span class="block truncate text-[11px] text-gray-400 dark:text-dark-400">{{ option.group_name }}</span>
          </span>
          <Icon v-if="studio.selectedOptionKey.value === creativeOptionKey(option)" name="check" size="sm" class="flex-shrink-0 text-primary-600 dark:text-primary-300" />
        </button>
      </div>

      <!-- 参数：尺寸 / 比例 / 数量，chips 单选 -->
      <div v-else-if="openPanel === 'params'" class="space-y-3 p-3">
        <div>
          <p class="param-label">{{ t('creative.panel.imageSize') }}</p>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="size in studio.imageSizeOptions.value"
              :key="size"
              type="button"
              class="param-chip"
              :class="studio.imageSize.value === size && 'param-chip-active'"
              @click="setImageSize(size)"
            >
              {{ size }}
            </button>
            <span v-if="!studio.imageSizeOptions.value.length" class="text-[11px] text-gray-400 dark:text-dark-400">—</span>
          </div>
        </div>
        <div>
          <p class="param-label">{{ t('creative.panel.aspectRatio') }}</p>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="ratio in ASPECT_RATIOS"
              :key="ratio"
              type="button"
              class="param-chip"
              :class="studio.aspectRatio.value === ratio && 'param-chip-active'"
              @click="setAspectRatio(ratio)"
            >
              {{ t(`creative.aspects.${ratio.replace(':', 'x')}`) }}
            </button>
          </div>
        </div>
        <div>
          <p class="param-label">{{ t('creative.panel.outputCount') }}</p>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="count in [1, 2, 3, 4]"
              :key="count"
              type="button"
              class="param-chip"
              :class="studio.outputCount.value === count && 'param-chip-active'"
              @click="setOutputCount(count)"
            >
              {{ t('creative.panel.outputCountOption', { n: count }) }}
            </button>
          </div>
        </div>
      </div>

      <!-- 操作：文生图 / 图生图 / 局部重绘，带说明 -->
      <div v-else class="p-1.5">
        <p v-if="!studio.operationOptions.value.length" class="px-2.5 py-2 text-[11px] text-gray-400 dark:text-dark-400">
          {{ t('creative.composer.selectModelFirst') }}
        </p>
        <button
          v-for="op in studio.operationOptions.value"
          :key="op"
          type="button"
          class="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
          :class="studio.operation.value === op && 'bg-primary-600/5 dark:bg-primary-900/20'"
          @click="selectOperation(op)"
        >
          <span class="min-w-0 flex-1">
            <span class="block text-xs font-medium text-gray-800 dark:text-gray-100">{{ t(`creative.operations.${op}`, op) }}</span>
            <span class="block text-[11px] text-gray-400 dark:text-dark-400">{{ t(`creative.operationsDesc.${op}`) }}</span>
          </span>
          <Icon v-if="studio.operation.value === op" name="check" size="sm" class="flex-shrink-0 text-primary-600 dark:text-primary-300" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * 创作台聊天式输入框（替代旧左侧面板）：
 * - 主体为提示词输入区 + 右下圆形发送按钮；左下三个调参 chip 展开模型 / 参数 / 操作面板
 * - 位置由父级控制（底部居中或跟随选中图片），本组件只负责内容与发送
 * - 状态全部经由 props 传入的 studio（useCreativeStudio 返回值）读写
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { onClickOutside } from '@vueuse/core'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import type { useCreativeStudio } from '@/composables/useCreativeStudio'
import { creativeOptionKey } from '@/composables/useCreativeStudio'
import type { CreativeModelOption, CreativeOperation } from '@/api/creative'

type Studio = ReturnType<typeof useCreativeStudio>

interface Props {
  studio: Studio
  // 画布当前是否有选中的图片（用于图生图 / 局部重绘的前置提示）
  hasSelection: boolean
}

interface Emits {
  (e: 'generate'): void
}

const props = defineProps<Props>()
// 本地别名：studio 为 props 传入的共享状态机，子组件经它读写
const studio = props.studio
const emit = defineEmits<Emits>()
const { t } = useI18n()
const appStore = useAppStore()
const { formatBalanceAmount } = useBalanceDisplay()

const PROMPT_MAX = 8000
// 输入框自适应高度上限（约 6 行）
const TEXTAREA_MAX_HEIGHT = 160

const ASPECT_RATIOS = ['1:1', '4:3', '3:4', '16:9', '9:16']

const rootRef = ref<HTMLDivElement | null>(null)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
// 当前展开的调参面板（同时只开一个）
const openPanel = ref<'model' | 'params' | 'operation' | null>(null)

// 点击输入框外部时收起调参面板
onClickOutside(rootRef, () => {
  openPanel.value = null
})

const prompt = computed({
  get: () => studio.prompt.value,
  set: (value: string) => {
    studio.prompt.value = value
  },
})

const promptLength = computed(() => studio.prompt.value.length)

// 图生图 / 局部重绘且未选中图片时的引导提示
const operationHint = computed(() => {
  if (props.hasSelection || !studio.selectedOption.value) return ''
  if (studio.operation.value === 'edit') return t('creative.panel.selectImageHint')
  if (studio.operation.value === 'inpaint') return t('creative.panel.maskHint')
  return ''
})

// 模型目录为空时的空态提示（加载失败时 models 同样为空，伴随 error 红条展示）
const showModelsEmptyHint = computed(
  () => !studio.loadingModels.value && studio.models.value.length === 0,
)

// 功能被管理员关闭时提示联系管理员开启，否则提示分组未配置图片生成
const modelsEmptyHintText = computed(() =>
  appStore.cachedPublicSettings?.creative_enabled === false
    ? t('creative.panel.studioDisabled')
    : t('creative.panel.noModelsAvailable'),
)

// 三个 chip 的当前值标签
const modelChipLabel = computed(() => {
  const option = studio.selectedOption.value
  return option ? option.model : t('creative.composer.selectModel')
})
const paramsChipLabel = computed(() => t('creative.composer.params'))
const operationChipLabel = computed(() => t(`creative.operations.${studio.operation.value}`, studio.operation.value))

function togglePanel(panel: 'model' | 'params' | 'operation'): void {
  openPanel.value = openPanel.value === panel ? null : panel
}

// 选择模型后收起面板；参数面板支持连续调整，不自动收起
function selectModel(option: CreativeModelOption): void {
  studio.selectOption(creativeOptionKey(option))
  openPanel.value = null
}

function selectOperation(op: CreativeOperation): void {
  studio.operation.value = op
  openPanel.value = null
}

// 参数 chips 选择（经别名写入，避免模板内联赋值触发 prop mutation 校验）
function setImageSize(size: string): void {
  studio.imageSize.value = size
}

function setAspectRatio(ratio: string): void {
  studio.aspectRatio.value = ratio
}

function setOutputCount(count: number): void {
  studio.outputCount.value = count
}

// Ctrl / Cmd + Enter 发送；普通 Enter 换行
function onKeydown(event: KeyboardEvent): void {
  if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
    event.preventDefault()
    if (studio.canGenerate.value) emit('generate')
  }
}

// 输入框高度随内容自适应（不超过上限）
function autosize(): void {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, TEXTAREA_MAX_HEIGHT)}px`
}
</script>

<style scoped>
.composer-textarea {
  max-height: 160px;
  overflow-y: auto;
}

.composer-chip {
  @apply inline-flex h-8 items-center gap-1 rounded-full border border-primary-900/10 bg-white px-2.5 text-xs text-gray-600 transition-colors;
  @apply hover:border-black/20 hover:text-gray-900;
  @apply dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-400 dark:hover:text-gray-100;
}

.composer-chip-active {
  @apply border-primary-500/50 text-primary-700 dark:border-primary-500/50 dark:text-primary-300;
}

.param-label {
  @apply mb-1.5 text-[11px] font-medium text-gray-500 dark:text-dark-400;
}

.param-chip {
  @apply rounded-full border border-primary-900/10 px-2.5 py-1 text-[11px] text-gray-600 transition-colors;
  @apply hover:border-black/20 hover:text-gray-900;
  @apply dark:border-dark-600 dark:text-gray-300 dark:hover:border-dark-400 dark:hover:text-gray-100;
}

.param-chip-active {
  @apply border-primary-500 bg-primary-600/10 text-primary-700 dark:border-primary-500 dark:text-primary-300;
}
</style>
