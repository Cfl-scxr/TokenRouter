<template>
  <!-- 画布即整个背景：透明无边框，圆点网格铺满 -->
  <div ref="containerRef" class="dot-grid relative h-full w-full overflow-hidden">
    <canvas ref="canvasElRef"></canvas>

    <!-- 浮动工具栏：画笔 / 粗细 / 形状 / 移除选中 / 清空画布（移动端底部，桌面端顶部） -->
    <div
      class="absolute bottom-3 left-1/2 z-10 flex -translate-x-1/2 items-center gap-1.5 rounded-xl border border-primary-900/10 bg-white/90 px-2 py-1.5 shadow-md backdrop-blur dark:border-dark-600 dark:bg-dark-900/90 lg:bottom-auto lg:top-3"
    >
      <button
        type="button"
        class="canvas-tool-btn"
        :class="brushOn && 'canvas-tool-btn-active'"
        :title="t('creative.canvas.brush')"
        @click="setBrushOn(!brushOn)"
      >
        <Icon name="edit" size="sm" />
      </button>
      <span class="mx-0.5 h-5 w-px bg-primary-900/10 dark:bg-dark-600"></span>
      <!-- 画笔粗细滑块：8–96 -->
      <div class="flex items-center gap-1.5 px-1">
        <input
          v-model.number="brushSize"
          type="range"
          min="8"
          max="96"
          step="1"
          class="w-24 cursor-pointer accent-primary-600"
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
      <span class="mx-0.5 h-5 w-px bg-primary-900/10 dark:bg-dark-600"></span>
      <button
        type="button"
        class="canvas-tool-btn"
        :disabled="!selectedImage"
        :title="t('creative.canvas.removeSelected')"
        @click="removeSelected"
      >
        <Icon name="x" size="sm" />
      </button>
      <button type="button" class="canvas-tool-btn" :title="t('creative.canvas.reset')" @click="resetCanvas">
        <Icon name="trash" size="sm" />
      </button>
    </div>

    <!-- 画笔模式提示 -->
    <div
      v-if="brushOn"
      class="pointer-events-none absolute bottom-16 left-1/2 z-10 -translate-x-1/2 whitespace-nowrap rounded-full bg-black/60 px-3 py-1 text-xs text-white dark:bg-white/15 lg:bottom-auto lg:top-14"
    >
      {{ t('creative.canvas.maskBrushHint') }}
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * 创作台无限画布（fabric 7）
 * - 逻辑尺寸跟随容器；空白处拖拽平移视角（改 viewportTransform），滚轮以光标为中心缩放（0.2–3）
 * - 图片对象可点选 / 拖动 / Delete 删除；生成输出自动按"上一个放置位置右侧 40px、约 2200px 换行"上板并平滑平移视角
 * - 画笔即 mask 工具：白色轨迹作为 path 对象画在图片上层，data.kind = 'mask'，可导出透明底 PNG
 * - 场景快照（含 data 自定义属性，图片 src 以 asset:// 占位）防抖存入 IndexedDB，刷新后恢复并重建输出注册表
 */
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Canvas, FabricImage, PencilBrush, Point, type FabricObject, type TMat2D } from 'fabric'
import { SquarePencilBrush } from './SquarePencilBrush'
import Icon from '@/components/icons/Icon.vue'
import {
  LocalStoreQuotaError,
  loadAsset,
  loadSceneJson,
  localAssetKey,
  outputAssetKey,
  saveAsset,
  saveSceneJson,
} from '@/utils/creativeLocalStore'

interface Emits {
  (e: 'error', message: string): void
}

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
// 图片 src 在场景快照中的占位协议，恢复时回 IndexedDB 取 blob
const ASSET_PROTOCOL = 'asset://'
// mask 画笔颜色（导出为白底透明 PNG 的白色轨迹）
const MASK_COLOR = '#ffffff'
// 圆点网格基准间距（px，缩放 1 时）
const DOT_GRID_SIZE = 20

// 圆点网格跟随视角移动：背景位置 = 视口平移分量，间距 = 基准 × 当前缩放，
// 让网格像画在世界坐标系里一样，拖动/缩放时与图片同步。
function syncDotGrid(): void {
  const el = containerRef.value
  if (!el || !canvas) return
  const vpt = canvas.viewportTransform
  el.style.backgroundPosition = `${vpt[4]}px ${vpt[5]}px`
  const size = DOT_GRID_SIZE * canvas.getZoom()
  el.style.backgroundSize = `${size}px ${size}px`
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
// 画笔开关 / 粗细（8–96）/ 形状
const brushOn = ref(false)
const brushSize = ref(28)
const brushShape = ref<BrushShape>('round')
// 当前选中的图片对象（普通对象不展示移除入口）
const selectedImage = shallowRef<FabricObject | null>(null)

let canvas: Canvas | null = null
let resizeObserver: ResizeObserver | null = null
let sceneSaveTimer: ReturnType<typeof setTimeout> | null = null
// 平移拖拽状态：pointerId 统一鼠标 / 触摸
let isPanning = false
let lastClientX = 0
let lastClientY = 0
// 平滑平移动画的 rAF 句柄
let panAnimFrame: number | null = null
// 输出注册表：runId:outputIndex → 画布对象（历史点击平移用）
const outputRegistry = new Map<string, FabricObject>()
// 图片原始 blob 的运行时缓存：assetKey → blob（生成时取源图，避免反复读 IndexedDB）
const runtimeBlobs = new Map<string, Blob>()
// 上一个放置位置（场景坐标，right/bottom 为右缘 / 下缘）
let lastPlaced: { right: number; top: number; bottom: number } | null = null
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
  resizeObserver = new ResizeObserver(fitToContainer)
  resizeObserver.observe(container)
  window.addEventListener('keydown', onKeyDown)
  syncDotGrid()
  void restoreScene()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown)
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

function bindCanvasEvents(): void {
  if (!canvas) return
  // 空白处按下左键 / 单指 = 平移视角
  canvas.on('mouse:down', (event) => {
    if (!canvas || brushOn.value) return
    if (event.target) return
    if (!isPrimaryPointer(event.e)) return
    isPanning = true
    const client = clientPoint(event.e)
    lastClientX = client.x
    lastClientY = client.y
    canvas.discardActiveObject()
    canvas.defaultCursor = 'grabbing'
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
  // 画笔落笔完成：标记为 mask 轨迹，画在图片上层、不参与选中
  canvas.on('path:created', (event) => {
    const path = event.path as FabricObject | undefined
    if (path && brushOn.value) {
      setObjectData(path, { kind: 'mask' })
      path.set({ selectable: false, evented: false })
    }
    scheduleSceneSave()
  })
  canvas.on('object:added', (event) => {
    registerOutputObject(event.target as FabricObject)
    scheduleSceneSave()
  })
  canvas.on('object:modified', () => scheduleSceneSave())
  canvas.on('object:removed', (event) => {
    unregisterOutputObject(event.target as FabricObject)
    scheduleSceneSave()
  })
  canvas.on('selection:created', (event) => {
    selectedImage.value = pickImage(event.selected?.[0])
  })
  canvas.on('selection:updated', (event) => {
    selectedImage.value = pickImage(event.selected?.[0])
  })
  canvas.on('selection:cleared', () => {
    selectedImage.value = null
  })
}

function stopPanning(): void {
  isPanning = false
  if (canvas) canvas.defaultCursor = brushOn.value ? 'crosshair' : 'grab'
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

// Delete / Backspace 删除选中对象（输入控件聚焦时不拦截）
function onKeyDown(event: KeyboardEvent): void {
  if (!canvas || event.key !== 'Delete' && event.key !== 'Backspace') return
  const target = event.target as HTMLElement | null
  if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) return
  const active = canvas.getActiveObject()
  if (!active) return
  event.preventDefault()
  canvas.remove(active)
  canvas.discardActiveObject()
  canvas.requestRenderAll()
}

// ==================== 画笔（mask）模式 ====================

function makeBrush(): PencilBrush {
  const brush =
    brushShape.value === 'square' ? new SquarePencilBrush(canvas!) : new PencilBrush(canvas!)
  brush.color = MASK_COLOR
  brush.width = brushSize.value
  return brush
}

function setBrushOn(on: boolean): void {
  if (!canvas) return
  brushOn.value = on
  if (on) {
    canvas.discardActiveObject()
    selectedImage.value = null
    lockAllObjects()
    canvas.freeDrawingBrush = makeBrush()
    canvas.isDrawingMode = true
    canvas.defaultCursor = 'crosshair'
  } else {
    canvas.isDrawingMode = false
    canvas.freeDrawingBrush = undefined
    unlockAllObjects()
    canvas.defaultCursor = 'grab'
  }
  canvas.requestRenderAll()
}

function setBrushShape(shape: BrushShape): void {
  brushShape.value = shape
  // 画笔开启时立即换笔，保证下一次落笔即新形状
  if (brushOn.value && canvas) canvas.freeDrawingBrush = makeBrush()
}

watch(brushSize, (size) => {
  if (canvas?.freeDrawingBrush) canvas.freeDrawingBrush.width = size
})

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

// 把图片 blob 放上画布：左上角定位于 position，登记 data 与运行时 blob
async function addImageToScene(
  blob: Blob,
  meta: { assetKey: string; runId?: string; outputIndex?: number },
  position: { x: number; y: number },
): Promise<FabricImage | null> {
  if (!canvas) return null
  const url = URL.createObjectURL(blob)
  try {
    const image = await FabricImage.fromURL(url)
    image.set({
      left: position.x,
      top: position.y,
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
    const size = await probeImageSize(asset.blob)
    const position = nextPlacementPoint(size.width, size.height)
    const image = await addImageToScene(asset.blob, { assetKey, runId: asset.runId, outputIndex: asset.outputIndex }, position)
    if (!image) return
    lastPlaced = {
      right: position.x + size.width,
      top: position.y,
      bottom: position.y + size.height,
    }
    panToScenePoint({ x: position.x + size.width / 2, y: position.y + size.height / 2 })
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
    const size = await probeImageSize(blob)
    const center = viewCenterScene()
    await addImageToScene(blob, { assetKey }, { x: center.x - size.width / 2, y: center.y - size.height / 2 })
    scheduleSceneSave()
  } catch (error) {
    console.error('Failed to place uploaded image:', error)
    emit('error', t('creative.error.loadImageFailed'))
  }
}

// ==================== 生成输入采集 ====================

// 当前选中的图片对象
function selectedImageObject(): FabricImage | null {
  if (!canvas) return null
  const active = canvas.getActiveObject()
  return active instanceof FabricImage ? active : null
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
// → toCanvasElement 裁剪 → 离屏拉伸到原图自然尺寸 → 透明底 PNG（白色轨迹）
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

function resetCanvas(): void {
  if (!canvas) return
  stopPanAnim()
  if (brushOn.value) setBrushOn(false)
  canvas.discardActiveObject()
  canvas.clear()
  canvas.backgroundColor = ''
  canvas.setViewportTransform([1, 0, 0, 1, 0, 0])
  syncDotGrid()
  lastPlaced = null
  outputRegistry.clear()
  runtimeBlobs.clear()
  selectedImage.value = null
  canvas.requestRenderAll()
  scheduleSceneSave()
}

// ==================== 输出注册表（历史点击平移） ====================

function registerOutputObject(object: FabricObject): void {
  const data = objectData(object)
  if (data.kind === 'image' && typeof data.runId === 'string' && typeof data.outputIndex === 'number') {
    outputRegistry.set(`${data.runId}:${data.outputIndex}`, object)
  }
}

function unregisterOutputObject(object: FabricObject): void {
  const data = objectData(object)
  if (data.kind === 'image' && typeof data.runId === 'string' && typeof data.outputIndex === 'number') {
    outputRegistry.delete(`${data.runId}:${data.outputIndex}`)
  }
}

// 历史列表点击：输出在画布上时选中并平滑平移过去，返回是否命中
function panToRunOutput(runId: string, outputIndex: number): boolean {
  if (!canvas) return false
  const object = outputRegistry.get(`${runId}:${outputIndex}`)
  if (!object || !canvas.getObjects().includes(object)) return false
  const rect = object.getBoundingRect()
  canvas.setActiveObject(object)
  selectedImage.value = object
  canvas.requestRenderAll()
  panToScenePoint({ x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 })
  return true
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

// 序列化（含自定义 data）：图片像素不进快照，src 用 asset://<assetKey> 占位
function snapshotScene(): string {
  if (!canvas) return '{}'
  const json = canvas.toObject(['data']) as { objects?: Array<Record<string, unknown> & ObjectWithData> }
  for (const object of json.objects ?? []) {
    const assetKey = object.data?.assetKey
    if (object.type === 'image' && typeof assetKey === 'string') {
      object.src = `${ASSET_PROTOCOL}${assetKey}`
    }
  }
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
    canvas.getObjects().forEach(registerOutputObject)
    canvas.requestRenderAll()
    // 恢复的视口/缩放同步到圆点网格
    syncDotGrid()
  } catch (error) {
    console.error('Failed to restore creative scene:', error)
  }
}

defineExpose({
  placeOutput,
  panToRunOutput,
  addUploadedImage,
  getSelectedImageBlob,
  getMaskBlob,
  resetCanvas,
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
</style>
