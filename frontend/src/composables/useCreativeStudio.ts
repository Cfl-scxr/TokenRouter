/**
 * 创作台核心状态机
 * 职责：模型目录、参数选择与持久恢复、创建 run（幂等重试）、轮询收割输出、历史关联本地素材。
 * 轮询定时器在 composable 内注册 onBeforeUnmount 清理。
 */

import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  CREATIVE_RUN_TERMINAL_STATUSES,
  cancelCreativeRun,
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
  deleteAsset,
  listAssets,
  loadAsset,
  loadSetting,
  localAssetKey,
  outputAssetKey,
  saveAsset,
  saveSetting,
  type LocalAsset,
} from '@/utils/creativeLocalStore'

// 生成参数来源：画布导出 + 本地上传的合成
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
  outputCount: number
}

const SETTINGS_KEY = 'creative:selection'
// 清空本机数据的时间水位线：此后的历史列表只展示该时间之后创建的服务端任务
const CLEARED_AT_KEY = 'creative:clearedAt'
const MASK_KEY = localAssetKey('mask', 'current')
const PROMPT_MAX_LENGTH = 8000

// 轮询节奏：前 10 秒每 1s，之后每 3s
const POLL_FAST_INTERVAL = 1000
const POLL_SLOW_INTERVAL = 3000
const POLL_FAST_WINDOW = 10000

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
  const outputCount = ref(1)
  const sourceAssets = ref<LocalAsset[]>([])
  const maskAsset = ref<LocalAsset | null>(null)
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

  // ==================== 计算属性 ====================

  const selectedOption = computed(
    () => models.value.find((m) => creativeOptionKey(m) === selectedOptionKey.value) ?? null,
  )

  const operationOptions = computed(() => selectedOption.value?.operations ?? [])

  const imageSizeOptions = computed(() => selectedOption.value?.image_sizes ?? [])

  // 估算费用：price_1k 为 1K 单价，2K 按两倍估算，供用户提交前参考
  const estimatedCost = computed(() => {
    const option = selectedOption.value
    if (!option || typeof option.price_1k !== 'number') return null
    const sizeMultiplier = imageSize.value === '2K' ? 2 : 1
    return option.price_1k * sizeMultiplier * outputCount.value
  })

  const canGenerate = computed(() => {
    if (!selectedOption.value || busy.value) return false
    if (!operationOptions.value.includes(operation.value)) return false
    if (prompt.value.length > PROMPT_MAX_LENGTH) return false
    if ((operation.value === 'edit' || operation.value === 'inpaint') && sourceAssets.value.length === 0)
      return false
    if (operation.value === 'inpaint' && !maskAsset.value) return false
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
      if (typeof saved.outputCount === 'number') outputCount.value = saved.outputCount
      normalizeSelection()
    } catch (e) {
      console.error('Failed to restore creative settings:', e)
    }
  }

  // 选择变更后兜底：operation/imageSize 必须在选项能力范围内
  function normalizeSelection(): void {
    const option = selectedOption.value
    if (!option) return
    if (!option.operations.includes(operation.value)) {
      operation.value = option.operations[0] ?? 'generate'
    }
    if (!option.image_sizes.includes(imageSize.value)) {
      imageSize.value = option.image_sizes[0] ?? ''
    }
  }

  function selectOption(key: string): void {
    selectedOptionKey.value = key
    normalizeSelection()
  }

  // 参数变化持久化，下次进入恢复
  watch(
    [selectedOptionKey, operation, imageSize, aspectRatio, outputCount],
    () => {
      const snapshot: CreativeSelectionSettings = {
        optionKey: selectedOptionKey.value,
        operation: operation.value,
        imageSize: imageSize.value,
        aspectRatio: aspectRatio.value,
        outputCount: outputCount.value,
      }
      void saveSetting(SETTINGS_KEY, snapshot).catch(() => {
        // 设置持久化失败不影响使用
      })
    },
    { flush: 'post' },
  )

  // ==================== 本地素材 ====================

  // 上传/画布导出图作为源图：入本地库并加入当前表单
  async function addSourceAsset(blob: Blob): Promise<LocalAsset> {
    const localId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
    const asset: LocalAsset = {
      key: localAssetKey('source', localId),
      kind: 'source',
      blob,
      createdAt: Date.now(),
    }
    try {
      await saveAsset(asset)
    } catch (e) {
      // 配额不足给出明确提示，其余按原样抛出
      if (e instanceof LocalStoreQuotaError) {
        error.value = t('creative.error.quotaExceeded')
      }
      throw e
    }
    sourceAssets.value = [...sourceAssets.value, asset]
    return asset
  }

  async function removeSourceAsset(key: string): Promise<void> {
    await deleteAsset(key).catch(() => {
      // 本地删除失败不阻塞表单状态
    })
    sourceAssets.value = sourceAssets.value.filter((a) => a.key !== key)
  }

  async function setMaskAsset(blob: Blob): Promise<void> {
    const asset: LocalAsset = { key: MASK_KEY, kind: 'mask', blob, createdAt: Date.now() }
    await saveAsset(asset)
    maskAsset.value = asset
  }

  async function clearMask(): Promise<void> {
    await deleteAsset(MASK_KEY).catch(() => {})
    maskAsset.value = null
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
      form.append('output_count', String(outputCount.value))
      form.append('response_mime_type', 'image/png')
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
            await harvestOutputs(run)
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

  // 终态 succeeded：逐个取回未 ack 的输出 → 存本地 → ack
  async function harvestOutputs(run: CreativeRun): Promise<void> {
    const outputs = Array.isArray(run.outputs) ? run.outputs : []
    for (const output of outputs) {
      if (output.status !== 'succeeded' || output.acked_at) continue
      const key = outputAssetKey(run.id, output.output_index)
      try {
        const blob = await getCreativeRunOutputContent(run.id, output.output_index)
        await saveAsset({
          key,
          kind: 'output',
          blob,
          runId: run.id,
          outputIndex: output.output_index,
          createdAt: Date.now(),
        })
        await ackCreativeRunOutput(run.id, output.output_index)
        missingOutputKeys.value.delete(key)
      } catch (e) {
        // 单个输出 transient 取回失败（410 / result_lost）只标记 missing，不中断其它输出；
        // 本地配额不足时单独提示用户下载备份
        console.error(`Failed to harvest creative output ${key}:`, e)
        if (e instanceof LocalStoreQuotaError) {
          error.value = t('creative.error.quotaExceeded')
        }
        missingOutputKeys.value.add(key)
      }
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
      runHistory.value = items
      const [sources, outputs] = await Promise.all([listAssets('source'), listAssets('output')])
      sourceAssets.value = sources
      const map = new Map(outputs.map((a) => [a.key, a]))
      outputAssetMap.value = map
      const missing = new Set<string>()
      for (const run of items) {
        for (const output of run.outputs ?? []) {
          if (output.status !== 'succeeded') continue
          const key = outputAssetKey(run.id, output.output_index)
          if (!map.has(key)) missing.add(key)
        }
      }
      missingOutputKeys.value = missing
      if (!maskAsset.value) {
        maskAsset.value = await loadAsset(MASK_KEY)
      }
      if (currentRun.value) {
        const fresh = page.items.find((r) => r.id === currentRun.value?.id)
        if (fresh) currentRun.value = fresh
      }
    } catch (e) {
      console.error('Failed to refresh creative history:', e)
    } finally {
      loadingHistory.value = false
    }
  }

  // ==================== 取消与清空 ====================

  async function cancelRun(): Promise<boolean> {
    if (!currentRun.value || CREATIVE_RUN_TERMINAL_STATUSES.includes(currentRun.value.status)) {
      return false
    }
    try {
      const run = await cancelCreativeRun(currentRun.value.id)
      currentRun.value = run
      upsertRunInHistory(run)
      stopPolling()
      await refreshHistory()
      return true
    } catch (e) {
      error.value = extractErrorMessage(e) || t('creative.error.cancelFailed')
      return false
    }
  }

  // 清空本机创作数据（素材 + 场景 + 设置）并重置内存状态；
  // 同时记录清空时间水位线，此后的历史列表只展示该时间之后创建的服务端任务。
  async function clearLocalData(): Promise<void> {
    stopPolling()
    try {
      await clearAll()
    } catch (e) {
      error.value = extractErrorMessage(e) || t('creative.error.clearFailed')
      throw e
    }
    await saveSetting(CLEARED_AT_KEY, Date.now()).catch(() => {
      // 水位线写入失败不影响清空本身
    })
    sourceAssets.value = []
    maskAsset.value = null
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
    outputCount,
    sourceAssets,
    maskAsset,
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
    estimatedCost,
    canGenerate,
    // 方法
    loadModels,
    selectOption,
    addSourceAsset,
    removeSourceAsset,
    setMaskAsset,
    clearMask,
    createRun,
    refreshHistory,
    cancelRun,
    clearLocalData,
  }
}
