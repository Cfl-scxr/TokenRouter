<template>
  <!-- 画布即整个背景：透明无边框，圆点网格铺满 -->
  <div ref="containerRef" class="dot-grid relative h-full w-full overflow-hidden">
    <canvas ref="canvasElRef"></canvas>

    <!-- 浮动工具栏（移动端底部，桌面端顶部）：上传 | 局部重绘画笔组 | 删除选中 / 清空 -->
    <div
      class="absolute bottom-3 left-1/2 z-10 flex -translate-x-1/2 items-center gap-1.5 rounded-full border border-primary-900/10 bg-white/90 px-2 py-1.5 shadow-md backdrop-blur dark:border-dark-600 dark:bg-dark-900/90 lg:bottom-auto lg:top-3"
    >
      <!-- 上传图片：裁剪确认后直接放上画布当前视角中心 -->
      <button type="button" class="canvas-tool-btn" :title="t('creative.panel.uploadSource')" @click="fileInputRef?.click()">
        <Icon name="upload" size="sm" />
      </button>
      <input
        ref="fileInputRef"
        type="file"
        accept="image/png,image/jpeg,image/webp"
        multiple
        class="hidden"
        @change="onFilesPicked"
      />
      <!-- 下载选中图片到本地 -->
      <button
        type="button"
        class="canvas-tool-btn"
        :disabled="!selectedImage"
        :title="t('creative.canvas.downloadSelected')"
        @click="downloadSelected"
      >
        <Icon name="download" size="sm" />
      </button>

      <!-- 画笔组：仅局部重绘模式可用（选中图片后自动进入涂抹，可用开关暂停去移动视角） -->
      <template v-if="isInpaint">
        <span class="mx-0.5 h-5 w-px bg-primary-900/10 dark:bg-dark-600"></span>
        <button
          type="button"
          class="canvas-tool-btn"
          :class="painting && 'canvas-tool-btn-active'"
          :title="painting ? t('creative.canvas.paintToggleOff') : t('creative.canvas.paintToggleOn')"
          @click="togglePainting"
        >
          <Icon name="edit" size="sm" />
        </button>
        <!-- 清除当前所有涂抹笔迹 -->
        <button
          type="button"
          class="canvas-tool-btn"
          :disabled="!hasMaskStrokes"
          :title="t('creative.canvas.clearMask')"
          @click="clearMask"
        >
          <Icon name="trash" size="sm" />
        </button>
        <!-- 撤销上一笔涂抹（同 Ctrl/Cmd+Z） -->
        <button
          type="button"
          class="canvas-tool-btn"
          :disabled="!canUndoMask"
          :title="t('creative.canvas.undoMask')"
          @click="undoMask"
        >
          <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M9 15L3 9m0 0l6-6M3 9h12a6 6 0 010 12h-3" />
          </svg>
        </button>
        <!-- 画笔粗细滑块：8–96（固定高度与工具栏按钮同高；轨道/滑块配色见 .brush-size） -->
        <div class="flex items-center gap-1.5 px-1">
          <input
            v-model.number="brushSize"
            type="range"
            min="8"
            max="96"
            step="1"
            class="brush-size h-8 w-16 cursor-pointer sm:w-24"
            :title="t('creative.canvas.brushSize')"
          />
          <span class="w-6 text-center text-[11px] tabular-nums text-gray-500 dark:text-dark-400">{{ brushSize }}</span>
        </div>
        <!-- 笔迹形状：圆头 / 方头 -->
        <button
          type="button"
          class="canvas-tool-btn"
          :class="brushShape === 'round' && 'canvas-tool-btn-active'"
          :title="t('creative.canvas.shapeRound')"
          @click="setBrushShape('round')"
        >
          <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="1.5">
            <circle cx="12" cy="12" r="5" />
          </svg>
        </button>
        <button
          type="button"
          class="canvas-tool-btn"
          :class="brushShape === 'square' && 'canvas-tool-btn-active'"
          :title="t('creative.canvas.shapeSquare')"
          @click="setBrushShape('square')"
        >
          <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="1.5">
            <rect x="7" y="7" width="10" height="10" />
          </svg>
        </button>
      </template>

      <span class="mx-0.5 h-5 w-px bg-primary-900/10 dark:bg-dark-600"></span>
      <!-- 删除选中图片 -->
      <button
        type="button"
        class="canvas-tool-btn"
        :disabled="!selectedImage"
        :title="t('creative.canvas.removeSelected')"
        @click="removeSelected"
      >
        <Icon name="x" size="sm" />
      </button>
    </div>

    <!-- 局部重绘未选中图片：引导点击选择目标图片（位于工具栏下方，避免重叠） -->
    <div
      v-if="isInpaint && !inpaintAnchor"
      class="pointer-events-none absolute bottom-16 left-1/2 z-10 -translate-x-1/2 whitespace-nowrap rounded-full bg-black/60 px-3 py-1 text-xs text-white dark:bg-white/15 lg:bottom-auto lg:top-16"
    >
      {{ t('creative.canvas.inpaintPickHint') }}
    </div>
    <!-- 图生图未选中源图：同款胶囊引导 -->
    <div
      v-else-if="isEdit && !selectedImage"
      class="pointer-events-none absolute bottom-16 left-1/2 z-10 -translate-x-1/2 whitespace-nowrap rounded-full bg-black/60 px-3 py-1 text-xs text-white dark:bg-white/15 lg:bottom-auto lg:top-16"
    >
      {{ t('creative.panel.selectImageHint') }}
    </div>
    <!-- 涂抹引导：首次落笔前提示紫色笔迹即重绘区域 -->
    <div
      v-else-if="painting && !hasMaskStrokes"
      class="pointer-events-none absolute bottom-16 left-1/2 z-10 -translate-x-1/2 whitespace-nowrap rounded-full bg-black/60 px-3 py-1 text-xs text-white dark:bg-white/15 lg:bottom-auto lg:top-16"
    >
      {{ t('creative.canvas.maskPaintHint') }}
    </div>

    <!-- 裁剪弹窗队列：每张图片依次进入，确认/跳过后直接放上画布 -->
    <CropperModal :show="cropQueue.length > 0" :blob="cropQueue[0] ?? null" @confirm="onCropConfirm" @skip="onCropConfirm" @cancel="onCropCancel" />
  </div>
</template>

<script setup lang="ts">
/**
 * 创作台无限画布（fabric 7）
 * - 逻辑尺寸跟随容器；空白处拖拽平移视角（改 viewportTransform），滚轮以光标为中心缩放（0.2–3）
 * - 图片对象可点选 / 拖动 / Delete 删除；生成输出自动按"上一个放置位置右侧 40px、约 2200px 换行"上板并平滑平移视角
 * - 局部重绘：选中图片自动进入涂抹模式（紫色笔迹 = 重绘区域，导出时自动转白底 mask）；
 *   涂抹中可用中键 / 右键拖拽平移，工具栏开关可暂停涂抹去移动 / 换选图片
 * - 工具栏：上传、下载选中、画笔组（仅局部重绘）、删除选中；清空画布收在左上角设置里
 * - 场景快照（含 data 自定义属性，图片 src 以 asset:// 占位）防抖存入 IndexedDB，刷新后恢复并重建输出注册表
 */
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import { Canvas, FabricImage, PencilBrush, Point, Rect, type FabricObject, type TMat2D } from 'fabric'
import { SquarePencilBrush } from './SquarePencilBrush'
import Icon from '@/components/icons/Icon.vue'
import CropperModal from './CropperModal.vue'
import type { CreativeOperation } from '@/api/creative'
import {
  LocalStoreQuotaError,
  loadAsset,
  loadSceneJson,
  localAssetKey,
  outputAssetKey,
  saveAsset,
  saveSceneJson,
} from '@/utils/creativeLocalStore'

interface Props {
  // 当前操作：局部重绘时启用画笔组并在选中图片后自动进入涂抹模式
  operation: CreativeOperation
}

interface Emits {
  (e: 'error', message: string): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { t } = useI18n()

// 场景快照在本地库中的 key（沿用既有约定）
const SCENE_KEY = 'creative:canvas'
// 场景自动保存防抖
const SCENE_SAVE_DEBOUNCE = 1000
// 缩放范围
const MIN_ZOOM = 0.2
const MAX_ZOOM = 3
// 输出自动上板的排布参数
const PLACE_GAP = 40
const PLACE_WRAP_X = 2200
// 图片上板缩放（上传与生成输出统一）：原始图过大，按 1/4 线性尺寸（1/16 面积）放置，blob 本身不变
const PLACE_SCALE = 0.25
// 图片 src 在场景快照中的占位协议，恢复时回 IndexedDB 取 blob
const ASSET_PROTOCOL = 'asset://'
// mask 导出色（导出为白底透明 PNG 的白色轨迹）；展示用紫色叠加层，二者分离避免白图上看不见笔迹
const MASK_COLOR = '#ffffff'
const MASK_TINT = 'rgba(168, 85, 247, 0.55)'
// 涂抹锚点描边（运行时辅助对象，不进快照、不进 mask）
const ANCHOR_OUTLINE_KIND = 'anchor-outline'
const ANCHOR_OUTLINE_STYLE = {
  fill: 'transparent',
  stroke: 'rgba(0, 210, 255, 0.7)',
  strokeWidth: 1.5,
  strokeDashArray: [6, 4],
}
// 圆点网格的场景间距（px，缩放 1 时）
const GRID_SPACING = 20

// 圆点网格锚定场景坐标：背景位置 = 视口平移分量，间距 = 场景间距 × 缩放系数；
// 因此平移时网格与图片同步滑动，缩小变密、放大变稀疏，与画布缩放观感一致。
function syncDotGrid(): void {
  const el = containerRef.value
  if (!el || !canvas) return
  const vpt = canvas.viewportTransform
  const spacing = GRID_SPACING * canvas.getZoom()
  el.style.backgroundPosition = `${vpt[4]}px ${vpt[5]}px`
  el.style.backgroundSize = `${spacing}px ${spacing}px`
}

// fabric 7 类型未声明自定义 data 属性，运行时允许挂任意键，这里做最小封装
type ObjectWithData = { data?: Record<string, unknown> }

function objectData(object: FabricObject): Record<string, unknown> {
  return (object as unknown as ObjectWithData).data ?? {}
}

function setObjectData(object: FabricObject, data: Record<string, unknown>): void {
  const target = object as unknown as ObjectWithData
  target.data = data
}

type BrushShape = 'round' | 'square'

const containerRef = ref<HTMLDivElement | null>(null)
const canvasElRef = ref<HTMLCanvasElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
// 待裁剪队列：确认/跳过一张后自动出队下一张
const cropQueue = ref<Blob[]>([])
// 涂抹模式开关：局部重绘 + 锚定图片时自动开启（工具栏可暂停）
const painting = ref(false)
// 是否已有 mask 笔迹（控制"清除涂抹"/橡皮按钮可用态）
const hasMaskStrokes = ref(false)
// 画笔粗细（8–96）/ 形状
const brushSize = ref(28)
const brushShape = ref<BrushShape>('round')
// 当前选中的图片对象（fabric 活动对象；画笔落笔时 fabric 会丢弃选中，不能作为 mask 锚点依据）
const selectedImage = shallowRef<FabricObject | null>(null)
// mask 锚定的图片对象：选中图片时设置，与 fabric 选中态解耦，避免画笔落笔自动丢弃选中后涂抹被中断
const inpaintAnchor = shallowRef<FabricObject | null>(null)
// 手动暂停涂抹（移动视角 / 换选图片后再恢复）
const paintSuspended = ref(false)
// 锚点描边对象（运行时辅助，涂抹期间标示目标图片）
let anchorOutline: Rect | null = null

let canvas: Canvas | null = null
let resizeObserver: ResizeObserver | null = null
let sceneSaveTimer: ReturnType<typeof setTimeout> | null = null
// 平移拖拽状态：pointerId 统一鼠标 / 触摸
let isPanning = false
let lastClientX = 0
let lastClientY = 0
// 平滑平移动画的 rAF 句柄
let panAnimFrame: number | null = null
// 图片原始 blob 的运行时缓存：assetKey → blob（生成时取源图，避免反复读 IndexedDB）
const runtimeBlobs = new Map<string, Blob>()
// 上一个放置位置（场景坐标，right/bottom 为右缘 / 下缘）
let lastPlaced: { right: number; top: number; bottom: number } | null = null
// mask 笔迹撤销栈（LIFO，上限见 MASK_UNDO_LIMIT）：记录笔迹新增与整体清除
type MaskUndoEntry =
  | { type: 'add'; path: FabricObject }
  | { type: 'clear'; paths: FabricObject[] }
const maskUndoStack: MaskUndoEntry[] = []
// mask 笔迹共享裁剪框（限制笔迹不画出目标图片；fabric 允许跨对象共享 clipPath 实例）
let maskClipRect: Rect | null = null
// 画笔模式下被锁定对象的原始交互状态
let lockedStates: Map<FabricObject, { selectable: boolean; evented: boolean }> | null = null

// ==================== 初始化 ====================

onMounted(() => {
  const element = canvasElRef.value
  const container = containerRef.value
  if (!element || !container) return
  canvas = new Canvas(element, {
    width: Math.max(container.clientWidth, 320),
    height: Math.max(container.clientHeight, 320),
    preserveObjectStacking: true,
    // 背景透明，圆点网格由容器 CSS 透出；导出的 mask 带 alpha
    backgroundColor: '',
    // 空白拖拽用于平移视角，不做框选
    selection: false,
    defaultCursor: 'grab',
  })
  bindCanvasEvents()
  // 涂抹模式下右键 / 中键拖拽平移，需屏蔽画布上的右键菜单
  container.addEventListener('contextmenu', suppressContextMenu)
  resizeObserver = new ResizeObserver(fitToContainer)
  resizeObserver.observe(container)
  window.addEventListener('keydown', onKeyDown)
  syncDotGrid()
  void restoreScene()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown)
  containerRef.value?.removeEventListener('contextmenu', suppressContextMenu)
  resizeObserver?.disconnect()
  resizeObserver = null
  stopPanAnim()
  if (sceneSaveTimer) clearTimeout(sceneSaveTimer)
  if (canvas) {
    persistSceneNow()
    void canvas.dispose()
    canvas = null
  }
})

// 画布逻辑尺寸跟随容器（无上界，配合平移形成无限画布）
function fitToContainer(): void {
  if (!canvas || !containerRef.value) return
  const { clientWidth, clientHeight } = containerRef.value
  if (clientWidth <= 0 || clientHeight <= 0) return
  canvas.setDimensions({ width: Math.floor(clientWidth), height: Math.floor(clientHeight) })
}

// ==================== 事件绑定 ====================

function suppressContextMenu(event: Event): void {
  event.preventDefault()
}

function bindCanvasEvents(): void {
  if (!canvas) return
  // 空白处按下左键 / 单指 = 平移视角；涂抹模式下左键落笔，橡皮模式下左键擦除，中键 / 右键拖拽平移
  canvas.on('mouse:down', (event) => {
    if (!canvas) return
    if (painting.value) {
      if (isPanButton(event.e)) startPan(event.e)
      return
    }
    if (event.target) return
    if (!isPrimaryPointer(event.e)) return
    startPan(event.e)
    canvas.discardActiveObject()
  })
  canvas.on('mouse:move', (event) => {
    if (!canvas || !isPanning) return
    const client = clientPoint(event.e)
    const dx = client.x - lastClientX
    const dy = client.y - lastClientY
    lastClientX = client.x
    lastClientY = client.y
    // 视口平移与缩放无关：直接改 vpt 平移分量
    const vpt = [...canvas.viewportTransform] as TMat2D
    vpt[4] += dx
    vpt[5] += dy
    canvas.setViewportTransform(vpt)
    syncDotGrid()
    scheduleSceneSave()
  })
  canvas.on('mouse:up', () => stopPanning())
  canvas.on('mouse:wheel', (event) => {
    if (!canvas) return
    event.e.preventDefault()
    event.e.stopPropagation()
    const wheel = event.e as WheelEvent
    // 以光标位置为缩放中心（zoomToPoint 接收画布元素坐标系内的点）
    const next = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, canvas.getZoom() * 0.999 ** wheel.deltaY))
    canvas.zoomToPoint(new Point(wheel.offsetX, wheel.offsetY), next)
    syncDotGrid()
    scheduleSceneSave()
  })
  // 画笔落笔完成：标记为 mask 轨迹，画在图片上层、不参与选中；入撤销栈并套用图片范围裁剪
  canvas.on('path:created', (event) => {
    const path = event.path as FabricObject | undefined
    if (path && painting.value) {
      setObjectData(path, { kind: 'mask' })
      path.set({ selectable: false, evented: false })
      if (maskClipRect) {
        path.clipPath = maskClipRect
      }
      hasMaskStrokes.value = true
      pushMaskUndo({ type: 'add', path })
    }
    scheduleSceneSave()
  })
  canvas.on('object:added', () => {
    scheduleSceneSave()
  })
  canvas.on('object:modified', () => {
    scheduleSceneSave()
  })
  canvas.on('object:removed', (event) => {
    // 锚点图片被删除时同步清空，涂抹模式随之退出
    if (event.target && inpaintAnchor.value === event.target) {
      inpaintAnchor.value = null
    }
    if (!canvas?.getObjects().some((object) => objectData(object).kind === 'mask')) {
      hasMaskStrokes.value = false
    }
    scheduleSceneSave()
  })
  canvas.on('selection:created', (event) => {
    onImageSelected(pickImage(event.selected?.[0]))
  })
  canvas.on('selection:updated', (event) => {
    onImageSelected(pickImage(event.selected?.[0]))
  })
  canvas.on('selection:cleared', () => {
    selectedImage.value = null
    // fabric 画笔落笔时会自动丢弃选中对象：涂抹期间保留锚点，仅手动取消选中才清空
    if (!painting.value) {
      inpaintAnchor.value = null
    }
  })
}

// 平移拖拽开始（坐标记录 + 抓手光标）
function startPan(event: Event): void {
  if (!canvas) return
  isPanning = true
  const client = clientPoint(event)
  lastClientX = client.x
  lastClientY = client.y
  canvas.defaultCursor = 'grabbing'
}

// 涂抹模式下用于平移的按键：鼠标中键 / 右键
function isPanButton(event: Event): boolean {
  const mouse = event as MouseEvent
  return typeof mouse.button === 'number' && (mouse.button === 1 || mouse.button === 2)
}

function stopPanning(): void {
  isPanning = false
  if (canvas) canvas.defaultCursor = painting.value ? 'crosshair' : 'grab'
}

// 鼠标左键或单指触摸才可平移
function isPrimaryPointer(event: Event): boolean {
  const mouse = event as MouseEvent
  if (typeof mouse.button === 'number' && mouse.button !== 0) return false
  const touch = event as TouchEvent
  if (touch.touches && touch.touches.length > 1) return false
  return true
}

// 统一鼠标 / 触摸的屏幕坐标
function clientPoint(event: Event): { x: number; y: number } {
  const touch = event as TouchEvent
  const point = touch.touches?.[0] ?? touch.changedTouches?.[0]
  if (point) return { x: point.clientX, y: point.clientY }
  const mouse = event as MouseEvent
  return { x: mouse.clientX, y: mouse.clientY }
}

function pickImage(target: FabricObject | undefined): FabricObject | null {
  return target instanceof FabricImage ? target : null
}

// Delete / Backspace 删除选中对象；Ctrl / Cmd + Z 撤销上一笔涂抹（输入控件聚焦时不拦截）
function onKeyDown(event: KeyboardEvent): void {
  if (!canvas) return
  const target = event.target as HTMLElement | null
  if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) return
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'z' && !event.shiftKey) {
    event.preventDefault()
    undoMask()
    return
  }
  if (painting.value || event.key !== 'Delete' && event.key !== 'Backspace') return
  const active = canvas.getActiveObject()
  if (!active) return
  event.preventDefault()
  canvas.remove(active)
  canvas.discardActiveObject()
  canvas.requestRenderAll()
}

// ==================== 涂抹（mask）模式 ====================

// 仅局部重绘开放画笔组与涂抹模式
const isInpaint = computed(() => props.operation === 'inpaint')
// 图生图模式（未选中源图时展示引导胶囊）
const isEdit = computed(() => props.operation === 'edit')

// 选中图片时登记为涂抹锚点（换选自动切换）；仅局部重绘模式下登记
function onImageSelected(image: FabricObject | null): void {
  selectedImage.value = image
  if (image && isInpaint.value) {
    inpaintAnchor.value = image
    paintSuspended.value = false
  }
}

// 涂抹条件 = 局部重绘 + 有锚点图片 + 未手动暂停；任一条件变化统一经 syncPainting 进出
watch([isInpaint, inpaintAnchor, paintSuspended], syncPainting)

function syncPainting(): void {
  const shouldPaint = isInpaint.value && inpaintAnchor.value !== null && !paintSuspended.value
  if (shouldPaint) enterPainting()
  else exitPainting()
  updateAnchorOutline()
}

function makeBrush(): PencilBrush {
  const brush =
    brushShape.value === 'square' ? new SquarePencilBrush(canvas!) : new PencilBrush(canvas!)
  // 展示用紫色叠加层；导出 mask 时才统一改回白色
  brush.color = MASK_TINT
  brush.width = brushSize.value
  return brush
}

// 进入涂抹：锁定全部对象，仅落笔作画（锚点用独立描边标示，不依赖 fabric 选中态）
function enterPainting(): void {
  if (!canvas || painting.value || !isInpaint.value || !inpaintAnchor.value) return
  painting.value = true
  lockAllObjects()
  refreshMaskClip()
  canvas.freeDrawingBrush = makeBrush()
  canvas.isDrawingMode = true
  canvas.defaultCursor = 'crosshair'
  canvas.requestRenderAll()
}

function exitPainting(): void {
  if (!canvas || !painting.value) return
  painting.value = false
  canvas.isDrawingMode = false
  canvas.freeDrawingBrush = undefined
  unlockAllObjects()
  canvas.defaultCursor = 'grab'
  canvas.requestRenderAll()
}

// 锚点描边：涂抹期间在目标图片外围画一圈青色虚线，避免"在涂哪张图"失焦
function updateAnchorOutline(): void {
  const anchor = inpaintAnchor.value
  if (!canvas || !anchor || !painting.value) {
    removeAnchorOutline()
    return
  }
  if (!anchorOutline) {
    anchorOutline = new Rect({
      ...ANCHOR_OUTLINE_STYLE,
      selectable: false,
      evented: false,
    })
    setObjectData(anchorOutline, { kind: ANCHOR_OUTLINE_KIND })
    canvas.add(anchorOutline)
  }
  const width = ((anchor as unknown as { width?: number }).width ?? 0) * (anchor.scaleX ?? 1)
  const height = ((anchor as unknown as { height?: number }).height ?? 0) * (anchor.scaleY ?? 1)
  anchorOutline.set({ left: anchor.left, top: anchor.top, width, height, angle: anchor.angle ?? 0 })
  anchorOutline.setCoords()
  canvas.requestRenderAll()
}

function removeAnchorOutline(): void {
  if (anchorOutline && canvas) {
    canvas.remove(anchorOutline)
  }
  anchorOutline = null
}

// 刷新 mask 笔迹的共享裁剪框：限制笔迹只能落在锚点图片的轴对齐包围盒内（场景坐标）。
// 进入涂抹 / 换锚点时调用；锚点涂抹期间被锁定不会移动，无需持续刷新。
function refreshMaskClip(): void {
  maskClipRect = null
  if (!canvas || !inpaintAnchor.value) return
  const vpt = canvas.viewportTransform
  const zoom = canvas.getZoom() || 1
  // getBoundingRect 返回视口坐标，换算回场景坐标作为 absolutePositioned 裁剪框
  const vp = inpaintAnchor.value.getBoundingRect()
  maskClipRect = new Rect({
    left: (vp.left - vpt[4]) / zoom,
    top: (vp.top - vpt[5]) / zoom,
    width: vp.width / zoom,
    height: vp.height / zoom,
  })
  maskClipRect.absolutePositioned = true
  // 局部变量收窄类型（forEach 闭包内无法收窄模块级 let）
  const clip = maskClipRect
  canvas.getObjects().forEach((object) => {
    if (objectData(object).kind === 'mask') {
      object.clipPath = clip
    }
  })
}

// 工具栏涂抹开关：暂停涂抹去移动视角 / 换选图片，再次点击恢复
function togglePainting(): void {
  paintSuspended.value = painting.value
}

// mask 撤销栈上限，防止长会话无限增长
const MASK_UNDO_LIMIT = 50

// 撤销是否可用（工具栏按钮置灰依据）
const canUndoMask = ref(false)

function pushMaskUndo(entry: MaskUndoEntry): void {
  maskUndoStack.push(entry)
  if (maskUndoStack.length > MASK_UNDO_LIMIT) {
    maskUndoStack.shift()
  }
  canUndoMask.value = true
}

// 撤销上一笔：新增 → 移除该笔迹；清除 → 整体恢复被清除的笔迹
function undoMask(): void {
  if (!canvas) return
  const entry = maskUndoStack.pop()
  if (!entry) {
    canUndoMask.value = false
    return
  }
  if (entry.type === 'add') {
    canvas.remove(entry.path)
  } else {
    entry.paths.forEach((path) => canvas!.add(path))
    hasMaskStrokes.value = true
  }
  canUndoMask.value = maskUndoStack.length > 0
  canvas.requestRenderAll()
  scheduleSceneSave()
}

function setBrushShape(shape: BrushShape): void {
  brushShape.value = shape
  // 涂抹开启时立即换笔，保证下一次落笔即新形状
  if (painting.value && canvas) canvas.freeDrawingBrush = makeBrush()
}

watch(brushSize, (size) => {
  if (canvas?.freeDrawingBrush) canvas.freeDrawingBrush.width = size
})

// 清除全部 mask 笔迹（整体入撤销栈，可一步恢复）
function clearMask(): void {
  if (!canvas) return
  const paths = canvas.getObjects().filter((object) => objectData(object).kind === 'mask')
  if (!paths.length) return
  paths.forEach((object) => canvas!.remove(object))
  hasMaskStrokes.value = false
  pushMaskUndo({ type: 'clear', paths })
}

function lockAllObjects(): void {
  if (!canvas) return
  lockedStates = new Map()
  canvas.getObjects().forEach((object) => {
    lockedStates?.set(object, { selectable: object.selectable, evented: object.evented })
    object.set({ selectable: false, evented: false })
  })
}

function unlockAllObjects(): void {
  lockedStates?.forEach((state, object) => {
    object.set({ selectable: state.selectable, evented: state.evented })
  })
  lockedStates = null
}

// ==================== 视角与平滑平移 ====================

// 当前视口中心的场景坐标
function viewCenterScene(): { x: number; y: number } {
  const vpt = canvas!.viewportTransform
  const zoom = canvas!.getZoom()
  return {
    x: (canvas!.getWidth() / 2 - vpt[4]) / zoom,
    y: (canvas!.getHeight() / 2 - vpt[5]) / zoom,
  }
}

function stopPanAnim(): void {
  if (panAnimFrame !== null) cancelAnimationFrame(panAnimFrame)
  panAnimFrame = null
}

// 视角平滑移动到指定场景点（保持当前缩放）
function panToScenePoint(point: { x: number; y: number }): void {
  if (!canvas) return
  stopPanAnim()
  const zoom = canvas.getZoom()
  const targetX = canvas.getWidth() / 2 - zoom * point.x
  const targetY = canvas.getHeight() / 2 - zoom * point.y
  const startX = canvas.viewportTransform[4]
  const startY = canvas.viewportTransform[5]
  const duration = 240
  const startTime = performance.now()
  const step = (now: number) => {
    if (!canvas) return
    const progress = Math.min(1, (now - startTime) / duration)
    const eased = 1 - (1 - progress) ** 3
    const vpt = [...canvas.viewportTransform] as TMat2D
    vpt[4] = startX + (targetX - startX) * eased
    vpt[5] = startY + (targetY - startY) * eased
    canvas.setViewportTransform(vpt)
    syncDotGrid()
    if (progress < 1) {
      panAnimFrame = requestAnimationFrame(step)
    } else {
      panAnimFrame = null
      scheduleSceneSave()
    }
  }
  panAnimFrame = requestAnimationFrame(step)
}

// ==================== 图片上板 ====================

// 读取 blob 的像素尺寸（优先 createImageBitmap，失败回退 Image 元素）
async function probeImageSize(blob: Blob): Promise<{ width: number; height: number }> {
  try {
    const bitmap = await createImageBitmap(blob)
    const size = { width: bitmap.width, height: bitmap.height }
    bitmap.close()
    return size
  } catch {
    return await new Promise((resolve, reject) => {
      const url = URL.createObjectURL(blob)
      const image = new Image()
      image.onload = () => {
        URL.revokeObjectURL(url)
        resolve({ width: image.naturalWidth, height: image.naturalHeight })
      }
      image.onerror = () => {
        URL.revokeObjectURL(url)
        reject(new Error('image load failed'))
      }
      image.src = url
    })
  }
}

// 下一个放置位置（场景坐标，返回左上角）：上一个右侧 40px，超出约 2200px 换行
function nextPlacementPoint(width: number, height: number): { x: number; y: number } {
  if (!lastPlaced) {
    const center = viewCenterScene()
    return { x: center.x - width / 2, y: center.y - height / 2 }
  }
  let x = lastPlaced.right + PLACE_GAP
  let y = lastPlaced.top
  if (x + width > PLACE_WRAP_X) {
    x = 0
    y = lastPlaced.bottom + PLACE_GAP
  }
  return { x, y }
}

// 把图片 blob 放上画布：左上角定位于 position，按 scale 缩放显示（原始 blob 不受影响），登记 data 与运行时 blob
async function addImageToScene(
  blob: Blob,
  meta: { assetKey: string; runId?: string; outputIndex?: number },
  position: { x: number; y: number },
  scale = 1,
): Promise<FabricImage | null> {
  if (!canvas) return null
  const url = URL.createObjectURL(blob)
  try {
    const image = await FabricImage.fromURL(url)
    image.set({
      left: position.x,
      top: position.y,
      scaleX: scale,
      scaleY: scale,
    })
    setObjectData(image, {
      kind: 'image',
      assetKey: meta.assetKey,
      ...(meta.runId !== undefined ? { runId: meta.runId, outputIndex: meta.outputIndex } : {}),
    })
    canvas.add(image)
    canvas.setActiveObject(image)
    selectedImage.value = image
    canvas.requestRenderAll()
    return image
  } catch (error) {
    console.error('Failed to load image into canvas:', error)
    emit('error', t('creative.error.loadImageFailed'))
    return null
  } finally {
    URL.revokeObjectURL(url)
  }
}

// 收割成功的输出自动上板：记录 blob → 放置 → 视角移动到新图中心
async function placeOutput(asset: { blob: Blob; runId: string; outputIndex: number }): Promise<void> {
  if (!canvas) return
  const assetKey = outputAssetKey(asset.runId, asset.outputIndex)
  runtimeBlobs.set(assetKey, asset.blob)
  try {
    // 生成输出与上传图统一按 1/4 尺寸上板；排布、换行与视角定位均按缩放后尺寸计算
    const size = await probeImageSize(asset.blob)
    const placedWidth = size.width * PLACE_SCALE
    const placedHeight = size.height * PLACE_SCALE
    const position = nextPlacementPoint(placedWidth, placedHeight)
    const image = await addImageToScene(
      asset.blob,
      { assetKey, runId: asset.runId, outputIndex: asset.outputIndex },
      position,
      PLACE_SCALE,
    )
    if (!image) return
    lastPlaced = {
      right: position.x + placedWidth,
      top: position.y,
      bottom: position.y + placedHeight,
    }
    panToScenePoint({ x: position.x + placedWidth / 2, y: position.y + placedHeight / 2 })
    scheduleSceneSave()
  } catch (error) {
    console.error('Failed to place creative output:', error)
    emit('error', t('creative.error.loadImageFailed'))
  }
}

// 上传（裁剪确认后）放上画布：先存本地库保证刷新后可恢复，再放到当前视角中心
async function addUploadedImage(blob: Blob): Promise<void> {
  if (!canvas) return
  const assetKey = localAssetKey('source', `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`)
  try {
    await saveAsset({ key: assetKey, kind: 'source', blob, createdAt: Date.now() })
  } catch (error) {
    // 本地保存失败（多为配额不足）：不上板，避免画出无法恢复的场景
    if (error instanceof LocalStoreQuotaError) {
      emit('error', t('creative.error.quotaExceeded'))
    } else {
      emit('error', t('creative.error.loadImageFailed'))
    }
    return
  }
  runtimeBlobs.set(assetKey, blob)
  try {
    // 上传图按 1/4 尺寸上板（与生成输出统一；原始 blob 不变，源图仍发全分辨率给模型）
    const size = await probeImageSize(blob)
    const center = viewCenterScene()
    const scale = PLACE_SCALE
    await addImageToScene(
      blob,
      { assetKey },
      { x: center.x - (size.width * scale) / 2, y: center.y - (size.height * scale) / 2 },
      scale,
    )
    scheduleSceneSave()
  } catch (error) {
    console.error('Failed to place uploaded image:', error)
    emit('error', t('creative.error.loadImageFailed'))
  }
}

// ==================== 生成输入采集 ====================

// 当前关联图片对象：优先 fabric 活动对象，缺失时回退到涂抹锚点
// （画笔落笔时 fabric 会自动丢弃选中，涂抹期间锚点才是可靠的图片引用）
function selectedImageObject(): FabricImage | null {
  if (!canvas) return null
  const active = canvas.getActiveObject()
  if (active instanceof FabricImage) return active
  return inpaintAnchor.value instanceof FabricImage ? inpaintAnchor.value : null
}

// 选中图片的原始 blob：运行时缓存优先，缺失时回 IndexedDB 取
async function getSelectedImageBlob(): Promise<Blob | null> {
  const image = selectedImageObject()
  if (!image) return null
  const assetKey = objectData(image).assetKey
  if (typeof assetKey !== 'string') return null
  const cached = runtimeBlobs.get(assetKey)
  if (cached) return cached
  const asset = await loadAsset(assetKey).catch(() => null)
  if (asset) {
    runtimeBlobs.set(assetKey, asset.blob)
    return asset.blob
  }
  return null
}

// 导出 mask：选中图片 → 取其场景包围盒 → 临时单位阵视口 + 隐藏非 mask 对象
// → 展示色笔迹临时改回纯白 → toCanvasElement 裁剪 → 离屏拉伸到原图自然尺寸 → 透明底 PNG
async function getMaskBlob(): Promise<Blob | null> {
  if (!canvas) return null
  const image = selectedImageObject()
  if (!image) return null
  const maskObjects = canvas.getObjects().filter((object) => objectData(object).kind === 'mask')
  if (!maskObjects.length) return null

  const rect = image.getBoundingRect()
  const hidden: FabricObject[] = []
  canvas.getObjects().forEach((object) => {
    if (objectData(object).kind !== 'mask' && object.visible) {
      object.visible = false
      hidden.push(object)
    }
  })
  // 展示用紫色叠加层，导出必须是不透明的纯白轨迹：渲染前统一改色，结束后恢复
  const strokeBackup = maskObjects.map((object) => ({ object, stroke: object.stroke }))
  strokeBackup.forEach(({ object }) => {
    object.set('stroke', MASK_COLOR)
  })
  const viewport = canvas.viewportTransform
  // 单位阵视口：裁剪框即场景坐标系下的包围盒
  canvas.viewportTransform = [1, 0, 0, 1, 0, 0]
  let element: HTMLCanvasElement
  try {
    element = canvas.toCanvasElement(1, {
      left: rect.left,
      top: rect.top,
      width: rect.width,
      height: rect.height,
    })
  } finally {
    canvas.viewportTransform = viewport
    strokeBackup.forEach(({ object, stroke }) => {
      object.set('stroke', stroke ?? null)
    })
    hidden.forEach((object) => {
      object.visible = true
    })
    canvas.requestRenderAll()
    syncDotGrid()
  }

  // 拉伸回原图自然尺寸，与服务端"mask 尺寸必须与源图一致"的校验对齐
  const source = image.getElement() as HTMLImageElement
  const naturalWidth = source.naturalWidth || Math.max(1, Math.round(rect.width))
  const naturalHeight = source.naturalHeight || Math.max(1, Math.round(rect.height))
  const offscreen = document.createElement('canvas')
  offscreen.width = naturalWidth
  offscreen.height = naturalHeight
  const context = offscreen.getContext('2d')
  if (!context) return null
  context.drawImage(element, 0, 0, naturalWidth, naturalHeight)
  return await new Promise<Blob | null>((resolve) => offscreen.toBlob(resolve, 'image/png'))
}

// ==================== 移除与重置 ====================

function removeSelected(): void {
  if (!canvas) return
  const active = canvas.getActiveObject()
  if (!active) return
  canvas.remove(active)
  canvas.discardActiveObject()
  canvas.requestRenderAll()
}

// 下载选中图片的原始 blob 到本地（与历史下载一致走 file-saver）
async function downloadSelected(): Promise<void> {
  const blob = await getSelectedImageBlob()
  if (!blob) return
  const extension = (blob.type || 'image/png').split('/')[1] || 'png'
  saveAs(blob, `creative-image-${Date.now()}.${extension}`)
}

function resetCanvas(): void {
  if (!canvas) return
  stopPanAnim()
  inpaintAnchor.value = null
  paintSuspended.value = false
  maskUndoStack.length = 0
  canUndoMask.value = false
  maskClipRect = null
  exitPainting()
  removeAnchorOutline()
  canvas.discardActiveObject()
  canvas.clear()
  canvas.backgroundColor = ''
  canvas.setViewportTransform([1, 0, 0, 1, 0, 0])
  syncDotGrid()
  hasMaskStrokes.value = false
  lastPlaced = null
  runtimeBlobs.clear()
  selectedImage.value = null
  canvas.requestRenderAll()
  scheduleSceneSave()
}

// ==================== 场景持久化 ====================

function scheduleSceneSave(): void {
  if (sceneSaveTimer) clearTimeout(sceneSaveTimer)
  sceneSaveTimer = setTimeout(() => persistSceneNow(), SCENE_SAVE_DEBOUNCE)
}

function persistSceneNow(): void {
  if (!canvas) return
  const json = snapshotScene()
  void saveSceneJson(SCENE_KEY, json).catch((error) => {
    console.error('Failed to persist creative scene:', error)
  })
}

// 序列化（含自定义 data）：图片像素不进快照，src 用 asset://<assetKey> 占位；
// 锚点描边是运行时辅助对象，不持久化
function snapshotScene(): string {
  if (!canvas) return '{}'
  const json = canvas.toObject(['data']) as { objects?: Array<Record<string, unknown> & ObjectWithData> }
  for (const object of json.objects ?? []) {
    const assetKey = object.data?.assetKey
    if (object.type === 'image' && typeof assetKey === 'string') {
      object.src = `${ASSET_PROTOCOL}${assetKey}`
    }
  }
  json.objects = (json.objects ?? []).filter((object) => object.data?.kind !== ANCHOR_OUTLINE_KIND)
  return JSON.stringify(json)
}

// 挂载时恢复：asset:// 图片回 IndexedDB 取 blob，缺失的图跳过不阻塞
async function restoreScene(): Promise<void> {
  if (!canvas) return
  try {
    const stored = await loadSceneJson(SCENE_KEY)
    if (!stored) return
    const parsed = JSON.parse(stored) as { objects?: Array<Record<string, unknown> & ObjectWithData> }
    const objects = Array.isArray(parsed.objects) ? parsed.objects : []
    const pendingUrls: string[] = []
    const restored: typeof objects = []
    for (const object of objects) {
      // 旧快照里可能残留锚点描边等运行时辅助对象，直接跳过
      if (object.data?.kind === ANCHOR_OUTLINE_KIND) {
        continue
      }
      const assetKey = object.data?.assetKey
      if (object.type === 'image' && typeof assetKey === 'string') {
        const blob = await loadAsset(assetKey)
          .then((asset) => asset?.blob ?? null)
          .catch(() => null)
        if (!blob) {
          // 本地素材缺失：跳过该图，继续恢复其它对象
          continue
        }
        runtimeBlobs.set(assetKey, blob)
        const objectUrl = URL.createObjectURL(blob)
        object.src = objectUrl
        pendingUrls.push(objectUrl)
      } else if (
        object.type === 'image' &&
        typeof object.src === 'string' &&
        // 旧版本快照里的 blob: 对象 URL 已失效，无法恢复，跳过
        object.src.startsWith('blob:')
      ) {
        continue
      }
      restored.push(object)
    }
    parsed.objects = restored
    await canvas.loadFromJSON(parsed as never)
    pendingUrls.forEach((url) => URL.revokeObjectURL(url))
    canvas.requestRenderAll()
    // 恢复的视口/缩放同步到圆点网格
    syncDotGrid()
    hasMaskStrokes.value = canvas.getObjects().some((object) => objectData(object).kind === 'mask')
  } catch (error) {
    console.error('Failed to restore creative scene:', error)
  }
}

// ==================== 上传（裁剪流程） ====================

// 选择文件后全部进入裁剪队列
function onFilesPicked(event: Event): void {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files ?? []).filter((file) => file.type.startsWith('image/'))
  input.value = ''
  if (!files.length) return
  cropQueue.value = [...cropQueue.value, ...files]
}

// 裁剪确认（跳过也走这里，直接保留原图）：放上画布当前视角中心
function onCropConfirm(blob: Blob): void {
  cropQueue.value = cropQueue.value.slice(1)
  void addUploadedImage(blob)
}

// 取消裁剪：丢弃剩余队列
function onCropCancel(): void {
  cropQueue.value = []
}

defineExpose({
  placeOutput,
  addUploadedImage,
  getSelectedImageBlob,
  getMaskBlob,
  resetCanvas,
  clearMask,
  // 是否已有 mask 笔迹
  hasMaskStrokes,
})
</script>

<style scoped>
/* 深色圆点网格：浅色主题用暗点，dark 类下用亮点，画布背景透明透出 */
.dot-grid {
  background-image: radial-gradient(circle, rgb(15 23 42 / 0.12) 1px, transparent 1px);
  background-size: 20px 20px;
}

.dark .dot-grid {
  background-image: radial-gradient(circle, rgb(255 255 255 / 0.14) 1px, transparent 1px);
}

.canvas-tool-btn {
  @apply inline-flex h-8 w-8 items-center justify-center rounded-md text-gray-600 transition-colors;
  @apply hover:bg-gray-100 hover:text-gray-900;
  @apply disabled:cursor-not-allowed disabled:opacity-40;
  @apply dark:text-gray-300 dark:hover:bg-dark-700 dark:hover:text-gray-100;
}

.canvas-tool-btn-active {
  @apply bg-primary-600/10 text-primary-700 dark:text-primary-300;
}

/* 画笔粗细滑块自定义配色：accent-color 对未填充轨道的着色在浅色模式下过深；
   浅色模式用浅灰轨道 + 品牌青滑块，深色模式用暗色轨道（配色对齐项目 dark-700） */
.brush-size {
  -webkit-appearance: none;
  appearance: none;
  background: transparent;
}

.brush-size::-webkit-slider-runnable-track {
  height: 6px;
  border-radius: 9999px;
  background: rgb(209 213 219);
}

.dark .brush-size::-webkit-slider-runnable-track {
  background: rgb(41 41 46);
}

.brush-size::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  margin-top: -5px;
  height: 16px;
  width: 16px;
  border: none;
  border-radius: 9999px;
  background: rgb(0 210 255);
}

.brush-size::-moz-range-track {
  height: 6px;
  border-radius: 9999px;
  background: rgb(209 213 219);
}

.dark .brush-size::-moz-range-track {
  background: rgb(41 41 46);
}

.brush-size::-moz-range-thumb {
  height: 16px;
  width: 16px;
  border: none;
  border-radius: 9999px;
  background: rgb(0 210 255);
}
</style>
