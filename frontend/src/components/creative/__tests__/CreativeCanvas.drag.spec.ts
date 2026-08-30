import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CreativeCanvas from '@/components/creative/CreativeCanvas.vue'
import {
  CREATIVE_OUTPUT_DRAG_MIME,
  serializeCreativeOutputDrag,
} from '@/utils/creativeDrag'
import { outputAssetKey, type LocalAsset } from '@/utils/creativeLocalStore'

const fabricState = vi.hoisted(() => ({
  main: null as any,
  mask: null as any,
  images: [] as any[],
  scenePoint: { x: 200, y: 150 },
  viewportPoint: { x: 200, y: 150 },
}))

const storeMocks = vi.hoisted(() => ({
  saveAsset: vi.fn(),
  loadAsset: vi.fn(),
  loadSceneJson: vi.fn(),
  saveSceneJson: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('@/utils/creativeLocalStore', () => ({
  LocalStoreQuotaError: class LocalStoreQuotaError extends Error {},
  loadAsset: storeMocks.loadAsset,
  loadSceneJson: storeMocks.loadSceneJson,
  saveAsset: storeMocks.saveAsset,
  saveSceneJson: storeMocks.saveSceneJson,
  localAssetKey: (kind: string, id: string) => `${kind}:local:${id}`,
  outputAssetKey: (runId: string, outputIndex: number) => `output:${runId}:${outputIndex}`,
}))

vi.mock('fabric', () => {
  class MockObject {
    left = 0
    top = 0
    width = 0
    height = 0
    scaleX = 1
    scaleY = 1
    angle = 0
    data?: Record<string, unknown>
    selectable = true
    evented = true

    set(values: Record<string, unknown>): this {
      Object.assign(this, values)
      return this
    }

    setCoords(): void {}

    getBoundingRect(): { left: number; top: number; width: number; height: number } {
      return {
        left: this.left,
        top: this.top,
        width: this.width * this.scaleX,
        height: this.height * this.scaleY,
      }
    }
  }

  class MockImage extends MockObject {
    width = 400
    height = 200

    getElement(): Partial<HTMLImageElement> {
      return { naturalWidth: 400, naturalHeight: 200, width: 400, height: 200 }
    }

    static async fromURL(): Promise<MockImage> {
      const image = new MockImage()
      fabricState.images.push(image)
      return image
    }
  }

  class MockCanvas {
    viewportTransform: [number, number, number, number, number, number] = [1, 0, 0, 1, 0, 0]
    width = 800
    height = 600
    backgroundColor = ''
    objects: MockObject[] = []
    active: MockObject | null = null

    constructor() {
      if (!fabricState.main) fabricState.main = this
      else fabricState.mask = this
    }

    on(): void {}
    getWidth(): number { return this.width }
    getHeight(): number { return this.height }
    getZoom(): number { return this.viewportTransform[0] }
    getScenePoint = vi.fn(() => ({
      x: (fabricState.viewportPoint.x - this.viewportTransform[4]) / this.getZoom(),
      y: (fabricState.viewportPoint.y - this.viewportTransform[5]) / this.getZoom(),
    }))
    setViewportTransform(value: [number, number, number, number, number, number]): void {
      this.viewportTransform = value
    }
    setDimensions(value: { width: number; height: number }): void {
      this.width = value.width
      this.height = value.height
    }
    requestRenderAll(): void {}
    add(...objects: MockObject[]): void { this.objects.push(...objects) }
    remove(...objects: MockObject[]): void {
      this.objects = this.objects.filter((object) => !objects.includes(object))
    }
    setActiveObject(object: MockObject): void { this.active = object }
    getActiveObject(): MockObject | null { return this.active }
    getActiveObjects(): MockObject[] { return this.active ? [this.active] : [] }
    discardActiveObject(): void { this.active = null }
    getObjects(): MockObject[] { return this.objects }
    toObject(): { objects: never[] } { return { objects: [] } }
    clear(): void { this.objects = [] }
    dispose(): void {}
  }

  class MockText extends MockObject {
    text: string
    width = 40
    height = 12

    constructor(text: string) {
      super()
      this.text = text
    }
  }

  class MockBrush {
    color = ''
    width = 0
    constructor() {}
  }

  class MockPoint {
    constructor(public x: number, public y: number) {}
  }

  return {
    Canvas: MockCanvas,
    FabricImage: MockImage,
    PencilBrush: MockBrush,
    Point: MockPoint,
    Rect: MockObject,
    StaticCanvas: MockCanvas,
    Text: MockText,
  }
})

function makeDropEvent(dataTransfer: Record<string, unknown>): DragEvent {
  const event = new Event('drop', { bubbles: true, cancelable: true }) as DragEvent
  Object.defineProperty(event, 'dataTransfer', { value: dataTransfer })
  return event
}

async function drop(wrapper: ReturnType<typeof mount>, dataTransfer: Record<string, unknown>): Promise<void> {
  wrapper.find('.dot-grid').element.dispatchEvent(makeDropEvent(dataTransfer))
  await flushPromises()
}

function mountCanvas() {
  return mount(CreativeCanvas, {
    props: { operation: 'generate' },
    global: { stubs: { CropperModal: true, Icon: true } },
  })
}

describe('CreativeCanvas 拖放', () => {
  beforeEach(() => {
    fabricState.main = null
    fabricState.mask = null
    fabricState.images = []
    fabricState.scenePoint = { x: 200, y: 150 }
    fabricState.viewportPoint = { x: 200, y: 150 }
    storeMocks.saveAsset.mockReset().mockResolvedValue(undefined)
    storeMocks.loadAsset.mockReset().mockResolvedValue(null)
    storeMocks.loadSceneJson.mockReset().mockResolvedValue(null)
    storeMocks.saveSceneJson.mockReset().mockResolvedValue(undefined)
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      value: vi.fn(() => 'blob:creative-test'),
    })
    Object.defineProperty(URL, 'revokeObjectURL', {
      configurable: true,
      value: vi.fn(),
    })
    vi.stubGlobal('createImageBitmap', vi.fn(async () => ({
      width: 400,
      height: 200,
      close: vi.fn(),
    })))
  })

  it('将外部图片以落点为中心放到画布并保存为源素材', async () => {
    const wrapper = mountCanvas()
    expect(fabricState.main).not.toBeNull()
    expect(wrapper.find('.dot-grid').exists()).toBe(true)
    const file = new File(['image'], 'source.png', { type: 'image/png' })

    await drop(wrapper, {
      types: ['Files'],
      files: [file],
      getData: vi.fn(() => ''),
      dropEffect: 'copy',
    })

    expect(storeMocks.saveAsset).toHaveBeenCalledWith(expect.objectContaining({ kind: 'source', blob: file }))
    expect(fabricState.images).toHaveLength(1)
    expect(fabricState.images[0]).toMatchObject({
      left: 150,
      top: 125,
      originX: 'left',
      originY: 'top',
      scaleX: 0.25,
      scaleY: 0.25,
    })
    wrapper.unmount()
  })

  it('多图拖入时从落点按固定间距斜向错开，单个保存失败不阻塞后续文件', async () => {
    storeMocks.saveAsset.mockRejectedValueOnce(new Error('quota')).mockResolvedValue(undefined)
    const wrapper = mountCanvas()
    const first = new File(['1'], 'first.png', { type: 'image/png' })
    const second = new File(['2'], 'second.webp', { type: 'image/webp' })

    await drop(wrapper, {
      types: ['Files'],
      files: [first, second],
      getData: vi.fn(() => ''),
      dropEffect: 'copy',
    })

    expect(fabricState.images).toHaveLength(1)
    expect(fabricState.images[0]).toMatchObject({ left: 190, top: 165 })
    wrapper.unmount()
  })

  it('按当前场景坐标把历史 output 拖到落点并保留运行记录元数据', async () => {
    const runId = 'crun_0123456789abcdef'
    const asset: LocalAsset = {
      key: outputAssetKey(runId, 1),
      kind: 'output',
      blob: new Blob(['output'], { type: 'image/png' }),
      runId,
      outputIndex: 1,
      createdAt: Date.now(),
    }
    storeMocks.loadAsset.mockResolvedValue(asset)
    const wrapper = mountCanvas()
    fabricState.main.viewportTransform = [2, 0, 0, 2, 20, 30]
    fabricState.viewportPoint = { x: 620, y: 530 }

    await drop(wrapper, {
      types: [CREATIVE_OUTPUT_DRAG_MIME],
      files: [],
      getData: vi.fn((type: string) => type === CREATIVE_OUTPUT_DRAG_MIME
        ? serializeCreativeOutputDrag({ runId, outputIndex: 1 })
        : ''),
      dropEffect: 'copy',
    })

    expect(storeMocks.loadAsset).toHaveBeenCalledWith(asset.key)
    expect(fabricState.main.getScenePoint).toHaveBeenCalled()
    expect(fabricState.images[0]).toMatchObject({
      left: 250,
      top: 225,
      scaleX: 0.25,
      data: expect.objectContaining({ runId, outputIndex: 1, assetKey: asset.key }),
    })
    wrapper.unmount()
  })

  it('不支持的文件类型不会创建画布对象并返回错误', async () => {
    const wrapper = mountCanvas()
    const file = new File(['gif'], 'image.gif', { type: 'image/gif' })

    await drop(wrapper, {
      types: ['Files'],
      files: [file],
      getData: vi.fn(() => ''),
      dropEffect: 'copy',
    })

    expect(storeMocks.saveAsset).not.toHaveBeenCalled()
    expect(fabricState.images).toHaveLength(0)
    expect(wrapper.emitted('error')).toEqual([['creative.error.dropUnsupported']])
    wrapper.unmount()
  })
})
