/**
 * useCreativeStudioAccess 测试：模块级缓存只拉一次模型目录。
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'

vi.mock('@/api/creative', () => ({
  getCreativeModels: vi.fn(),
}))

import { getCreativeModels } from '@/api/creative'

const mockedGetModels = vi.mocked(getCreativeModels)

describe('useCreativeStudioAccess', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
  })

  // 每个用例独立导入模块，重置模块级缓存
  async function importFresh() {
    return import('@/composables/useCreativeStudioAccess')
  }

  it('拉取到模型目录时 canUseCreativeStudio 为 true', async () => {
    mockedGetModels.mockResolvedValue([
      {
        group_id: 'g1',
        group_name: 'Group A',
        model: 'model-x',
        operations: ['generate'],
        image_sizes: ['1K'],
        price_1k: 1,
      },
    ])
    const { useCreativeStudioAccess } = await importFresh()

    const access = useCreativeStudioAccess()
    expect(access.canUseCreativeStudio.value).toBe(false)
    await access.refreshCreativeStudioAccess()

    expect(access.canUseCreativeStudio.value).toBe(true)
    expect(access.creativeStudioAccessLoaded.value).toBe(true)
  })

  it('目录为空数组时不可用', async () => {
    mockedGetModels.mockResolvedValue([])
    const { useCreativeStudioAccess } = await importFresh()

    const access = useCreativeStudioAccess()
    await access.refreshCreativeStudioAccess()

    expect(access.canUseCreativeStudio.value).toBe(false)
  })

  it('请求失败时按不可用处理', async () => {
    mockedGetModels.mockRejectedValue(new Error('network'))
    const { useCreativeStudioAccess } = await importFresh()

    const access = useCreativeStudioAccess()
    await access.refreshCreativeStudioAccess()

    expect(access.canUseCreativeStudio.value).toBe(false)
    expect(access.creativeStudioAccessLoaded.value).toBe(true)
  })

  it('模块级缓存：重复刷新不重复请求', async () => {
    mockedGetModels.mockResolvedValue([])
    const { useCreativeStudioAccess } = await importFresh()

    const first = useCreativeStudioAccess()
    await first.refreshCreativeStudioAccess()
    const second = useCreativeStudioAccess()
    await second.refreshCreativeStudioAccess()

    expect(mockedGetModels).toHaveBeenCalledTimes(1)
  })
})
