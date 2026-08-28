<template>
  <div class="flex h-full min-h-0 flex-col">
    <!-- 工具栏 -->
    <div class="flex flex-wrap items-center gap-2 border-b border-primary-900/10 p-2 dark:border-dark-600">
      <button type="button" class="canvas-tool-btn" :title="t('creative.canvas.addText')" @click="addText">
        <Icon name="modalityText" size="sm" />
      </button>
      <button
        type="button"
        class="canvas-tool-btn"
        :class="maskMode && 'canvas-tool-btn-active'"
        :title="t('creative.canvas.maskMode')"
        @click="setMaskMode(!maskMode)"
      >
        <Icon name="edit" size="sm" />
      </button>
      <button
        type="button"
        class="canvas-tool-btn"
        :class="!maskLayerVisible && 'canvas-tool-btn-active'"
        :disabled="!hasMaskObjects"
        :title="t('creative.canvas.toggleMaskLayer')"
        @click="toggleMaskLayer()"
      >
        <Icon :name="maskLayerVisible ? 'eye' : 'eyeOff'" size="sm" />
      </button>
      <span class="mx-1 h-5 w-px bg-primary-900/10 dark:bg-dark-600"></span>
      <button
        type="button"
        class="canvas-tool-btn"
        :class="cropMode && 'canvas-tool-btn-active'"
        :title="t('creative.canvas.cropMode')"
        @click="setCropMode(!cropMode)"
      >
        <Icon name="copy" size="sm" />
      </button>
      <button type="button" class="canvas-tool-btn" :disabled="!selectedObject" :title="t('creative.canvas.rotate90')" @click="rotateSelection">
        <Icon name="refresh" size="sm" />
      </button>
      <button type="button" class="canvas-tool-btn" :disabled="!selectedObject" :title="t('creative.canvas.flipHorizontal')" @click="flipSelection('horizontal')">
        <Icon name="swap" size="sm" />
      </button>
      <button type="button" class="canvas-tool-btn" :disabled="!selectedObject" :title="t('creative.canvas.flipVertical')" @click="flipSelection('vertical')">
        <Icon name="arrowsUpDown" size="sm" />
      </button>
      <span class="mx-1 h-5 w-px bg-primary-900/10 dark:bg-dark-600"></span>
      <button type="button" class="canvas-tool-btn" :disabled="!undoStack.length" :title="t('creative.canvas.undo')" @click="undo">
        <Icon name="arrowLeft" size="sm" />
      </button>
      <button type="button" class="canvas-tool-btn" :disabled="!redoStack.length" :title="t('creative.canvas.redo')" @click="redo">
        <Icon name="arrowRight" size="sm" />
      </button>
      <button type="button" class="canvas-tool-btn" :title="t('creative.canvas.reset')" @click="resetCanvas">
        <Icon name="trash" size="sm" />
      </button>
      <span class="mx-1 h-5 w-px bg-primary-900/10 dark:bg-dark-600"></span>
      <button type="button" class="canvas-tool-btn" :title="t('creative.canvas.exportComposite')" @click="emitComposite">
        <Icon name="download" size="sm" />
      </button>
      <span v-if="maskMode" class="ml-auto rounded bg-primary-600/10 px-2 py-1 text-xs text-primary-700 dark:text-primary-300">
        {{ t('creative.canvas.maskModeHint') }}
      </span>
      <span v-else-if="cropMode" class="ml-auto rounded bg-primary-600/10 px-2 py-1 text-xs text-primary-700 dark:text-primary-300">
        {{ t('creative.canvas.cropModeHint') }}
      </span>
    </div>

    <div class="flex min-h-0 flex-1">
      <!-- 画布区：深色工作台底板，导出内容保持透明底 -->
      <div ref="containerRef" class="relative min-w-0 flex-1 overflow-hidden bg-[#1e1f24]">
        <canvas ref="canvasElRef" class="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2"></canvas>
      </div>

      <!-- 图层面板 -->
      <div class="hidden w-48 flex-shrink-0 flex-col border-l border-primary-900/10 dark:border-dark-600 md:flex">
        <div class="border-b border-primary-900/10 px-3 py-2 text-xs font-medium text-gray-500 dark:border-dark-600 dark:text-dark-400">
          {{ t('creative.canvas.layers') }}
        </div>
        <div class="min-h-0 flex-1 overflow-y-auto p-1">
          <div
            v-for="layer in reversedLayers"
            :key="layer.id"
            class="group flex cursor-pointer items-center gap-1 rounded px-2 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-dark-800"
            :class="selectedObject === layer.object && 'bg-gray-100 dark:bg-dark-800'"
            @click="selectLayer(layer.object)"
            @dblclick="editLayerText(layer.object)"
          >
            <button
              type="button"
              class="flex-shrink-0 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
              @click.stop="toggleLayerVisible(layer.object)"
            >
              <Icon :name="layer.visible ? 'eye' : 'eyeOff'" size="sm" />
            </button>
            <span class="min-w-0 flex-1 truncate text-gray-700 dark:text-gray-300">{{ layer.label }}</span>
            <button type="button" class="flex-shrink-0 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 md:opacity-0 md:group-hover:opacity-100" @click.stop="moveLayer(layer.object, 1)">
              <Icon name="chevronUp" size="sm" />
            </button>
            <button type="button" class="flex-shrink-0 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 md:opacity-0 md:group-hover:opacity-100" @click.stop="moveLayer(layer.object, -1)">
              <Icon name="chevronDown" size="sm" />
            </button>
            <button type="button" class="flex-shrink-0 text-gray-400 hover:text-red-500 md:opacity-0 md:group-hover:opacity-100" @click.stop="removeLayer(layer.object)">
              <Icon name="x" size="sm" />
            </button>
          </div>
          <p v-if="!layers.length" class="px-3 py-4 text-center text-xs text-gray-400 dark:text-dark-500">
            {{ t('creative.canvas.noLayers') }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * 创作台画布（fabric 7）
 * - 图片 / 文字对象可拖动、缩放、旋转、翻转
 * - mask 画笔模式：白色画笔轨迹单独标记为 mask 层，可显隐，可导出透明底 PNG
 * - 裁剪模式：拖出选区后对命中的图片应用 clipPath
 * - 撤销 / 重做 / 重置 / 导出合成图，场景 JSON 自动存入 IndexedDB
 */
import { computed, markRaw, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Canvas, FabricImage, IText, PencilBrush, Rect, type FabricObject } from 'fabric'
import Icon from '@/components/icons/Icon.vue'
import { loadSceneJson, saveSceneJson } from '@/utils/creativeLocalStore'

interface Props {
  aspectRatio?: string
}

interface Emits {
  (e: 'loaded'): void
  (e: 'composite', blob: Blob): void
  (e: 'error', message: string): void
}

const props = withDefaults(defineProps<Props>(), { aspectRatio: '1:1' })
const emit = defineEmits<Emits>()
const { t } = useI18n()

// 场景快照在本地库中的 key
const SCENE_KEY = 'creative:canvas'
// 快照栈上限
const HISTORY_LIMIT = 50
// 场景自动保存防抖
const SCENE_SAVE_DEBOUNCE = 800

// 画幅 → 逻辑尺寸（导出图按此尺寸）
const CANVAS_SIZES: Record<string, [number, number]> = {
  '1:1': [1024, 1024],
  '4:3': [1152, 864],
  '3:4': [864, 1152],
  '16:9': [1280, 720],
  '9:16': [720, 1280],
}

interface LayerItem {
  id: number
  object: FabricObject
  label: string
  visible: boolean
  isText: boolean
}

const containerRef = ref<HTMLDivElement | null>(null)
const canvasElRef = ref<HTMLCanvasElement | null>(null)
const maskMode = ref(false)
const cropMode = ref(false)
const maskLayerVisible = ref(true)
// shallowRef：元素里的 fabric 实例不能被 Vue 深度代理，否则类型与引用比较都会失真
const layers = shallowRef<LayerItem[]>([])
const selectedObject = shallowRef<FabricObject | null>(null)
const undoStack = ref<string[]>([])
const redoStack = ref<string[]>([])

const reversedLayers = computed(() => [...layers.value].reverse())
const hasMaskObjects = computed(() => layers.value.some((l) => isMaskObject(l.object)))

let canvas: Canvas | null = null
let resizeObserver: ResizeObserver | null = null
let layerSeq = 0
let historyPaused = false
let sceneSaveTimer: ReturnType<typeof setTimeout> | null = null
// mask 模式下被锁定对象的原始交互状态
let lockedStates: Map<FabricObject, { selectable: boolean; evented: boolean }> | null = null
// 裁剪框橡胶带
let cropRect: Rect | null = null
let cropStart: { x: number; y: number } | null = null

// fabric 7 类型未声明自定义 data 属性，运行时允许挂任意键，这里做最小封装
type ObjectWithData = { data?: Record<string, unknown> }

function objectData(object: FabricObject): Record<string, unknown> {
  return (object as unknown as ObjectWithData).data ?? {}
}

function setObjectData(object: FabricObject, data: Record<string, unknown>): void {
  const target = object as unknown as ObjectWithData
  target.data = data
}

// ==================== 初始化 ====================

onMounted(() => {
  if (!canvasElRef.value) return
  const [width, height] = logicalSize()
  canvas = new Canvas(canvasElRef.value, {
    width,
    height,
    preserveObjectStacking: true,
    // 背景透明，导出的合成图 / mask 都带 alpha
    backgroundColor: '',
  })
  bindCanvasEvents()
  fitCanvasCss()
  resizeObserver = new ResizeObserver(() => fitCanvasCss())
  if (containerRef.value) resizeObserver.observe(containerRef.value)
  void restoreScene()
  emit('loaded')
})

onBeforeUnmount(() => {
  if (sceneSaveTimer) clearTimeout(sceneSaveTimer)
  resizeObserver?.disconnect()
  resizeObserver = null
  if (canvas) {
    persistSceneNow()
    canvas.dispose()
    canvas = null
  }
})

watch(
  () => props.aspectRatio,
  () => {
    if (!canvas) return
    const [width, height] = logicalSize()
    // 仅改底板尺寸，已有对象保留
    canvas.setDimensions({ width, height }, { backstoreOnly: true })
    fitCanvasCss()
  },
)

function logicalSize(): [number, number] {
  return CANVAS_SIZES[props.aspectRatio] ?? CANVAS_SIZES['1:1']
}

// 画布 CSS 尺寸跟随容器，保持完整可见
function fitCanvasCss(): void {
  if (!canvas || !containerRef.value) return
  const { clientWidth, clientHeight } = containerRef.value
  if (clientWidth <= 0 || clientHeight <= 0) return
  const [logicalW, logicalH] = logicalSize()
  const scale = Math.min(clientWidth / logicalW, clientHeight / logicalH)
  canvas.setDimensions(
    { width: Math.floor(logicalW * scale), height: Math.floor(logicalH * scale) },
    { cssOnly: true },
  )
}

// ==================== 事件绑定 ====================

function bindCanvasEvents(): void {
  if (!canvas) return
  canvas.on('object:added', onSceneMutated)
  canvas.on('object:modified', onSceneMutated)
  canvas.on('object:removed', onSceneMutated)
  // 画笔落笔完成：mask 模式下标记为 mask 层对象
  canvas.on('path:created', (event: any) => {
    const path = event.path as FabricObject | undefined
    if (path && maskMode.value) {
      setObjectData(path, { ...objectData(path), mask: true })
      path.set({ selectable: false, evented: false })
    }
    onSceneMutated()
  })
  canvas.on('selection:created', (event: any) => {
    selectedObject.value = (event.selected?.[0] as FabricObject) ?? null
  })
  canvas.on('selection:updated', (event: any) => {
    selectedObject.value = (event.selected?.[0] as FabricObject) ?? null
  })
  canvas.on('selection:cleared', () => {
    selectedObject.value = null
  })
  // 双击文字层进入编辑
  canvas.on('mouse:dblclick', (event: any) => {
    const target = event.target as FabricObject | undefined
    if (target && target instanceof IText) {
      canvas?.setActiveObject(target)
      target.enterEditing()
    }
  })
  // 裁剪模式 rubber band
  canvas.on('mouse:down', (event: any) => {
    if (!cropMode.value || !canvas) return
    const point = canvas.getScenePoint(event.e)
    cropStart = { x: point.x, y: point.y }
    cropRect = new Rect({
      left: point.x,
      top: point.y,
      width: 0,
      height: 0,
      fill: 'rgba(255,255,255,0.15)',
      stroke: '#ffffff',
      strokeWidth: 1,
      strokeDashArray: [4, 4],
      selectable: false,
      evented: false,
    })
    canvas.add(cropRect)
  })
  canvas.on('mouse:move', (event: any) => {
    if (!cropMode.value || !cropRect || !cropStart || !canvas) return
    const point = canvas.getScenePoint(event.e)
    cropRect.set({
      left: Math.min(point.x, cropStart.x),
      top: Math.min(point.y, cropStart.y),
      width: Math.abs(point.x - cropStart.x),
      height: Math.abs(point.y - cropStart.y),
    })
    canvas.requestRenderAll()
  })
  canvas.on('mouse:up', () => {
    if (!cropMode.value) return
    finalizeCrop()
  })
}

function onSceneMutated(): void {
  refreshLayers()
  pushHistory()
}

// ==================== 图层 ====================

function isMaskObject(object: FabricObject): boolean {
  return objectData(object).mask === true
}

function layerLabel(object: FabricObject): string {
  if (object instanceof IText) {
    const text = object.text?.trim() || t('creative.canvas.textLayer')
    return text.length > 12 ? `${text.slice(0, 12)}…` : text
  }
  if (object instanceof FabricImage) return t('creative.canvas.imageLayer')
  if (isMaskObject(object)) return t('creative.canvas.maskLayer')
  return object.type ?? t('creative.canvas.objectLayer')
}

function refreshLayers(): void {
  if (!canvas) return
  layerSeq = 0
  layers.value = canvas
    .getObjects()
    .filter((object) => object !== cropRect)
    .map((object) => ({
      // markRaw 避免 Vue 把 fabric 实例代理成 reactive，破坏类型与 instanceof
      id: ++layerSeq,
      object: markRaw(object),
      label: layerLabel(object),
      visible: !!object.visible,
      isText: object instanceof IText,
    }))
}

function selectLayer(object: FabricObject): void {
  if (!canvas || !object.visible) return
  canvas.setActiveObject(object)
  canvas.requestRenderAll()
  selectedObject.value = object
}

function editLayerText(object: FabricObject): void {
  if (!(object instanceof IText) || !canvas) return
  canvas.setActiveObject(object)
  object.enterEditing()
}

function toggleLayerVisible(object: FabricObject): void {
  object.visible = !object.visible
  canvas?.requestRenderAll()
  refreshLayers()
}

function moveLayer(object: FabricObject, direction: 1 | -1): void {
  if (!canvas) return
  if (direction > 0) canvas.bringObjectForward(object)
  else canvas.sendObjectBackwards(object)
  canvas.requestRenderAll()
  pushHistory()
}

function removeLayer(object: FabricObject): void {
  if (!canvas) return
  if (selectedObject.value === object) selectedObject.value = null
  canvas.remove(object)
}

// ==================== 对象操作 ====================

function addText(): void {
  if (!canvas) return
  const [width] = logicalSize()
  const text = new IText(t('creative.canvas.defaultText'), {
    left: width / 2,
    top: 120,
    originX: 'center',
    originY: 'center',
    fill: '#ffffff',
    fontSize: 48,
  })
  canvas.add(text)
  canvas.setActiveObject(text)
  text.enterEditing()
  canvas.requestRenderAll()
}

function rotateSelection(): void {
  const target = selectedObject.value
  if (!target || !canvas) return
  target.rotate((target.angle + 90) % 360)
  canvas.requestRenderAll()
  pushHistory()
}

function flipSelection(direction: 'horizontal' | 'vertical'): void {
  const target = selectedObject.value
  if (!target || !canvas) return
  if (direction === 'horizontal') target.set('flipX', !target.flipX)
  else target.set('flipY', !target.flipY)
  canvas.requestRenderAll()
  pushHistory()
}

// ==================== mask 画笔模式 ====================

function setMaskMode(on: boolean): void {
  if (!canvas) return
  // 与裁剪模式互斥
  if (on && cropMode.value) setCropMode(false)
  maskMode.value = on
  if (on) {
    // 锁定已有对象，画笔不触发选中/拖动
    canvas.discardActiveObject()
    lockedStates = new Map()
    canvas.getObjects().forEach((object) => {
      lockedStates?.set(object, { selectable: object.selectable, evented: object.evented })
      object.set({ selectable: false, evented: false })
    })
    const brush = new PencilBrush(canvas)
    brush.color = '#ffffff'
    brush.width = 28
    canvas.freeDrawingBrush = brush
    canvas.isDrawingMode = true
  } else {
    canvas.isDrawingMode = false
    canvas.freeDrawingBrush = undefined
    lockedStates?.forEach((state, object) => {
      object.set({ selectable: state.selectable, evented: state.evented })
    })
    lockedStates = null
  }
  canvas.requestRenderAll()
}

function toggleMaskLayer(): void {
  if (!canvas) return
  maskLayerVisible.value = !maskLayerVisible.value
  canvas.getObjects().forEach((object) => {
    if (isMaskObject(object)) object.visible = maskLayerVisible.value
  })
  canvas.requestRenderAll()
  refreshLayers()
}

/**
 * 导出 mask：隐藏其它对象后按画布逻辑尺寸导出透明底 PNG（白色画笔轨迹）
 */
async function exportMaskBlob(): Promise<Blob | null> {
  if (!canvas) return null
  const maskObjects = canvas.getObjects().filter(isMaskObject)
  if (!maskObjects.length) return null
  const restore: FabricObject[] = []
  const restoreMask: FabricObject[] = []
  try {
    canvas.getObjects().forEach((object) => {
      if (isMaskObject(object)) {
        if (!object.visible) {
          object.visible = true
          restoreMask.push(object)
        }
      } else if (object.visible) {
        object.visible = false
        restore.push(object)
      }
    })
    const element = canvas.toCanvasElement(1)
    return await new Promise<Blob | null>((resolve) => element.toBlob(resolve, 'image/png'))
  } finally {
    restore.forEach((object) => {
      object.visible = true
    })
    restoreMask.forEach((object) => {
      object.visible = false
    })
    canvas.requestRenderAll()
  }
}

// ==================== 裁剪模式 ====================

function setCropMode(on: boolean): void {
  if (!canvas) return
  // 与 mask 模式互斥
  if (on && maskMode.value) setMaskMode(false)
  cropMode.value = on
  if (on) {
    canvas.discardActiveObject()
    canvas.selection = false
    canvas.defaultCursor = 'crosshair'
    canvas.getObjects().forEach((object) => {
      object.evented = false
    })
  } else {
    canvas.selection = true
    canvas.defaultCursor = 'default'
    removeCropRect()
    canvas.getObjects().forEach((object) => {
      if (!isMaskObject(object)) object.evented = true
    })
  }
  canvas.requestRenderAll()
}

function removeCropRect(): void {
  if (cropRect && canvas) {
    canvas.remove(cropRect)
  }
  cropRect = null
  cropStart = null
}

// 落笔结束：选区足够大时对命中的图片应用 clipPath
function finalizeCrop(): void {
  if (!canvas || !cropRect) return
  const rect = cropRect
  const width = (rect.width || 0) * (rect.scaleX || 1)
  const height = (rect.height || 0) * (rect.scaleY || 1)
  removeCropRect()
  if (width < 10 || height < 10) {
    canvas.requestRenderAll()
    return
  }
  const centerX = (rect.left || 0) + width / 2
  const centerY = (rect.top || 0) + height / 2
  // 命中最上层被选区中心覆盖的图片
  const candidates = canvas
    .getObjects()
    .filter((object) => object instanceof FabricImage && object.visible)
    .reverse()
  const target = candidates.find((object) => {
    const bounds = object.getBoundingRect()
    return (
      centerX >= bounds.left &&
      centerX <= bounds.left + bounds.width &&
      centerY >= bounds.top &&
      centerY <= bounds.top + bounds.height
    )
  })
  if (target) {
    const clip = new Rect({
      left: rect.left,
      top: rect.top,
      width,
      height,
      // 绝对定位：裁剪区域固定在画布坐标系，图片后续仍可移动
      absolutePositioned: true,
    })
    target.set('clipPath', clip)
    pushHistory()
  }
  setCropMode(false)
}

// ==================== 历史快照 ====================

function snapshot(): string {
  // fabric 7 的 toJSON 不带 propertiesToInclude，用 toObject 序列化自定义 data 字段
  return JSON.stringify(canvas?.toObject(['data']) ?? {})
}

function pushHistory(): void {
  if (!canvas || historyPaused) return
  undoStack.value.push(snapshot())
  if (undoStack.value.length > HISTORY_LIMIT) undoStack.value.shift()
  redoStack.value = []
  scheduleSceneSave()
}

async function loadSnapshot(json: string): Promise<void> {
  if (!canvas) return
  historyPaused = true
  try {
    await canvas.loadFromJSON(json)
    refreshLayers()
    canvas.requestRenderAll()
    scheduleSceneSave()
  } finally {
    historyPaused = false
  }
}

function undo(): void {
  const previous = undoStack.value.pop()
  if (!previous || !canvas) return
  redoStack.value.push(snapshot())
  void loadSnapshot(previous)
}

function redo(): void {
  const next = redoStack.value.pop()
  if (!next || !canvas) return
  undoStack.value.push(snapshot())
  void loadSnapshot(next)
}

function resetCanvas(): void {
  if (!canvas) return
  canvas.clear()
  canvas.backgroundColor = ''
  undoStack.value = []
  redoStack.value = []
  selectedObject.value = null
  refreshLayers()
  scheduleSceneSave()
}

// ==================== 场景持久化 ====================

function scheduleSceneSave(): void {
  if (sceneSaveTimer) clearTimeout(sceneSaveTimer)
  sceneSaveTimer = setTimeout(() => persistSceneNow(), SCENE_SAVE_DEBOUNCE)
}

function persistSceneNow(): void {
  if (!canvas) return
  const json = snapshot()
  void saveSceneJson(SCENE_KEY, json).catch((error) => {
    console.error('Failed to persist creative scene:', error)
  })
}

async function restoreScene(): Promise<void> {
  if (!canvas) return
  try {
    const json = await loadSceneJson(SCENE_KEY)
    if (json) {
      historyPaused = true
      await canvas.loadFromJSON(json)
      historyPaused = false
    }
  } catch (error) {
    console.error('Failed to restore creative scene:', error)
  } finally {
    historyPaused = false
    refreshLayers()
    fitCanvasCss()
    canvas.requestRenderAll()
  }
}

// ==================== 对外能力 ====================

// 加载图片（源图 / 生成结果 / 导出合成图二次编辑）
async function loadBlobAsImage(blob: Blob): Promise<void> {
  if (!canvas) return
  const url = URL.createObjectURL(blob)
  try {
    const image = await FabricImage.fromURL(url)
    const [canvasW, canvasH] = logicalSize()
    const scale = Math.min(canvasW / (image.width || canvasW), canvasH / (image.height || canvasH), 1) * 0.8
    image.set({
      left: canvasW / 2,
      top: canvasH / 2,
      originX: 'center',
      originY: 'center',
      scaleX: scale,
      scaleY: scale,
    })
    canvas.add(image)
    canvas.setActiveObject(image)
    canvas.requestRenderAll()
    emit('loaded')
  } catch (error) {
    console.error('Failed to load image into canvas:', error)
    emit('error', t('creative.error.loadImageFailed'))
  } finally {
    URL.revokeObjectURL(url)
  }
}

// 导出合成图：mask 画笔轨迹与裁剪框不属于画面内容，导出时临时隐藏
async function exportCompositeBlob(): Promise<Blob | null> {
  if (!canvas) return null
  const hidden: FabricObject[] = []
  try {
    canvas.getObjects().forEach((object) => {
      if (isMaskObject(object) && object.visible) {
        object.visible = false
        hidden.push(object)
      }
    })
    if (cropRect) cropRect.visible = false
    return await canvas.toBlob({ format: 'png', multiplier: 1 })
  } finally {
    hidden.forEach((object) => {
      object.visible = true
    })
    if (cropRect) cropRect.visible = true
    canvas.requestRenderAll()
  }
}

function emitComposite(): void {
  void exportCompositeBlob().then((blob) => {
    if (blob) emit('composite', blob)
  })
}

// 父组件生成时取 mask：进入过 mask 模式即导出当前轨迹
async function getMaskBlob(): Promise<Blob | null> {
  if (maskMode.value) setMaskMode(false)
  return exportMaskBlob()
}

defineExpose({
  loadBlobAsImage,
  exportCompositeBlob,
  exportMaskBlob,
  getMaskBlob,
  resetCanvas,
})
</script>

<style scoped>
.canvas-tool-btn {
  @apply inline-flex h-8 w-8 items-center justify-center rounded-md border border-primary-900/10 bg-white text-gray-600 transition-colors;
  @apply hover:border-black/20 hover:text-gray-900;
  @apply disabled:cursor-not-allowed disabled:opacity-40;
  @apply dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-600 dark:hover:text-gray-100;
}

.canvas-tool-btn-active {
  @apply border-primary-500 bg-primary-600/10 text-primary-700 dark:text-primary-300;
}
</style>
