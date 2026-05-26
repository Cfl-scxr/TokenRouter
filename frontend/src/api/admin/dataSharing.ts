import { apiClient } from '../client'
import type { DataShareNotice, DataShareSession, DataShareSessionFilters } from '../dataSharing'
import type { PaginatedResponse } from '@/types'

export interface DataShareStoragePoint {
  date: string
  storage_bytes: number
  session_count: number
}

export interface DataShareGroupStoragePoint {
  group_id: number
  group_name: string
  storage_bytes: number
  session_count: number
}

export interface DataShareStats {
  session_count: number
  exportable_count: number
  non_exportable_count: number
  total_storage_bytes: number
  total_tokens: number
  avg_tokens_per_session: number
  storage_trend: DataShareStoragePoint[]
  group_storage_breakdown: DataShareGroupStoragePoint[]
}

export interface AdminDataShareSessionFilters extends DataShareSessionFilters {
  user_id?: number
}

export async function getNotice(): Promise<DataShareNotice> {
  const { data } = await apiClient.get<DataShareNotice>('/admin/data-sharing/notice')
  return data
}

export async function updateNotice(content: string): Promise<DataShareNotice> {
  const { data } = await apiClient.put<DataShareNotice>('/admin/data-sharing/notice', { content })
  return data
}

export async function listSessions(
  page = 1,
  pageSize = 20,
  filters?: AdminDataShareSessionFilters
): Promise<PaginatedResponse<DataShareSession>> {
  const { data } = await apiClient.get<PaginatedResponse<DataShareSession>>('/admin/data-sharing/sessions', {
    params: { page, page_size: pageSize, ...filters }
  })
  return data
}

export async function getSession(id: number): Promise<DataShareSession> {
  const { data } = await apiClient.get<DataShareSession>(`/admin/data-sharing/sessions/${id}`)
  return data
}

export async function deleteSession(id: number): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete<{ deleted: boolean }>(`/admin/data-sharing/sessions/${id}`)
  return data
}

export async function batchDeleteSessions(
  ids: number[],
  filters?: AdminDataShareSessionFilters
): Promise<{ deleted: number }> {
  const { data } = await apiClient.post<{ deleted: number }>(
    '/admin/data-sharing/sessions/batch-delete',
    { ids },
    { params: filters }
  )
  return data
}

export async function exportSessions(filters?: AdminDataShareSessionFilters & { include_non_exportable?: boolean }): Promise<Blob> {
  const { data } = await apiClient.get<Blob>('/admin/data-sharing/export', {
    params: filters,
    responseType: 'blob'
  })
  return data
}

export async function getStats(filters?: AdminDataShareSessionFilters): Promise<DataShareStats> {
  const { data } = await apiClient.get<DataShareStats>('/admin/data-sharing/stats', {
    params: filters
  })
  return data
}

export const adminDataSharingAPI = {
  getNotice,
  updateNotice,
  listSessions,
  getSession,
  deleteSession,
  batchDeleteSessions,
  exportSessions,
  getStats
}

export default adminDataSharingAPI
