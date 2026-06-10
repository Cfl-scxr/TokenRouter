import { apiClient } from '../client'
import type { DataShareExportTicket, DataShareNotice, DataShareSession, DataShareSessionFilterOptions, DataShareSessionFilters } from '../dataSharing'
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

export interface DataShareQualityErrorPoint {
  error_code: string
  session_count: number
}

export interface DataShareInvalidUserPoint {
  user_id: number
  user_name: string
  user_email: string
  session_count: number
  invalid_count: number
  invalid_ratio: number
  storage_bytes: number
  total_tokens: number
}

export interface DataShareCaptureWorkerState {
  id: number
  job_kind: 'capture' | 'flush' | ''
}

export interface DataShareCaptureWorkerStats {
  queue_depth: number
  queue_capacity: number
  flush_queue_depth: number
  flush_queue_capacity: number
  worker_count: number
  worker_states?: DataShareCaptureWorkerState[] | null
  running_workers: number
  available_workers: number
  task_timeout_seconds: number
  compression_level: string
  submitted_total: number
  completed_total: number
  failed_total: number
  timeout_total: number
  dropped_total: number
  last_error: string
  last_error_at?: string | null
  last_success_at?: string | null
}

export interface DataShareCaptureBufferStats {
  enabled: boolean
  idle_flush_seconds: number
  max_sessions: number
  max_pending_events: number
  buffered_sessions: number
  pending_events: number
  flushing_sessions: number
  submitted_total: number
  flush_success_total: number
  flush_failed_total: number
  dropped_total: number
  last_flush_duration_millis: number
  last_error: string
  last_error_at?: string | null
  last_success_at?: string | null
}

export interface DataShareCaptureDurationBucket {
  label: string
  count: number
}

export interface DataShareCaptureDurationPart {
  key: string
  label: string
  category: string
  last_millis: number
  avg_millis: number
  p50_millis: number
  p95_millis: number
  max_millis: number
  sample_count: number
  buckets: DataShareCaptureDurationBucket[]
}

export interface DataShareCaptureDurationStats {
  window_size: number
  sample_count: number
  parts: DataShareCaptureDurationPart[]
}

export interface DataShareCaptureRuntimeSettings {
  worker_count: number
  queue_size: number
  flush_queue_size: number
  task_timeout_seconds: number
  compression_level: string
  buffer_enabled: boolean
  buffer_idle_flush_seconds: number
  buffer_max_sessions: number
  buffer_max_pending_events: number
  duration_window_size: number
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
  total_actual_cost: number
  avg_actual_cost_per_session: number
  storage_trend: DataShareStoragePoint[]
  group_storage_breakdown: DataShareGroupStoragePoint[]
  request_path_breakdown: DataShareRequestPathPoint[]
  model_breakdown: DataShareModelPoint[]
  user_agent_breakdown: DataShareUserAgentPoint[]
  quality_error_breakdown: DataShareQualityErrorPoint[]
  invalid_user_breakdown: DataShareInvalidUserPoint[]
  capture_worker?: DataShareCaptureWorkerStats | null
  capture_buffer?: DataShareCaptureBufferStats | null
  capture_durations?: DataShareCaptureDurationStats | null
}

export type DataShareCaptureSkipRuleMatchMode = 'contains' | 'equals'
export type DataShareCaptureSkipRuleFieldScope = 'system' | 'messages' | 'input' | 'instructions'

export interface DataShareCaptureSkipRule {
  id: string
  name: string
  enabled: boolean
  client_families: string[]
  request_paths: string[]
  models: string[]
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

export async function getRuntimeSettings(): Promise<DataShareCaptureRuntimeSettings> {
  const { data } = await apiClient.get<DataShareCaptureRuntimeSettings>('/admin/data-sharing/runtime-settings')
  return data
}

export async function updateRuntimeSettings(settings: DataShareCaptureRuntimeSettings): Promise<DataShareCaptureRuntimeSettings> {
  const { data } = await apiClient.put<DataShareCaptureRuntimeSettings>('/admin/data-sharing/runtime-settings', settings)
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

export async function getFilterOptions(): Promise<DataShareSessionFilterOptions> {
  const { data } = await apiClient.get<DataShareSessionFilterOptions>('/admin/data-sharing/filter-options')
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
  getRuntimeSettings,
  updateRuntimeSettings,
  listSessions,
  getFilterOptions,
  getSession,
  deleteSession,
  batchDeleteSessions,
  createExportTicket,
  createSessionExportTicket,
  getStats
}

export default adminDataSharingAPI
