import { apiClient } from './client'
import type { PaginatedResponse } from '@/types'

export interface DataShareNotice {
  content: string
  version: number
  updated_at: string
}

export interface DataShareSession {
  id: number
  trajectory_id: string
  session_id: string
  dataset: string
  provider: string
  model: string
  request_path: string
  user_agent: string
  status: string
  is_final_snapshot: boolean
  source_request_count: number
  system_prompt?: string | null
  tools?: Array<Record<string, unknown>>
  messages?: Array<Record<string, unknown>>
  usage?: Record<string, unknown>
  meta?: Record<string, unknown>
  session_json?: Record<string, unknown>
  exportable: boolean
  quality_status: 'complete' | 'partial' | 'invalid'
  quality_errors: string[]
  storage_bytes: number
  input_tokens: number
  output_tokens: number
  total_tokens: number
  user_id: number
  user_name?: string
  user_email?: string
  api_key_id: number
  api_key_name?: string
  group_id: number
  group_name?: string
  created_at: string
  ended_at?: string | null
  updated_at: string
}

export interface DataShareSessionFilters {
  ids?: number[] | string
  exclude_ids?: number[] | string
  select_all?: boolean
  search?: string
  api_key_id?: number
  api_key_name?: string
  group_id?: number
  group_name?: string
  request_path?: string
  user_agent?: string
  provider?: string
  model?: string
  exportable?: boolean | 'all'
  quality_status?: 'complete' | 'partial' | 'invalid' | 'all'
  start_date?: string
  end_date?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export async function getNotice(groupId?: number | null): Promise<DataShareNotice> {
  const { data } = await apiClient.get<DataShareNotice>('/data-sharing/notice', {
    params: groupId ? { group_id: groupId } : undefined
  })
  return data
}

export async function confirmNotice(groupId: number, version: number): Promise<{
  confirmed: boolean
  group_id: number
  version: number
  confirmed_at: string
}> {
  const { data } = await apiClient.post('/data-sharing/confirm', {
    group_id: groupId,
    version
  })
  return data
}

export async function listSessions(
  page = 1,
  pageSize = 20,
  filters?: DataShareSessionFilters
): Promise<PaginatedResponse<DataShareSession>> {
  const { data } = await apiClient.get<PaginatedResponse<DataShareSession>>('/data-sharing/sessions', {
    params: { page, page_size: pageSize, ...filters }
  })
  return data
}

export async function getSession(id: number): Promise<DataShareSession> {
  const { data } = await apiClient.get<DataShareSession>(`/data-sharing/sessions/${id}`)
  return data
}

export async function exportSessions(filters?: DataShareSessionFilters): Promise<Blob> {
  const { data } = await apiClient.get<Blob>('/data-sharing/export', {
    params: filters,
    responseType: 'blob'
  })
  return data
}

export async function exportSession(id: number): Promise<Blob> {
  const { data } = await apiClient.get<Blob>(`/data-sharing/sessions/${id}/export`, {
    responseType: 'blob'
  })
  return data
}

export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

export const dataSharingAPI = {
  getNotice,
  confirmNotice,
  listSessions,
  getSession,
  exportSessions,
  exportSession,
  downloadBlob
}

export default dataSharingAPI
