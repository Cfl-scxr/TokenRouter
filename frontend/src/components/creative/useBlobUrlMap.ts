/**
 * Blob → objectURL 的小工具：同 key 复用 URL，blob 变化时旧 URL 回收，
 * 组件卸载时统一回收，避免内存泄漏。
 */

import { onBeforeUnmount } from 'vue'

export function useBlobUrlMap() {
  const entries = new Map<string, { blob: Blob; url: string }>()

  function urlFor(key: string, blob: Blob): string {
    const cached = entries.get(key)
    if (cached && cached.blob === blob) return cached.url
    if (cached) URL.revokeObjectURL(cached.url)
    const url = URL.createObjectURL(blob)
    entries.set(key, { blob, url })
    return url
  }

  function release(key: string): void {
    const cached = entries.get(key)
    if (cached) {
      URL.revokeObjectURL(cached.url)
      entries.delete(key)
    }
  }

  function releaseAll(): void {
    entries.forEach((cached) => URL.revokeObjectURL(cached.url))
    entries.clear()
  }

  onBeforeUnmount(() => {
    releaseAll()
  })

  return { urlFor, release }
}
