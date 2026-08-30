/**
 * useCreativeStudio 状态机测试
 * mock API 层与本地存储层，fake timers 驱动轮询节奏。
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@/api/creative', () => ({
  CREATIVE_RUN_TERMINAL_STATUSES: ['succeeded', 'failed', 'cancelled', 'result_lost'],
  getCreativeModels: vi.fn(),
  createCreativeRun: vi.fn(),
  getCreativeRuns: vi.fn(),
  getCreativeRun: vi.fn(),
  getCreativeRunOutputContent: vi.fn(),
  ackCreativeRunOutput: vi.fn(),
}))

// 保留真实工具（key 组合、错误类），只 mock 存储副作用
vi.mock('@/utils/creativeLocalStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/utils/creativeLocalStore')>()
  return {
    ...actual,
    saveAsset: vi.fn(),
    deleteAsset: vi.fn(),
    listAssets: vi.fn(),
    loadAsset: vi.fn(),
    loadSetting: vi.fn(),
    saveSetting: vi.fn(),
    clearAll: vi.fn(),
  }
})

import * as creativeApi from '@/api/creative'
import * as localStore from '@/utils/creativeLocalStore'
import { useCreativeStudio, creativeOptionKey } from '@/composables/useCreativeStudio'
import type { CreativeRun } from '@/api/creative'

const mockedApi = vi.mocked(creativeApi)
const mockedStore = vi.mocked(localStore)

const MODEL = {
  group_id: 'g1',
  group_name: 'Group A',
  model: 'model-x',
  operations: ['generate', 'edit', 'inpaint'],
  image_sizes: ['1K', '2K'],
  price_1k: 2,
  price_2k: 3,
  price_4k: 4,
}

function makeRun(partial: Partial<CreativeRun> & Pick<CreativeRun, 'id' | 'status'>): CreativeRun {
  return {
    operation: 'generate',
    model: MODEL.model,
    group_id: MODEL.group_id,
    requested_output_count: 1,
    outputs: [],
    ...partial,
  }
}

// 在真实组件 setup 中挂载 composable，保证 onBeforeUnmount 有上下文
function mountStudio() {
  let studio!: ReturnType<typeof useCreativeStudio>
  const wrapper = mount(
    defineComponent({
      setup() {
        studio = useCreativeStudio()
        return () => h('div')
      },
    }),
  )
  return { studio, wrapper }
}

describe('useCreativeStudio', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    localStorage.clear()

    mockedApi.getCreativeModels.mockResolvedValue([{ ...MODEL }])
    mockedApi.createCreativeRun.mockResolvedValue(makeRun({ id: 'run-1', status: 'queued' }))
    mockedApi.getCreativeRuns.mockResolvedValue({ items: [], total: 0 })
    mockedApi.getCreativeRun.mockResolvedValue(makeRun({ id: 'run-1', status: 'succeeded' }))
    mockedApi.getCreativeRunOutputContent.mockResolvedValue(new Blob(['img'], { type: 'image/png' }))
    mockedApi.ackCreativeRunOutput.mockResolvedValue(undefined)
    mockedStore.saveAsset.mockResolvedValue({} as localStore.LocalAsset)
    mockedStore.deleteAsset.mockResolvedValue(undefined)
    mockedStore.loadAsset.mockResolvedValue(null)
    mockedStore.loadSetting.mockResolvedValue(null)
    mockedStore.saveSetting.mockResolvedValue(undefined)
    mockedStore.clearAll.mockResolvedValue(undefined)
    // listAssets 按 kind 返回，默认全部为空
    mockedStore.listAssets.mockImplementation((kind: string) =>
      Promise.resolve(kind === 'output' || kind === 'source' || kind === 'mask' ? [] : []),
    )
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  // 加载模型并选中唯一选项
  async function setupStudio() {
    const { studio, wrapper } = mountStudio()
    await studio.loadModels()
    studio.selectOption(creativeOptionKey(MODEL))
    return { studio, wrapper }
  }

  describe('createRun 校验', () => {
    it('edit 无源图时直接报错且不发起请求', async () => {
      const { studio } = await setupStudio()
      studio.operation.value = 'edit'

      const ok = await studio.createRun({ sourceBlobs: [], maskBlob: null })

      expect(ok).toBe(false)
      expect(studio.error.value).toBe('creative.error.sourceRequired')
      expect(mockedApi.createCreativeRun).not.toHaveBeenCalled()
    })

    it('inpaint 有源图但无 mask 时直接报错且不发起请求', async () => {
      const { studio } = await setupStudio()
      studio.operation.value = 'inpaint'

      const ok = await studio.createRun({ sourceBlobs: [new Blob(['a'])], maskBlob: null })

      expect(ok).toBe(false)
      expect(studio.error.value).toBe('creative.error.maskRequired')
      expect(mockedApi.createCreativeRun).not.toHaveBeenCalled()
    })

    it('操作不在模型支持列表时拒绝提交', async () => {
      const { studio } = await setupStudio()
      studio.operation.value = 'video'

      const ok = await studio.createRun({ sourceBlobs: [], maskBlob: null })

      expect(ok).toBe(false)
      expect(studio.error.value).toBe('creative.error.operationNotSupported')
      expect(mockedApi.createCreativeRun).not.toHaveBeenCalled()
    })
  })

  describe('canGenerate 表单门禁', () => {
    it('提示词为空或纯空白时禁止生成', async () => {
      const { studio } = await setupStudio()

      studio.prompt.value = ''
      expect(studio.canGenerate.value).toBe(false)

      studio.prompt.value = '   '
      expect(studio.canGenerate.value).toBe(false)

      studio.prompt.value = '一只猫'
      expect(studio.canGenerate.value).toBe(true)
    })
  })

  describe('estimatedCost 定价', () => {
    it('按所选尺寸读取模型目录的实际档位价格', async () => {
      const { studio } = await setupStudio()

      studio.imageSize.value = '1K'
      expect(studio.estimatedCost.value).toBe(2)
      studio.imageSize.value = '2K'
      expect(studio.estimatedCost.value).toBe(3)
      studio.imageSize.value = '4K'
      expect(studio.estimatedCost.value).toBe(4)
    })
  })

  describe('createRun 提交', () => {
    it('成功路径：FormData 字段齐全并启动轮询', async () => {
      const { studio } = await setupStudio()
      studio.prompt.value = '一只猫'
      const sourceBlob = new Blob(['src'], { type: 'image/png' })
      mockedApi.createCreativeRun.mockResolvedValue(makeRun({ id: 'run-9', status: 'queued' }))
      mockedApi.getCreativeRun.mockResolvedValue(makeRun({ id: 'run-9', status: 'succeeded' }))

      const ok = await studio.createRun({ sourceBlobs: [sourceBlob], maskBlob: null })

      expect(ok).toBe(true)
      expect(studio.error.value).toBe('')
      expect(mockedApi.createCreativeRun).toHaveBeenCalledTimes(1)

      const form = mockedApi.createCreativeRun.mock.calls[0][0] as FormData
      expect(form.get('group_id')).toBe('g1')
      expect(form.get('model')).toBe('model-x')
      expect(form.get('operation')).toBe('generate')
      expect(form.get('prompt')).toBe('一只猫')
      expect(form.get('image_size')).toBe('1K')
      expect(form.get('aspect_ratio')).toBe('1:1')
      expect(form.get('output_count')).toBeNull()
      expect(form.get('response_mime_type')).toBe('image/png')
      expect(form.getAll('source_images[]')).toHaveLength(1)

      // 进入轮询：前进 1s 后发出第一次详情查询
      expect(studio.polling.value).toBe(true)
      await vi.advanceTimersByTimeAsync(1000)
      expect(mockedApi.getCreativeRun).toHaveBeenCalledWith('run-9')
    })

    it('inpaint 时附加 mask 文件与幂等键', async () => {
      const { studio } = await setupStudio()
      studio.operation.value = 'inpaint'

      const ok = await studio.createRun({
        sourceBlobs: [new Blob(['src'])],
        maskBlob: new Blob(['mask']),
      })

      expect(ok).toBe(true)
      const [, key] = mockedApi.createCreativeRun.mock.calls[0]
      expect(typeof key).toBe('string')
      expect(key).toHaveLength(36)
      const form = mockedApi.createCreativeRun.mock.calls[0][0] as FormData
      const mask = form.get('mask') as File
      expect(mask).toBeInstanceOf(Blob)
      expect(mask.name).toBe('mask.png')
    })

    it('失败重试复用同一幂等键，成功后重置', async () => {
      const { studio } = await setupStudio()
      mockedApi.createCreativeRun.mockRejectedValue(new Error('network'))

      const first = await studio.createRun({ sourceBlobs: [], maskBlob: null })
      const retry = await studio.createRun({ sourceBlobs: [], maskBlob: null })

      expect(first).toBe(false)
      expect(retry).toBe(false)
      const keyAfterFail = mockedApi.createCreativeRun.mock.calls[1][1]
      expect(mockedApi.createCreativeRun.mock.calls[0][1]).toBe(keyAfterFail)

      // 成功后续重试仍用同一 key（同一次提交意图），随后重置
      mockedApi.createCreativeRun.mockResolvedValue(makeRun({ id: 'run-2', status: 'queued' }))
      mockedApi.getCreativeRun.mockResolvedValue(makeRun({ id: 'run-2', status: 'succeeded' }))
      const success = await studio.createRun({ sourceBlobs: [], maskBlob: null })
      expect(success).toBe(true)
      expect(mockedApi.createCreativeRun.mock.calls[2][1]).toBe(keyAfterFail)

      // 新一次提交生成新 key
      mockedApi.createCreativeRun.mockRejectedValue(new Error('network'))
      await studio.createRun({ sourceBlobs: [], maskBlob: null })
      const newKey = mockedApi.createCreativeRun.mock.calls[3][1]
      expect(newKey).not.toBe(keyAfterFail)
      expect(typeof newKey).toBe('string')
    })
  })

  describe('轮询节奏', () => {
    it('前 10s 每 1s 查询，之后每 3s，终态停止', async () => {
      const { studio } = await setupStudio()
      // 前 11 次返回 running，之后 succeeded
      let calls = 0
      mockedApi.getCreativeRun.mockImplementation(() => {
        calls += 1
        return Promise.resolve(
          makeRun({ id: 'run-1', status: calls <= 11 ? 'running' : 'succeeded' }),
        )
      })

      await studio.createRun({ sourceBlobs: [], maskBlob: null })
      expect(mockedApi.getCreativeRun).not.toHaveBeenCalled()

      await vi.advanceTimersByTimeAsync(1000)
      expect(mockedApi.getCreativeRun).toHaveBeenCalledTimes(1)

      // 前进到 t=10s：累计 10 次（t=1..10s 各一次）
      await vi.advanceTimersByTimeAsync(9000)
      expect(mockedApi.getCreativeRun).toHaveBeenCalledTimes(10)

      // t=10s 之后改为 3s 间隔
      await vi.advanceTimersByTimeAsync(3000)
      expect(mockedApi.getCreativeRun).toHaveBeenCalledTimes(11)
      await vi.advanceTimersByTimeAsync(3000)
      expect(mockedApi.getCreativeRun).toHaveBeenCalledTimes(12)

      // 第 12 次查询返回 succeeded，轮询停止
      expect(studio.polling.value).toBe(false)
      expect(studio.currentRun.value?.status).toBe('succeeded')
      const countAfterTerminal = mockedApi.getCreativeRun.mock.calls.length
      await vi.advanceTimersByTimeAsync(10000)
      expect(mockedApi.getCreativeRun).toHaveBeenCalledTimes(countAfterTerminal)
    })

    it('result_lost 终态直接停止，不尝试取回内容', async () => {
      const { studio } = await setupStudio()
      mockedApi.getCreativeRun.mockResolvedValue(
        makeRun({ id: 'run-1', status: 'result_lost' }),
      )

      await studio.createRun({ sourceBlobs: [], maskBlob: null })
      await vi.advanceTimersByTimeAsync(1000)

      expect(studio.polling.value).toBe(false)
      expect(studio.currentRun.value?.status).toBe('result_lost')
      expect(mockedApi.getCreativeRunOutputContent).not.toHaveBeenCalled()
    })

    it('组件卸载后定时器清理，不再轮询', async () => {
      const { studio, wrapper } = await setupStudio()
      mockedApi.getCreativeRun.mockResolvedValue(makeRun({ id: 'run-1', status: 'running' }))

      await studio.createRun({ sourceBlobs: [], maskBlob: null })
      await vi.advanceTimersByTimeAsync(1000)
      expect(mockedApi.getCreativeRun).toHaveBeenCalledTimes(1)

      wrapper.unmount()
      await vi.advanceTimersByTimeAsync(10000)
      expect(mockedApi.getCreativeRun).toHaveBeenCalledTimes(1)
    })
  })

  describe('输出收割', () => {
    function succeededRunWithOutputs() {
      return makeRun({
        id: 'run-1',
        status: 'succeeded',
        outputs: [
          { output_index: 0, status: 'succeeded' },
          { output_index: 1, status: 'succeeded' },
        ],
      })
    }

    it('succeeded 后逐输出取回 → 存本地 → ack', async () => {
      const { studio } = await setupStudio()
      mockedApi.getCreativeRun.mockResolvedValue(succeededRunWithOutputs())

      await studio.createRun({ sourceBlobs: [], maskBlob: null })
      await vi.advanceTimersByTimeAsync(1000)

      expect(mockedApi.getCreativeRunOutputContent).toHaveBeenCalledTimes(2)
      expect(mockedApi.getCreativeRunOutputContent).toHaveBeenNthCalledWith(1, 'run-1', 0)
      expect(mockedApi.getCreativeRunOutputContent).toHaveBeenNthCalledWith(2, 'run-1', 1)

      expect(mockedStore.saveAsset).toHaveBeenCalledTimes(2)
      const savedKeys = mockedStore.saveAsset.mock.calls.map(([asset]) => asset.key)
      expect(savedKeys).toEqual(['output:run-1:0', 'output:run-1:1'])
      for (const [asset] of mockedStore.saveAsset.mock.calls) {
        expect(asset.kind).toBe('output')
        expect(asset.runId).toBe('run-1')
      }

      expect(mockedApi.ackCreativeRunOutput).toHaveBeenCalledTimes(2)
      expect(mockedApi.ackCreativeRunOutput).toHaveBeenNthCalledWith(1, 'run-1', 0)
      expect(mockedApi.ackCreativeRunOutput).toHaveBeenNthCalledWith(2, 'run-1', 1)
      expect(studio.missingOutputKeys.value.size).toBe(0)
    })

    it('单个输出取回失败仅标 missing，不影响其它输出', async () => {
      const { studio } = await setupStudio()
      mockedApi.getCreativeRun.mockResolvedValue(succeededRunWithOutputs())
      mockedApi.getCreativeRunOutputContent.mockImplementation((_runId: string, index: number) => {
        if (index === 0) {
          return Promise.reject(Object.assign(new Error('gone'), { status: 410 }))
        }
        return Promise.resolve(new Blob(['img-1'], { type: 'image/png' }))
      })
      // 终态后 refreshHistory 会再次尝试自动恢复；输出 0 每次都失败，输出 1 已在本地。
      mockedApi.getCreativeRuns.mockResolvedValue({
        items: [succeededRunWithOutputs()],
        total: 1,
      })
      mockedStore.listAssets.mockImplementation((kind: string) =>
        Promise.resolve(
          kind === 'output'
            ? [
                {
                  key: 'output:run-1:1',
                  kind: 'output' as const,
                  blob: new Blob(['img']),
                  runId: 'run-1',
                  outputIndex: 1,
                  createdAt: 1,
                },
              ]
            : [],
        ),
      )

      await studio.createRun({ sourceBlobs: [], maskBlob: null })
      await vi.advanceTimersByTimeAsync(1000)

      // 输出 0 缺失，输出 1 仍正常收割
      expect(studio.missingOutputKeys.value.has('output:run-1:0')).toBe(true)
      expect(studio.missingOutputKeys.value.has('output:run-1:1')).toBe(false)
      expect(mockedStore.saveAsset).toHaveBeenCalledTimes(1)
      expect(mockedStore.saveAsset.mock.calls[0][0].outputIndex).toBe(1)
      expect(mockedApi.ackCreativeRunOutput).toHaveBeenCalledWith('run-1', 1)
    })

    it('已 ack 的输出跳过取回', async () => {
      const { studio } = await setupStudio()
      mockedApi.getCreativeRun.mockResolvedValue(
        makeRun({
          id: 'run-1',
          status: 'succeeded',
          outputs: [{ output_index: 0, status: 'succeeded', acked_at: 1725000000 }],
        }),
      )

      await studio.createRun({ sourceBlobs: [], maskBlob: null })
      await vi.advanceTimersByTimeAsync(1000)

      expect(mockedApi.getCreativeRunOutputContent).not.toHaveBeenCalled()
      expect(mockedApi.ackCreativeRunOutput).not.toHaveBeenCalled()
    })
  })

  describe('画布桥接', () => {
    function succeededRunWithTwoOutputs() {
      return makeRun({
        id: 'run-1',
        status: 'succeeded',
        outputs: [
          { output_index: 0, status: 'succeeded' },
          { output_index: 1, status: 'succeeded' },
        ],
      })
    }

    it('收割成功（save + ack 后）经桥接把输出放上画布', async () => {
      const { studio } = await setupStudio()
      const blob0 = new Blob(['img-0'], { type: 'image/png' })
      const blob1 = new Blob(['img-1'], { type: 'image/png' })
      mockedApi.getCreativeRun.mockResolvedValue(succeededRunWithTwoOutputs())
      mockedApi.getCreativeRunOutputContent.mockImplementation((_runId: string, index: number) =>
        Promise.resolve(index === 0 ? blob0 : blob1),
      )
      const bridge = { placeOutput: vi.fn(), importToCanvas: vi.fn() }
      studio.registerCanvasBridge(bridge)

      await studio.createRun({ sourceBlobs: [], maskBlob: null })
      await vi.advanceTimersByTimeAsync(1000)

      // 先 save + ack，再上板
      expect(mockedApi.ackCreativeRunOutput).toHaveBeenCalledTimes(2)
      expect(bridge.placeOutput).toHaveBeenCalledTimes(2)
      expect(bridge.placeOutput).toHaveBeenNthCalledWith(1, { blob: blob0, runId: 'run-1', outputIndex: 0 })
      expect(bridge.placeOutput).toHaveBeenNthCalledWith(2, { blob: blob1, runId: 'run-1', outputIndex: 1 })
      expect(studio.missingOutputKeys.value.size).toBe(0)
    })

    it('多输出上板按顺序等待前一张完成，避免并发覆盖画布状态', async () => {
      const { studio } = await setupStudio()
      mockedApi.getCreativeRun.mockResolvedValue(succeededRunWithTwoOutputs())

      let resolveFirst!: () => void
      const firstPlacementDone = new Promise<void>((resolve) => {
        resolveFirst = resolve
      })
      const started: number[] = []
      const bridge = {
        placeOutput: vi.fn(({ outputIndex }: { outputIndex: number }) => {
          started.push(outputIndex)
          return outputIndex === 0 ? firstPlacementDone : Promise.resolve()
        }),
        importToCanvas: vi.fn(),
      }
      studio.registerCanvasBridge(bridge)

      await studio.createRun({ sourceBlobs: [], maskBlob: null })
      await vi.advanceTimersByTimeAsync(1000)

      // 第一张尚未完成时，收割流程不能启动第二次画布委托。
      expect(started).toEqual([0])
      resolveFirst()
      await vi.advanceTimersByTimeAsync(0)

      expect(started).toEqual([0, 1])
    })

    it('桥接 placeOutput 抛异常不影响收割与 ack', async () => {
      const { studio } = await setupStudio()
      mockedApi.getCreativeRun.mockResolvedValue(succeededRunWithTwoOutputs())
      const bridge = {
        placeOutput: vi.fn(() => {
          throw new Error('canvas not ready')
        }),
        importToCanvas: vi.fn(),
      }
      studio.registerCanvasBridge(bridge)

      await studio.createRun({ sourceBlobs: [], maskBlob: null })
      await vi.advanceTimersByTimeAsync(1000)

      expect(mockedApi.ackCreativeRunOutput).toHaveBeenCalledTimes(2)
      expect(studio.missingOutputKeys.value.size).toBe(0)
    })

    it('importOutputToCanvas 取本地素材委托桥接；素材缺失或桥接异常时返回 false', async () => {
      const { studio } = await setupStudio()

      // 本地无素材：不调用桥接
      const bridge = { placeOutput: vi.fn(), importToCanvas: vi.fn() }
      studio.registerCanvasBridge(bridge)
      expect(studio.importOutputToCanvas('run-9', 0)).toBe(false)
      expect(bridge.importToCanvas).not.toHaveBeenCalled()

      // 本地有素材：委托桥接导入
      const blob = new Blob(['img'], { type: 'image/png' })
      studio.outputAssetMap.value = new Map([['output:run-9:0', { key: 'output:run-9:0', kind: 'output', blob, createdAt: 1 }]])
      expect(studio.importOutputToCanvas('run-9', 0)).toBe(true)
      expect(bridge.importToCanvas).toHaveBeenCalledWith(blob, 'run-9', 0)

      // 桥接抛异常时吞掉并返回 false
      studio.registerCanvasBridge({
        placeOutput: vi.fn(),
        importToCanvas: () => {
          throw new Error('boom')
        },
      })
      expect(studio.importOutputToCanvas('run-9', 0)).toBe(false)
    })
  })

  describe('本地数据', () => {
    it('clearLocalData 失败时写入错误文案并向上抛出', async () => {
      const { studio } = await setupStudio()
      mockedStore.clearAll.mockRejectedValue(new Error('disk full'))

      await expect(studio.clearLocalData()).rejects.toThrow('disk full')
      expect(studio.error.value).toBe('disk full')
    })

    it('clearLocalData 调 clearAll 并重置内存状态', async () => {
      const { studio } = await setupStudio()
      studio.currentRun.value = makeRun({ id: 'run-1', status: 'succeeded' })
      studio.runHistory.value = [makeRun({ id: 'run-1', status: 'succeeded' })]

      await studio.clearLocalData()

      expect(mockedStore.clearAll).toHaveBeenCalledTimes(1)
      // 清空后写入时间水位线，供历史列表过滤旧任务
      expect(mockedStore.saveSetting).toHaveBeenCalledWith('creative:clearedAt', expect.any(Number))
      expect(studio.currentRun.value).toBeNull()
      expect(studio.runHistory.value).toEqual([])
      expect(studio.missingOutputKeys.value.size).toBe(0)
    })
  })

  describe('refreshHistory', () => {
    it('按清空水位线隐藏旧任务，新任务仍展示', async () => {
      const { studio } = await setupStudio()
      const now = Date.now()
      mockedStore.loadSetting.mockImplementation((key: string) =>
        key === 'creative:clearedAt' ? Promise.resolve(now - 1000) : Promise.resolve(null),
      )
      mockedApi.getCreativeRuns.mockResolvedValue({
        items: [
          makeRun({ id: 'old-run', status: 'succeeded', created_at: Math.floor((now - 2000) / 1000) }),
          makeRun({ id: 'new-run', status: 'succeeded', created_at: Math.floor(now / 1000) }),
        ],
        total: 2,
      })

      await studio.refreshHistory()

      expect(studio.runHistory.value.map((r) => r.id)).toEqual(['new-run'])
    })

    it('历史刷新自动恢复未 ack 的服务端输出并关联本地素材', async () => {
      const { studio } = await setupStudio()
      const run = makeRun({
        id: 'run-7',
        status: 'succeeded',
        outputs: [
          { output_index: 0, status: 'succeeded' },
          { output_index: 1, status: 'succeeded' },
        ],
      })
      mockedApi.getCreativeRuns.mockResolvedValue({ items: [run], total: 1 })
      // 本地只有输出 0，输出 1 仍在服务端 transient 中。
      const localOutput = {
        key: 'output:run-7:0',
        kind: 'output' as const,
        blob: new Blob(['img']),
        runId: 'run-7',
        outputIndex: 0,
        createdAt: 1,
      }
      mockedStore.listAssets.mockImplementation((kind: string) =>
        Promise.resolve(kind === 'output' ? [localOutput] : []),
      )

      await studio.refreshHistory()

      expect(studio.runHistory.value.map((r) => r.id)).toEqual(['run-7'])
      expect(studio.outputAssetMap.value.get('output:run-7:0')).toEqual(localOutput)
      expect(studio.outputAssetMap.value.has('output:run-7:1')).toBe(true)
      expect(studio.missingOutputKeys.value.has('output:run-7:1')).toBe(false)
      expect(studio.missingOutputKeys.value.has('output:run-7:0')).toBe(false)
      expect(mockedApi.getCreativeRunOutputContent).toHaveBeenCalledWith('run-7', 1)
      expect(mockedApi.ackCreativeRunOutput).toHaveBeenCalledWith('run-7', 1)
    })

    it('历史刷新自动收割尚未 ack 的服务端输出', async () => {
      const { studio } = await setupStudio()
      const run = makeRun({
        id: 'run-history-recover',
        status: 'succeeded',
        outputs: [{ output_index: 0, status: 'succeeded' }],
      })
      const blob = new Blob(['history-image'], { type: 'image/png' })
      mockedApi.getCreativeRuns.mockResolvedValue({ items: [run], total: 1 })
      mockedApi.getCreativeRunOutputContent.mockResolvedValue(blob)

      await studio.refreshHistory()

      expect(mockedApi.getCreativeRunOutputContent).toHaveBeenCalledWith('run-history-recover', 0)
      expect(mockedStore.saveAsset).toHaveBeenCalledWith(
        expect.objectContaining({
          key: 'output:run-history-recover:0',
          kind: 'output',
          blob,
        }),
      )
      expect(mockedApi.ackCreativeRunOutput).toHaveBeenCalledWith('run-history-recover', 0)
      expect(studio.outputAssetMap.value.get('output:run-history-recover:0')?.blob).toBe(blob)
      expect(studio.missingOutputKeys.value.has('output:run-history-recover:0')).toBe(false)
    })

    it('本地已有未 ack 素材时只重试 ack，不重复下载', async () => {
      const { studio } = await setupStudio()
      const run = makeRun({
        id: 'run-local-recover',
        status: 'succeeded',
        outputs: [{ output_index: 0, status: 'succeeded' }],
      })
      const localOutput = {
        key: 'output:run-local-recover:0',
        kind: 'output' as const,
        blob: new Blob(['local-image']),
        runId: 'run-local-recover',
        outputIndex: 0,
        createdAt: 1,
      }
      mockedApi.getCreativeRuns.mockResolvedValue({ items: [run], total: 1 })
      mockedStore.listAssets.mockImplementation((kind: string) =>
        Promise.resolve(kind === 'output' ? [localOutput] : []),
      )

      await studio.refreshHistory()

      expect(mockedApi.getCreativeRunOutputContent).not.toHaveBeenCalled()
      expect(mockedApi.ackCreativeRunOutput).toHaveBeenCalledWith('run-local-recover', 0)
      expect(studio.outputAssetMap.value.get(localOutput.key)).toEqual(localOutput)
      expect(studio.missingOutputKeys.value.has(localOutput.key)).toBe(false)
    })

    it('本地保存成功但 ack 失败时仍保留素材，不标记 missing', async () => {
      const { studio } = await setupStudio()
      const run = makeRun({
        id: 'run-ack-retry',
        status: 'succeeded',
        outputs: [{ output_index: 0, status: 'succeeded' }],
      })
      mockedApi.getCreativeRuns.mockResolvedValue({ items: [run], total: 1 })
      mockedApi.ackCreativeRunOutput.mockRejectedValue(new Error('ack network error'))

      await studio.refreshHistory()

      expect(mockedStore.saveAsset).toHaveBeenCalledTimes(1)
      expect(studio.outputAssetMap.value.has('output:run-ack-retry:0')).toBe(true)
      expect(studio.missingOutputKeys.value.has('output:run-ack-retry:0')).toBe(false)
    })

    it('已 ack 且本地无素材时不尝试从服务端恢复', async () => {
      const { studio } = await setupStudio()
      const run = makeRun({
        id: 'run-acked-missing',
        status: 'succeeded',
        outputs: [{ output_index: 0, status: 'succeeded', acked_at: 1725000000 }],
      })
      mockedApi.getCreativeRuns.mockResolvedValue({ items: [run], total: 1 })

      await studio.refreshHistory()

      expect(mockedApi.getCreativeRunOutputContent).not.toHaveBeenCalled()
      expect(mockedApi.ackCreativeRunOutput).not.toHaveBeenCalled()
      expect(studio.missingOutputKeys.value.has('output:run-acked-missing:0')).toBe(true)
    })

    it('并发历史刷新只接受最新请求，旧响应不能覆盖新列表', async () => {
      const { studio } = await setupStudio()
      const oldRun = makeRun({ id: 'run-old', status: 'succeeded' })
      const newRun = makeRun({
        id: 'run-new',
        status: 'succeeded',
        outputs: [{ output_index: 0, status: 'succeeded' }],
      })
      let resolveOld!: (page: { items: CreativeRun[]; total: number }) => void
      mockedApi.getCreativeRuns
        .mockImplementationOnce(
          () =>
            new Promise((resolve) => {
              resolveOld = resolve
            }),
        )
        .mockResolvedValueOnce({ items: [newRun], total: 1 })

      const oldRefresh = studio.refreshHistory()
      await Promise.resolve()
      const newRefresh = studio.refreshHistory()
      await newRefresh

      resolveOld({ items: [oldRun], total: 1 })
      await oldRefresh

      expect(studio.runHistory.value.map((run) => run.id)).toEqual(['run-new'])
      expect(studio.outputAssetMap.value.has('output:run-new:0')).toBe(true)
    })
  })
})
