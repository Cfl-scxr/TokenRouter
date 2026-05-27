import { apiClient } from '../client'
import type { DataShareExportTicket, DataShareNotice, DataShareSession, DataShareSessionFilters } from '../dataSharing'
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

export interface DataShareRequestPathPoint {
  request_path: string
  storage_bytes: number
  session_count: number
  total_tokens: number
}

export interface DataShareModelPoint {
  model: string
  storage_bytes: number
  session_count: number
  total_tokens: number
}

export interface DataShareUserAgentPoint {
  user_agent: string
  storage_bytes: number
  session_count: number
  total_tokens: number
}

export interface DataShareStats {
  session_count: number
  exportable_count: number
  non_exportable_count: number
  complete_count: number
  partial_count: number
  invalid_count: number
  total_storage_bytes: number
  total_tokens: number
  avg_tokens_per_session: number
  storage_trend: DataShareStoragePoint[]
  group_storage_breakdown: DataShareGroupStoragePoint[]
  request_path_breakdown: DataShareRequestPathPoint[]
  model_breakdown: DataShareModelPoint[]
  user_agent_breakdown: DataShareUserAgentPoint[]
}

export type DataShareCaptureSkipRuleMatchMode = 'contains' | 'equals'
export type DataShareCaptureSkipRuleFieldScope = 'system' | 'messages' | 'input' | 'instructions'

export interface DataShareCaptureSkipRule {
  id: string
  name: string
  enabled: boolean
  client_families: string[]
  request_paths: string[]
  field_scopes: DataShareCaptureSkipRuleFieldScope[]
  patterns: string[]
  case_sensitive: boolean
  match_mode: DataShareCaptureSkipRuleMatchMode
}

export interface DataShareStorageLimit {
  limit_bytes: number
  current_storage_bytes: number
  enabled: boolean
  exceeded: boolean
  usage_ratio: number
}

export interface AdminDataShareSessionFilters extends DataShareSessionFilters {
  user_id?: number
  user_name?: string
}

export async function getNotice(): Promise<DataShareNotice> {
  const { data } = await apiClient.get<DataShareNotice>('/admin/data-sharing/notice')
  return data
}

export async function updateNotice(content: string): Promise<DataShareNotice> {
  const { data } = await apiClient.put<DataShareNotice>('/admin/data-sharing/notice', { content })
  return data
}

export async function getSkipRules(): Promise<DataShareCaptureSkipRule[]> {
  const { data } = await apiClient.get<DataShareCaptureSkipRule[]>('/admin/data-sharing/skip-rules')
  return data
}

export async function updateSkipRules(rules: DataShareCaptureSkipRule[]): Promise<DataShareCaptureSkipRule[]> {
  const { data } = await apiClient.put<DataShareCaptureSkipRule[]>('/admin/data-sharing/skip-rules', { rules })
  return data
}

export async function getStorageLimit(): Promise<DataShareStorageLimit> {
  const { data } = await apiClient.get<DataShareStorageLimit>('/admin/data-sharing/storage-limit')
  return data
}

export async function updateStorageLimit(limitBytes: number): Promise<DataShareStorageLimit> {
  const { data } = await apiClient.put<DataShareStorageLimit>('/admin/data-sharing/storage-limit', { limit_bytes: limitBytes })
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

export async function createExportTicket(filters?: AdminDataShareSessionFilters): Promise<DataShareExportTicket> {
  const { data } = await apiClient.post<DataShareExportTicket>('/admin/data-sharing/export-ticket', null, {
    params: filters
  })
  return data
}

export async function createSessionExportTicket(id: number): Promise<DataShareExportTicket> {
  const { data } = await apiClient.post<DataShareExportTicket>(`/admin/data-sharing/sessions/${id}/export-ticket`)
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
  getSkipRules,
  updateSkipRules,
  getStorageLimit,
  updateStorageLimit,
  listSessions,
  getSession,
  deleteSession,
  batchDeleteSessions,
  createExportTicket,
  createSessionExportTicket,
  getStats
}

export default adminDataSharingAPI
