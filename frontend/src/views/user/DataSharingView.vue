<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="grid gap-4 md:grid-cols-4">
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <Icon name="database" size="md" class="text-blue-600 dark:text-blue-400" />
            <div>
              <p class="text-xs text-gray-500 dark:text-gray-400">Session</p>
              <p class="text-xl font-semibold text-gray-900 dark:text-white">{{ pagination.total }}</p>
            </div>
          </div>
        </div>
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <Icon name="download" size="md" class="text-emerald-600 dark:text-emerald-400" />
            <div>
              <p class="text-xs text-gray-500 dark:text-gray-400">合格</p>
              <p class="text-xl font-semibold text-gray-900 dark:text-white">{{ exportableCount }}</p>
            </div>
          </div>
        </div>
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <Icon name="cube" size="md" class="text-amber-600 dark:text-amber-400" />
            <div>
              <p class="text-xs text-gray-500 dark:text-gray-400">占用空间</p>
              <p class="text-xl font-semibold text-gray-900 dark:text-white">{{ formatBytes(totalStorageBytes) }}</p>
            </div>
          </div>
        </div>
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <Icon name="chart" size="md" class="text-purple-600 dark:text-purple-400" />
            <div>
              <p class="text-xs text-gray-500 dark:text-gray-400">Token</p>
              <p class="text-xl font-semibold text-gray-900 dark:text-white">{{ formatNumber(totalTokens) }}</p>
            </div>
          </div>
        </div>
      </div>

      <TablePageLayout>
        <template #filters>
          <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
            <div class="flex flex-1 flex-wrap items-center gap-3">
              <div class="relative w-full sm:w-64">
                <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input
                  v-model="filters.search"
                  type="text"
                  class="input pl-10"
                  placeholder="搜索 session、轨迹或模型"
                  @input="handleFilterChange"
                />
              </div>
              <Select v-model="filters.quality_status" :options="qualityOptions" class="w-40" @change="handleFilterChange" />
              <Select v-model="filters.request_path" :options="requestPathOptions" class="w-52" @change="handleFilterChange" />
              <input v-model="filters.start_date" type="date" class="input w-40" @change="handleFilterChange" />
              <input v-model="filters.end_date" type="date" class="input w-40" @change="handleFilterChange" />
            </div>
            <div class="flex flex-wrap justify-end gap-3">
              <button class="btn btn-secondary" :disabled="loading" @click="loadSessions">
                <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
              </button>
              <button class="btn btn-primary" :disabled="exporting || selectedCount === 0" @click="downloadSelected">
                <Icon name="download" size="md" class="mr-2" />
                下载已选 JSONL
              </button>
            </div>
          </div>
        </template>

        <template #table>
          <DataTable
            :columns="columns"
            :data="sessions"
            :loading="loading"
            :server-side-sort="true"
            default-sort-key="created_at"
            default-sort-order="desc"
            @sort="handleSort"
          >
            <template #header-select>
              <div class="flex flex-col items-center gap-1 normal-case">
                <span v-if="selectedCount > 0" class="whitespace-nowrap text-[11px] font-medium leading-none text-primary-600 dark:text-primary-400" :title="selectionSummary">
                  已选 {{ formatNumber(selectedCount) }}
                </span>
                <input
                  :checked="allMatchingSelected"
                  :disabled="pagination.total === 0"
                  :indeterminate="selectionIndeterminate"
                  type="checkbox"
                  class="rounded border-gray-300 text-primary-600"
                  title="选择当前筛选条件下的所有条目"
                  @change="toggleSelectAll"
                />
              </div>
            </template>
            <template #cell-select="{ row }">
              <input :checked="isSelected(row.id)" type="checkbox" class="rounded border-gray-300 text-primary-600" @change="toggleSelect(row.id)" />
            </template>
            <template #cell-session_id="{ value, row }">
              <div class="max-w-xs">
                <p class="truncate font-medium text-gray-900 dark:text-white">{{ value }}</p>
                <p class="truncate text-xs text-gray-500 dark:text-gray-400">{{ row.trajectory_id }}</p>
              </div>
            </template>
            <template #cell-model="{ value }">
              <span class="badge badge-gray">{{ value || '-' }}</span>
            </template>
            <template #cell-request_path="{ value }">
              <span class="badge badge-gray">{{ value || '-' }}</span>
            </template>
            <template #cell-quality_status="{ value, row }">
              <span :class="['badge', qualityBadgeClass(value)]">
                {{ qualityLabel(value) }}<span v-if="value === 'invalid' && row.quality_errors?.length"> {{ row.quality_errors.length }}</span>
              </span>
            </template>
            <template #cell-storage_bytes="{ value }">
              {{ formatBytes(value) }}
            </template>
            <template #cell-total_tokens="{ value }">
              {{ formatNumber(value) }}
            </template>
            <template #cell-created_at="{ value }">
              {{ formatDate(value) }}
            </template>
            <template #cell-actions="{ row }">
              <div class="flex items-center gap-1">
                <button class="btn btn-ghost btn-sm" @click="openDetail(row)">
                  <Icon name="eye" size="sm" class="mr-1" />
                  查看
                </button>
                <button class="btn btn-ghost btn-sm" @click="downloadOne(row)">
                  <Icon name="download" size="sm" class="mr-1" />
                  下载
                </button>
              </div>
            </template>
            <template #empty>
              <EmptyState title="暂无数据共享记录" description="使用数据共享分组产生的对话数据会显示在这里。" />
            </template>
          </DataTable>
        </template>

        <template #pagination>
          <Pagination
            v-if="pagination.total > 0"
            :page="pagination.page"
            :total="pagination.total"
            :page-size="pagination.page_size"
            @update:page="handlePageChange"
            @update:pageSize="handlePageSizeChange"
          />
        </template>
      </TablePageLayout>
    </div>

    <BaseDialog :show="detailOpen" title="数据共享详情" width="extra-wide" @close="detailOpen = false">
      <div v-if="detailLoading" class="flex h-48 items-center justify-center">
        <LoadingSpinner />
      </div>
      <div v-else-if="selectedSession" class="space-y-4">
        <div class="grid gap-3 md:grid-cols-5">
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">Session</p>
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ selectedSession.session_id }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">模型</p>
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ selectedSession.model || '-' }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">请求路径</p>
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ selectedSession.request_path || '-' }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">Token</p>
            <p class="text-sm font-medium text-gray-900 dark:text-white">{{ formatNumber(selectedSession.total_tokens) }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">空间</p>
            <p class="text-sm font-medium text-gray-900 dark:text-white">{{ formatBytes(selectedSession.storage_bytes) }}</p>
          </div>
        </div>
        <pre class="max-h-[60vh] overflow-auto rounded-lg bg-gray-950 p-4 text-xs leading-relaxed text-gray-100">{{ prettySession }}</pre>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import { dataSharingAPI, type DataShareSession, type DataShareSessionFilters } from '@/api/dataSharing'
import { useAppStore } from '@/stores/app'
import type { Column } from '@/components/common/types'

const appStore = useAppStore()

const sessions = ref<DataShareSession[]>([])
const selectedSession = ref<DataShareSession | null>(null)
// 选中状态支持两种模式：显式 ID 列表，以及“当前筛选条件全集 + 排除列表”。
const selectedIds = ref<Set<number>>(new Set())
const excludedIds = ref<Set<number>>(new Set())
const selectAllMatching = ref(false)
const loading = ref(false)
const exporting = ref(false)
const detailOpen = ref(false)
const detailLoading = ref(false)

const pagination = reactive({ page: 1, page_size: 20, total: 0, pages: 1 })
const sortState = reactive({ sort_by: 'created_at', sort_order: 'desc' as 'asc' | 'desc' })
const filters = reactive({
  search: '',
  request_path: 'all',
  quality_status: 'all' as 'all' | 'complete' | 'partial' | 'invalid',
  start_date: '',
  end_date: ''
})

const qualityOptions = [
  { value: 'all', label: '全部质量' },
  { value: 'complete', label: '完整' },
  { value: 'partial', label: '部分完整' },
  { value: 'invalid', label: '无效' }
]

const columns: Column[] = [
  { key: 'select', label: '' },
  { key: 'session_id', label: 'Session', sortable: true },
  { key: 'provider', label: 'Provider', sortable: true },
  { key: 'request_path', label: '请求路径', sortable: true },
  { key: 'model', label: '模型', sortable: true },
  { key: 'quality_status', label: '质量', sortable: true },
  { key: 'storage_bytes', label: '空间', sortable: true },
  { key: 'total_tokens', label: 'Token', sortable: true },
  { key: 'created_at', label: '创建时间', sortable: true },
  { key: 'actions', label: '操作' }
]

const exportableCount = computed(() => sessions.value.filter(item => item.exportable).length)
const totalStorageBytes = computed(() => sessions.value.reduce((sum, item) => sum + (item.storage_bytes || 0), 0))
const totalTokens = computed(() => sessions.value.reduce((sum, item) => sum + (item.total_tokens || 0), 0))
const selectedCount = computed(() => {
  if (selectAllMatching.value) {
    return Math.max(pagination.total - excludedIds.value.size, 0)
  }
  return selectedIds.value.size
})
const allMatchingSelected = computed(() => selectAllMatching.value && pagination.total > 0 && excludedIds.value.size === 0)
const selectionIndeterminate = computed(() => selectedCount.value > 0 && !allMatchingSelected.value)
const selectionSummary = computed(() => {
  if (selectAllMatching.value) {
    return `已选择当前筛选条件下 ${formatNumber(selectedCount.value)} 条`
  }
  return `已选择 ${formatNumber(selectedCount.value)} 条`
})
const prettySession = computed(() => JSON.stringify(selectedSession.value?.session_json || selectedSession.value, null, 2))
const requestPathOptions = computed(() => {
  const values = new Set<string>()
  for (const row of sessions.value) {
    if (row.request_path) values.add(row.request_path)
  }
  return [
    { value: 'all', label: '全部路径' },
    ...Array.from(values).sort().map(value => ({ value, label: value }))
  ]
})

let filterTimer: number | null = null

function buildFilters(): DataShareSessionFilters {
  const out: DataShareSessionFilters = {
    sort_by: sortState.sort_by,
    sort_order: sortState.sort_order
  }
  if (filters.search.trim()) out.search = filters.search.trim()
  if (filters.request_path !== 'all') out.request_path = filters.request_path
  if (filters.quality_status !== 'all') out.quality_status = filters.quality_status
  if (filters.start_date) out.start_date = filters.start_date
  if (filters.end_date) out.end_date = filters.end_date
  return out
}

async function loadSessions() {
  loading.value = true
  try {
    const res = await dataSharingAPI.listSessions(pagination.page, pagination.page_size, buildFilters())
    sessions.value = res.items
    pagination.total = res.total
    pagination.pages = res.pages
  } catch (error) {
    appStore.showError('加载数据共享记录失败')
  } finally {
    loading.value = false
  }
}

function handleFilterChange() {
  pagination.page = 1
  clearSelection()
  if (filterTimer) window.clearTimeout(filterTimer)
  filterTimer = window.setTimeout(loadSessions, 250)
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadSessions()
}

function handlePageChange(page: number) {
  pagination.page = page
  loadSessions()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadSessions()
}

function clearSelection() {
  selectedIds.value = new Set()
  excludedIds.value = new Set()
  selectAllMatching.value = false
}

function isSelected(id: number) {
  return selectAllMatching.value ? !excludedIds.value.has(id) : selectedIds.value.has(id)
}

function toggleSelect(id: number) {
  if (selectAllMatching.value) {
    const next = new Set(excludedIds.value)
    if (next.has(id)) {
      next.delete(id)
    } else {
      next.add(id)
    }
    excludedIds.value = next
    return
  }
  const next = new Set(selectedIds.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  selectedIds.value = next
}

function toggleSelectAll(event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  if (!checked) {
    clearSelection()
    return
  }
  selectedIds.value = new Set()
  excludedIds.value = new Set()
  selectAllMatching.value = true
}

function buildSelectionFilters(): DataShareSessionFilters {
  const out = buildFilters()
  if (selectAllMatching.value) {
    out.select_all = true
    const excluded = Array.from(excludedIds.value)
    if (excluded.length) out.exclude_ids = excluded.join(',')
    return out
  }
  const ids = Array.from(selectedIds.value)
  if (ids.length) out.ids = ids.join(',')
  return out
}

async function openDetail(row: DataShareSession) {
  detailOpen.value = true
  detailLoading.value = true
  selectedSession.value = null
  try {
    selectedSession.value = await dataSharingAPI.getSession(row.id)
  } catch (error) {
    appStore.showError('加载详情失败')
  } finally {
    detailLoading.value = false
  }
}

async function downloadSelected() {
  if (selectedCount.value === 0) return
  exporting.value = true
  try {
    const blob = await dataSharingAPI.exportSessions(buildSelectionFilters())
    dataSharingAPI.downloadBlob(blob, `data-sharing-${Date.now()}.jsonl`)
  } catch (error) {
    appStore.showError('下载失败')
  } finally {
    exporting.value = false
  }
}

async function downloadOne(row: DataShareSession) {
  try {
    const blob = await dataSharingAPI.exportSession(row.id)
    dataSharingAPI.downloadBlob(blob, `data-sharing-session-${row.id}.jsonl`)
  } catch (error) {
    appStore.showError('下载失败')
  }
}

function formatDate(value?: string | null) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function formatNumber(value?: number | null) {
  return new Intl.NumberFormat().format(value || 0)
}

function qualityLabel(value?: string) {
  if (value === 'complete') return '完整'
  if (value === 'partial') return '部分完整'
  return '无效'
}

function qualityBadgeClass(value?: string) {
  if (value === 'complete') return 'badge-success'
  if (value === 'partial') return 'badge-warning'
  return 'badge-danger'
}

function formatBytes(value?: number | null) {
  const bytes = value || 0
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let size = bytes / 1024
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit++
  }
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${units[unit]}`
}

onMounted(loadSessions)
</script>
