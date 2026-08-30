/**
 * 创作台核心状态机
 * 职责：模型目录、参数选择与持久恢复、创建 run（幂等重试）、轮询收割输出、历史关联本地素材、画布桥接。
 * 源图 / mask 不再经状态机管理：由视图在点击生成时从画布收集（选中的图片 + 画笔 mask）。
 * 轮询定时器在 composable 内注册 onBeforeUnmount 清理。
 */

import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  CREATIVE_RUN_TERMINAL_STATUSES,
  createCreativeRun,
  getCreativeModels,
  getCreativeRun,
  getCreativeRunOutputContent,
  getCreativeRuns,
  ackCreativeRunOutput,
  type CreativeModelOption,
  type CreativeOperation,
  type CreativeRun,
} from '@/api/creative'
import {
  LocalStoreQuotaError,
  clearAll,
  listAssets,
  loadSetting,
  outputAssetKey,
  saveAsset,
  saveSetting,
  type LocalAsset,
} from '@/utils/creativeLocalStore'

// 生成参数来源：视图在点击生成时从画布收集（选中的源图 + 画笔 mask）
export interface CreativeExportInput {
  sourceBlobs: Blob[]
  maskBlob: Blob | null
}

// 上次选择持久化的结构
interface CreativeSelectionSettings {
  optionKey: string
  operation: string
  imageSize: string
  aspectRatio: string
  quality: string
}

const SETTINGS_KEY = 'creative:selection'
// 清空本机数据的时间水位线：此后的历史列表只展示该时间之后创建的服务端任务
const CLEARED_AT_KEY = 'creative:clearedAt'
const PROMPT_MAX_LENGTH = 8000

// 轮询节奏：前 10 秒每 1s，之后每 3s
const POLL_FAST_INTERVAL = 1000
const POLL_SLOW_INTERVAL = 3000
const POLL_FAST_WINDOW = 10000

// 画布桥接：视图注册后，收割成功的输出自动放上画布，历史里的输出可一键导入画布
export interface CreativeCanvasBridge {
  // 收割成功（save + ack 后）把输出图片放到画布
  placeOutput(asset: { blob: Blob; runId: string; outputIndex: number }): void
  // 把历史里的本地输出素材放到画布（与自动上板同一入口）
  importToCanvas(blob: Blob, runId: string, outputIndex: number): void
}

// group + model 合成选项 key
export function creativeOptionKey(option: Pick<CreativeModelOption, 'group_id' | 'model'>): string {
  return `${option.group_id}::${option.model}`
}

export function useCreativeStudio() {
  const { t } = useI18n()

  // ==================== 状态 ====================

  const models = ref<CreativeModelOption[]>([])
  const loadingModels = ref(false)
  const selectedOptionKey = ref('')
  const operation = ref<CreativeOperation>('generate')
  const prompt = ref('')
  const imageSize = ref('')
  const aspectRatio = ref('1:1')
  // 生图画质档位（low/medium/high），仅 OpenAI 平台模型可选，空串 = 不指定（上游默认）
  const quality = ref('')
  const currentRun = ref<CreativeRun | null>(null)
  const runHistory = ref<CreativeRun[]>([])
  const loadingHistory = ref(false)
  const polling = ref(false)
  const busy = ref(false)
  const error = ref('')
  // 服务端 succeeded 但本地缺失 blob 的输出 key 集合（runId:index 不展示素材）
  const missingOutputKeys = ref<Set<string>>(new Set())
  // 本地输出素材索引：outputAssetKey → asset
  const outputAssetMap = ref<Map<string, LocalAsset>>(new Map())
  // 当前表单提交意图的幂等键：失败后重试复用，成功后重置
  const activeIdempotencyKey = ref('')
  // 画布桥接实例（视图在挂载时注册，卸载时可传 null 解绑）
  let canvasBridge: CreativeCanvasBridge | null = null

  // ==================== 计算属性 ====================

  const selectedOption = computed(
    () => models.value.find((m) => creativeOptionKey(m) === selectedOptionKey.value) ?? null,
  )

  const operationOptions = computed(() => selectedOption.value?.operations ?? [])

  const imageSizeOptions = computed(() => selectedOption.value?.image_sizes ?? [])

  // 可选画质档位（模型不支持时为空，参数面板隐藏画质行）
  const qualityOptions = computed(() => selectedOption.value?.qualities ?? [])

  // 估算费用直接使用模型目录返回的档位价格，避免创作台与模型广场价格口径不一致。
  const estimatedCost = computed(() => {
    const option = selectedOption.value
    if (!option) return null
    switch (imageSize.value) {
      case '2K':
        return option.price_2k
      case '4K':
        return option.price_4k
      default:
        return option.price_1k
    }
  })

  const canGenerate = computed(() => {
    if (!selectedOption.value || busy.value) return false
    if (!operationOptions.value.includes(operation.value)) return false
    // 提示词为空（纯空白）时禁止提交
    if (prompt.value.trim().length === 0) return false
    if (prompt.value.length > PROMPT_MAX_LENGTH) return false
    // 源图 / mask 由画布在点击生成时即时收集，这里不做前置拦截
    return true
  })

  // ==================== 模型目录与选择恢复 ====================

  async function loadModels(): Promise<void> {
    if (loadingModels.value) return
    loadingModels.value = true
    try {
      models.value = await getCreativeModels()
      await restoreSettings()
    } catch (e) {
      console.error('Failed to load creative models:', e)
      error.value = t('creative.error.loadModelsFailed')
    } finally {
      loadingModels.value = false
    }
  }

  // 从 settings 恢复上次选择；模型已下线时静默回退默认
  async function restoreSettings(): Promise<void> {
    try {
      const saved = await loadSetting<CreativeSelectionSettings>(SETTINGS_KEY)
      if (!saved) return
      if (models.value.some((m) => creativeOptionKey(m) === saved.optionKey)) {
        selectedOptionKey.value = saved.optionKey
      }
      if (saved.operation) operation.value = saved.operation
      if (saved.imageSize) imageSize.value = saved.imageSize
      if (saved.aspectRatio) aspectRatio.value = saved.aspectRatio
      if (typeof saved.quality === 'string') quality.value = saved.quality
      normalizeSelection()
    } catch (e) {
      console.error('Failed to restore creative settings:', e)
    }
  }

  // 选择变更后兜底：operation/imageSize/quality 必须在选项能力范围内
  function normalizeSelection(): void {
    const option = selectedOption.value
    if (!option) return
    if (!option.operations.includes(operation.value)) {
      operation.value = option.operations[0] ?? 'generate'
    }
    if (!option.image_sizes.includes(imageSize.value)) {
      imageSize.value = option.image_sizes[0] ?? ''
    }
    const qualities = option.qualities ?? []
    if (quality.value && !qualities.includes(quality.value)) {
      quality.value = ''
    }
  }

  function selectOption(key: string): void {
    selectedOptionKey.value = key
    normalizeSelection()
  }

  // 参数变化持久化，下次进入恢复
  watch(
    [selectedOptionKey, operation, imageSize, aspectRatio, quality],
    () => {
      const snapshot: CreativeSelectionSettings = {
        optionKey: selectedOptionKey.value,
        operation: operation.value,
        imageSize: imageSize.value,
        aspectRatio: aspectRatio.value,
        quality: quality.value,
      }
      void saveSetting(SETTINGS_KEY, snapshot).catch(() => {
        // 设置持久化失败不影响使用
      })
    },
    { flush: 'post' },
  )

  // ==================== 画布桥接 ====================

  // 视图挂载画布后注册桥接；传入 null 解绑（组件卸载时）
  function registerCanvasBridge(bridge: CreativeCanvasBridge | null): void {
    canvasBridge = bridge
  }

  // 历史里的输出导入画布：取本地素材调用画布桥接；素材缺失或画布未就绪时返回 false
  function importOutputToCanvas(runId: string, outputIndex: number): boolean {
    const asset = outputAssetMap.value.get(outputAssetKey(runId, outputIndex))
    if (!asset || !canvasBridge) return false
    try {
      canvasBridge.importToCanvas(asset.blob, runId, outputIndex)
      return true
    } catch (e) {
      console.error('Failed to import creative output to canvas:', e)
      return false
    }
  }

  // ==================== 创建 run ====================

  async function createRun(exported: CreativeExportInput): Promise<boolean> {
    if (busy.value) return false
    const option = selectedOption.value
    if (!option) {
      error.value = t('creative.error.noModel')
      return false
    }
    if (!option.operations.includes(operation.value)) {
      error.value = t('creative.error.operationNotSupported')
      return false
    }
    if (prompt.value.length > PROMPT_MAX_LENGTH) {
      error.value = t('creative.error.promptTooLong')
      return false
    }
    if ((operation.value === 'edit' || operation.value === 'inpaint') && exported.sourceBlobs.length === 0) {
      error.value = t('creative.error.sourceRequired')
      return false
    }
    if (operation.value === 'inpaint' && !exported.maskBlob) {
      error.value = t('creative.error.maskRequired')
      return false
    }

    busy.value = true
    error.value = ''
    try {
      // 同一表单提交意图复用同一幂等键，直到成功
      if (!activeIdempotencyKey.value) {
        activeIdempotencyKey.value = crypto.randomUUID()
      }
      const form = new FormData()
      form.append('group_id', option.group_id)
      form.append('model', option.model)
      form.append('operation', operation.value)
      form.append('prompt', prompt.value)
      form.append('image_size', imageSize.value)
      form.append('aspect_ratio', aspectRatio.value)
      form.append('response_mime_type', 'image/png')
      // 画质仅 OpenAI 平台模型可选，非空才提交（空 = 上游默认）
      if (quality.value) {
        form.append('quality', quality.value)
      }
      exported.sourceBlobs.forEach((blob, index) => {
        form.append('source_images[]', blob, `source-${index}.png`)
      })
      if (exported.maskBlob) {
        form.append('mask', exported.maskBlob, 'mask.png')
      }

      const run = await createCreativeRun(form, activeIdempotencyKey.value)
      // 提交成功，重置幂等键；失败重试时保留
      activeIdempotencyKey.value = ''
      currentRun.value = run
      upsertRunInHistory(run)
      startPolling(run.id)
      return true
    } catch (e) {
      error.value = extractErrorMessage(e) || t('creative.error.submitFailed')
      return false
    } finally {
      busy.value = false
    }
  }

  // ==================== 轮询与输出收割 ====================

  let pollTimer: ReturnType<typeof setTimeout> | null = null
  let pollStartedAt = 0
  // 历史刷新采用最新请求胜出，避免旧请求覆盖较新的任务与本地素材索引。
  let historyRefreshGeneration = 0

  function stopPolling(): void {
    if (pollTimer) {
      clearTimeout(pollTimer)
      pollTimer = null
    }
    polling.value = false
  }

  function startPolling(runId: string): void {
    stopPolling()
    polling.value = true
    pollStartedAt = Date.now()

    const tick = async () => {
      pollTimer = null
      try {
        const run = await getCreativeRun(runId)
        currentRun.value = run
        upsertRunInHistory(run)
        if (CREATIVE_RUN_TERMINAL_STATUSES.includes(run.status)) {
          stopPolling()
          if (run.status === 'succeeded') {
            await harvestOutputs(run, { placeOnCanvas: true })
          }
          await refreshHistory()
          return
        }
      } catch (e) {
        // 单次轮询失败不打断流程，按原节奏继续
        console.error('Creative run poll failed:', e)
      }
      const interval = Date.now() - pollStartedAt < POLL_FAST_WINDOW ? POLL_FAST_INTERVAL : POLL_SLOW_INTERVAL
      pollTimer = setTimeout(() => void tick(), interval)
    }

    pollTimer = setTimeout(() => void tick(), POLL_FAST_INTERVAL)
  }

  interface HarvestOptions {
    // 当前任务完成时自动放上画布；历史恢复只保存本地，不重复上板。
    placeOnCanvas?: boolean
    // 历史刷新传入工作副本，避免旧请求直接覆盖当前索引。
    assets?: Map<string, LocalAsset>
    missing?: Set<string>
    isCurrent?: () => boolean
  }

  // 终态 succeeded：逐个取回未 ack 的输出 → 存本地 → ack → 可选放上画布。
  // 历史刷新也复用该流程，因此页面重新进入时仍能收割尚未 ack 的 transient 输出。
  async function harvestOutputs(run: CreativeRun, options: HarvestOptions = {}): Promise<void> {
    const outputs = Array.isArray(run.outputs) ? run.outputs : []
    const assets = options.assets ?? new Map(outputAssetMap.value)
    const missing = options.missing ?? new Set(missingOutputKeys.value)
    for (const output of outputs) {
      if (options.isCurrent && !options.isCurrent()) return
      if (output.status !== 'succeeded') continue
      const key = outputAssetKey(run.id, output.output_index)
      if (output.acked_at && !assets.has(key)) {
        // ack 后 transient 已被服务端删除，当前浏览器没有副本时无法恢复。
        missing.add(key)
        continue
      }
      try {
        let asset = assets.get(key)
        if (!asset) {
          const blob = await getCreativeRunOutputContent(run.id, output.output_index)
          asset = {
            key,
            kind: 'output',
            blob,
            runId: run.id,
            outputIndex: output.output_index,
            createdAt: Date.now(),
          }
          await saveAsset(asset)
          assets.set(key, asset)
          // 保存成功后立即合并更新，不能等下一次历史刷新才显示图片。
          if (!options.isCurrent || options.isCurrent()) {
            const nextMap = new Map(outputAssetMap.value)
            nextMap.set(key, asset)
            outputAssetMap.value = nextMap
          }
        }

        // 本地已有素材时也重试 ack，但 ack 失败不能抹掉可用的本地图片。
        if (!output.acked_at) {
          try {
            await ackCreativeRunOutput(run.id, output.output_index)
          } catch (e) {
            console.error(`Failed to ack creative output ${key}:`, e)
          }
        }

        if (options.placeOnCanvas) {
          // 本地保存 + ack（或 ack 失败但本地已保存）后再上画布；画布异常不影响收割结果。
          try {
            canvasBridge?.placeOutput({ blob: asset.blob, runId: run.id, outputIndex: output.output_index })
          } catch (e) {
            console.error(`Failed to place creative output ${key} on canvas:`, e)
          }
        }
        missing.delete(key)
      } catch (e) {
        // 单个输出 transient 取回或本地保存失败只标记 missing，不中断其它输出。
        console.error(`Failed to harvest creative output ${key}:`, e)
        if (e instanceof LocalStoreQuotaError) {
          error.value = t('creative.error.quotaExceeded')
        }
        missing.add(key)
      }
    }
    if (!options.assets) {
      outputAssetMap.value = assets
      missingOutputKeys.value = missing
    }
  }

  // ==================== 历史与本地素材关联 ====================

  function upsertRunInHistory(run: CreativeRun): void {
    const index = runHistory.value.findIndex((r) => r.id === run.id)
    if (index >= 0) runHistory.value.splice(index, 1, run)
    else runHistory.value.unshift(run)
  }

  // 拉取服务端历史 + 本地输出素材索引；服务端标记成功但本地无 blob 的输出记为 missing。
  // 历史列表只展示本机清空时间之后创建的任务：更早的任务元数据仍在服务端，仅不再展示。
  async function refreshHistory(): Promise<void> {
    const generation = ++historyRefreshGeneration
    loadingHistory.value = true
    try {
      const page = await getCreativeRuns(1, 20)
      const clearedAt = await loadSetting<number>(CLEARED_AT_KEY)
      const items =
        typeof clearedAt === 'number' && clearedAt > 0
          ? page.items.filter(
              (run) => typeof run.created_at !== 'number' || run.created_at * 1000 > clearedAt,
            )
          : page.items
      if (generation !== historyRefreshGeneration) return
      runHistory.value = items
      // 只需输出素材索引（missing 判定用）；源图 / mask 素材已由画布自行管理
      const outputs = await listAssets('output')
      const map = new Map(outputs.map((a) => [a.key, a]))
      if (generation !== historyRefreshGeneration) return
      const missing = new Set<string>()
      for (const run of items) {
        await harvestOutputs(run, {
          assets: map,
          missing,
          placeOnCanvas: false,
          isCurrent: () => generation === historyRefreshGeneration,
        })
        if (generation !== historyRefreshGeneration) return
        for (const output of run.outputs ?? []) {
          if (output.status !== 'succeeded') continue
          // 收割失败或已 ack 且本地没有副本时保留缺失占位。
          const key = outputAssetKey(run.id, output.output_index)
          if (!map.has(key)) {
            missing.add(key)
          }
        }
      }
      if (generation !== historyRefreshGeneration) return
      // 合并历史快照期间终态收割刚保存的素材，避免旧快照覆盖最新内存索引。
      for (const [key, asset] of outputAssetMap.value) {
        if (!map.has(key)) map.set(key, asset)
      }
      for (const key of missing) {
        if (map.has(key)) missing.delete(key)
      }
      outputAssetMap.value = map
      missingOutputKeys.value = missing
      if (currentRun.value) {
        const fresh = page.items.find((r) => r.id === currentRun.value?.id)
        if (fresh) currentRun.value = fresh
      }
    } catch (e) {
      console.error('Failed to refresh creative history:', e)
    } finally {
      if (generation === historyRefreshGeneration) {
        loadingHistory.value = false
      }
    }
  }

  // ==================== 本地数据 ====================

  // 清空本机创作数据（素材 + 场景 + 设置）并重置内存状态；
  // 同时记录清空时间水位线，此后的历史列表只展示该时间之后创建的服务端任务。
  async function clearLocalData(): Promise<void> {
    stopPolling()
    // 使清空过程中仍在进行的历史请求失效，避免旧素材重新写回内存。
    historyRefreshGeneration++
    try {
      await clearAll()
    } catch (e) {
      error.value = extractErrorMessage(e) || t('creative.error.clearFailed')
      throw e
    }
    await saveSetting(CLEARED_AT_KEY, Date.now()).catch(() => {
      // 水位线写入失败不影响清空本身
    })
    currentRun.value = null
    runHistory.value = []
    outputAssetMap.value = new Map()
    missingOutputKeys.value = new Set()
    activeIdempotencyKey.value = ''
  }

  // ==================== 工具 ====================

  function extractErrorMessage(e: unknown): string {
    const message = (e as { message?: unknown })?.message
    return typeof message === 'string' ? message : ''
  }

  // 组件卸载时清理轮询定时器，避免内存泄漏与野回调
  onBeforeUnmount(() => {
    stopPolling()
  })

  return {
    // 状态
    models,
    loadingModels,
    selectedOptionKey,
    selectedOption,
    operation,
    prompt,
    imageSize,
    aspectRatio,
    quality,
    currentRun,
    runHistory,
    loadingHistory,
    polling,
    busy,
    error,
    missingOutputKeys,
    outputAssetMap,
    // 计算
    operationOptions,
    imageSizeOptions,
    qualityOptions,
    estimatedCost,
    canGenerate,
    // 方法
    loadModels,
    selectOption,
    createRun,
    refreshHistory,
    clearLocalData,
    registerCanvasBridge,
    importOutputToCanvas,
  }
}
