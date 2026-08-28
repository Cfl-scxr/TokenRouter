import { computed, ref } from 'vue'
import { getCreativeModels, type CreativeModelOption } from '@/api/creative'

// 模块级缓存：整个会话只拉一次模型目录
const loaded = ref(false)
const loading = ref(false)
const creativeModels = ref<CreativeModelOption[]>([])
let pendingLoad: Promise<boolean> | null = null

async function loadCreativeAccess(force = false): Promise<boolean> {
  if (loaded.value && !force) return creativeModels.value.length > 0
  if (pendingLoad && !force) return pendingLoad

  loading.value = true
  pendingLoad = (async () => {
    try {
      const models = await getCreativeModels()
      creativeModels.value = Array.isArray(models) ? models : []
      loaded.value = true
      return creativeModels.value.length > 0
    } catch {
      // 目录拉取失败按不可用处理，不暴露入口
      creativeModels.value = []
      loaded.value = true
      return false
    } finally {
      loading.value = false
      pendingLoad = null
    }
  })()

  return pendingLoad
}

export function useCreativeStudioAccess() {
  const canUseCreativeStudio = computed(() => creativeModels.value.length > 0)

  return {
    canUseCreativeStudio,
    creativeStudioAccessLoaded: computed(() => loaded.value),
    creativeStudioAccessLoading: computed(() => loading.value),
    refreshCreativeStudioAccess: loadCreativeAccess,
  }
}
