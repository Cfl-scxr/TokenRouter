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
  payload_encoding?: string
  payload_bytes?: number
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

export interface DataShareExportTicket {
  token: string
  download_url: string
  filename: string
  encoding: 'jsonl' | 'zstd'
  expires_at: string
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

export async function createExportTicket(filters?: DataShareSessionFilters): Promise<DataShareExportTicket> {
  const { data } = await apiClient.post<DataShareExportTicket>('/data-sharing/export-ticket', null, {
    params: filters
  })
  return data
}

export async function createSessionExportTicket(id: number): Promise<DataShareExportTicket> {
  const { data } = await apiClient.post<DataShareExportTicket>(`/data-sharing/sessions/${id}/export-ticket`)
  return data
}

export function startTicketDownload(ticket: DataShareExportTicket) {
  if (!ticket.download_url) return
  const link = document.createElement('a')
  link.href = resolveDownloadURL(ticket.download_url)
  link.download = ticket.filename || ''
  link.rel = 'noopener'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

function resolveDownloadURL(downloadURL: string): string {
  if (/^https?:\/\//i.test(downloadURL)) return downloadURL
  const apiBase = (apiClient.defaults.baseURL || '/api/v1').replace(/\/$/, '')
  const path = downloadURL.startsWith('/api/v1/')
    ? downloadURL.slice('/api/v1'.length)
    : downloadURL
  return apiBase + (path.startsWith('/') ? path : `/${path}`)
}

export const dataSharingAPI = {
  getNotice,
  confirmNotice,
  listSessions,
  getSession,
  createExportTicket,
  createSessionExportTicket,
  startTicketDownload
}

export default dataSharingAPI
