<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="grid gap-4 md:grid-cols-3 xl:grid-cols-6">
        <div class="card p-4">
          <p class="text-xs text-gray-500 dark:text-gray-400">Session 总数</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatNumber(stats?.session_count) }}</p>
        </div>
        <div class="card p-4">
          <p class="text-xs text-gray-500 dark:text-gray-400">完整</p>
          <p class="mt-2 text-2xl font-semibold text-emerald-600 dark:text-emerald-400">{{ formatNumber(stats?.complete_count) }}</p>
        </div>
        <div class="card p-4">
          <p class="text-xs text-gray-500 dark:text-gray-400">部分完整</p>
          <p class="mt-2 text-2xl font-semibold text-amber-600 dark:text-amber-400">{{ formatNumber(stats?.partial_count) }}</p>
        </div>
        <div class="card p-4">
          <p class="text-xs text-gray-500 dark:text-gray-400">无效</p>
          <p class="mt-2 text-2xl font-semibold text-red-600 dark:text-red-400">{{ formatNumber(stats?.invalid_count) }}</p>
        </div>
        <div class="card p-4">
          <p class="text-xs text-gray-500 dark:text-gray-400">占用空间</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatBytes(stats?.total_storage_bytes) }}</p>
        </div>
        <div class="card p-4">
          <p class="text-xs text-gray-500 dark:text-gray-400">单 session 平均 token</p>
          <p class="mt-2 text-2xl font-semibold text-purple-600 dark:text-purple-400">
            {{ formatNumber(Math.round(stats?.avg_tokens_per_session || 0)) }}
          </p>
        </div>
      </div>

      <div class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.7fr)_minmax(320px,0.65fr)]">
        <div class="card p-4">
          <div class="mb-4 flex items-center justify-between">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">空间增长趋势</h2>
            <button class="btn btn-secondary btn-sm" :disabled="statsLoading" @click="loadStats">
              <Icon name="refresh" size="sm" :class="statsLoading ? 'animate-spin' : ''" />
            </button>
          </div>
          <div class="h-64">
            <div v-if="statsLoading" class="flex h-full items-center justify-center">
              <LoadingSpinner />
            </div>
            <Line v-else-if="storageTrendChartData" :data="storageTrendChartData" :options="lineChartOptions" />
            <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">暂无趋势数据</div>
          </div>
        </div>

        <div class="card p-4">
          <h2 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">分组空间占用</h2>
          <div class="h-64">
            <div v-if="statsLoading" class="flex h-full items-center justify-center">
              <LoadingSpinner />
            </div>
            <Bar v-else-if="groupStorageChartData" :data="groupStorageChartData" :options="barChartOptions" />
            <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">暂无分组数据</div>
          </div>
        </div>

        <div class="card p-4">
          <h2 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">请求路径分布</h2>
          <div class="h-64">
            <div v-if="statsLoading" class="flex h-full items-center justify-center">
              <LoadingSpinner />
            </div>
            <Doughnut v-else-if="requestPathChartData" :data="requestPathChartData" :options="doughnutChartOptions" />
            <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">暂无路径数据</div>
          </div>
        </div>
      </div>

      <div class="card p-4">
        <div class="mb-3 flex items-center justify-between gap-3">
          <div>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">数据共享须知</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">当前版本：{{ notice?.version || '-' }}</p>
          </div>
          <button class="btn btn-primary" :disabled="savingNotice" @click="saveNotice">
            <Icon name="check" size="md" class="mr-2" />
            保存须知
          </button>
        </div>
        <textarea
          v-model="noticeContent"
          rows="5"
          class="input"
          placeholder="请输入用户切换到数据共享分组前需要确认的须知内容"
        ></textarea>
      </div>

      <TablePageLayout>
        <template #filters>
          <div class="flex flex-col gap-4">
            <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
              <div class="flex flex-1 flex-wrap items-center gap-3">
                <div class="relative w-full sm:w-64">
                  <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                  <input v-model="filters.search" type="text" class="input pl-10" placeholder="搜索 session、轨迹或模型" @input="handleFilterChange" />
                </div>
                <input v-model="filters.user_name" type="text" class="input w-36" placeholder="用户名称" @input="handleFilterChange" />
                <input v-model="filters.api_key_name" type="text" class="input w-36" placeholder="Key 名称" @input="handleFilterChange" />
                <input v-model="filters.group_name" type="text" class="input w-36" placeholder="分组名称" @input="handleFilterChange" />
                <Select v-model="filters.request_path" :options="requestPathOptions" class="w-52" @change="handleFilterChange" />
                <Select v-model="filters.quality_status" :options="qualityOptions" class="w-40" @change="handleFilterChange" />
                <input v-model="filters.start_date" type="date" class="input w-40" @change="handleFilterChange" />
                <input v-model="filters.end_date" type="date" class="input w-40" @change="handleFilterChange" />
              </div>
              <div class="flex flex-wrap justify-end gap-3">
                <button class="btn btn-secondary" :disabled="loading" @click="refreshAll">
                  <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
                </button>
                <button class="btn btn-secondary" :disabled="selectedCount === 0" @click="batchDelete">
                  <Icon name="trash" size="md" class="mr-2" />
                  删除已选
                </button>
                <button class="btn btn-primary" :disabled="exporting || selectedCount === 0" @click="downloadSelected">
                  <Icon name="download" size="md" class="mr-2" />
                  导出已选 JSONL
                </button>
              </div>
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
                <p class="truncate text-xs text-gray-500 dark:text-gray-400">
                  {{ displayUser(row) }} / {{ displayAPIKey(row) }} / {{ displayGroup(row) }}
                </p>
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
            <template #cell-storage_bytes="{ value }">{{ formatBytes(value) }}</template>
            <template #cell-total_tokens="{ value }">{{ formatNumber(value) }}</template>
            <template #cell-created_at="{ value }">{{ formatDate(value) }}</template>
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
                <button class="btn btn-ghost btn-sm text-red-600 hover:text-red-700" @click="deleteOne(row)">
                  <Icon name="trash" size="sm" class="mr-1" />
                  删除
                </button>
              </div>
            </template>
            <template #empty>
              <EmptyState title="暂无数据共享记录" description="数据共享分组产生的对话 session 会显示在这里。" />
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
        <div class="flex flex-wrap gap-2">
          <span class="badge badge-gray">用户 {{ displayUser(selectedSession) }}</span>
          <span class="badge badge-gray">Key {{ displayAPIKey(selectedSession) }}</span>
          <span class="badge badge-gray">分组 {{ displayGroup(selectedSession) }}</span>
          <span :class="['badge', qualityBadgeClass(selectedSession.quality_status)]">
            {{ qualityLabel(selectedSession.quality_status) }}
          </span>
        </div>
        <div v-if="selectedSession.quality_errors?.length" class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
          {{ selectedSession.quality_errors.join(', ') }}
        </div>
        <pre class="max-h-[60vh] overflow-auto rounded-lg bg-gray-950 p-4 text-xs leading-relaxed text-gray-100">{{ prettySession }}</pre>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  ArcElement,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Bar, Doughnut, Line } from 'vue-chartjs'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminDataSharingAPI, type AdminDataShareSessionFilters, type DataShareStats } from '@/api/admin/dataSharing'
import { dataSharingAPI, type DataShareNotice, type DataShareSession } from '@/api/dataSharing'
import { useAppStore } from '@/stores/app'
import type { Column } from '@/components/common/types'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, BarElement, ArcElement, Tooltip, Legend, Filler)

const appStore = useAppStore()

const notice = ref<DataShareNotice | null>(null)
const noticeContent = ref('')
const stats = ref<DataShareStats | null>(null)
const sessions = ref<DataShareSession[]>([])
const selectedSession = ref<DataShareSession | null>(null)
// 选中状态支持两种模式：显式 ID 列表，以及“当前筛选条件全集 + 排除列表”。
const selectedIds = ref<Set<number>>(new Set())
const excludedIds = ref<Set<number>>(new Set())
const selectAllMatching = ref(false)

const loading = ref(false)
const statsLoading = ref(false)
const savingNotice = ref(false)
const exporting = ref(false)
const detailOpen = ref(false)
const detailLoading = ref(false)

const pagination = reactive({ page: 1, page_size: 20, total: 0, pages: 1 })
const sortState = reactive({ sort_by: 'created_at', sort_order: 'desc' as 'asc' | 'desc' })
const filters = reactive({
  search: '',
  user_name: '',
  api_key_name: '',
  group_name: '',
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

const isDarkMode = computed(() => document.documentElement.classList.contains('dark'))
const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  storage: '#2563eb',
  sessions: '#10b981',
  group: '#7c3aed'
}))

const requestPathPalette = ['#2563eb', '#10b981', '#f59e0b', '#ef4444', '#7c3aed', '#0891b2', '#db2777', '#65a30d']

const storageTrendChartData = computed(() => {
  const points = stats.value?.storage_trend || []
  if (!points.length) return null
  return {
    labels: points.map(point => point.date),
    datasets: [
      {
        label: '空间',
        data: points.map(point => point.storage_bytes),
        borderColor: chartColors.value.storage,
        backgroundColor: `${chartColors.value.storage}22`,
        fill: true,
        tension: 0.3
      },
      {
        label: 'Session',
        data: points.map(point => point.session_count),
        borderColor: chartColors.value.sessions,
        backgroundColor: `${chartColors.value.sessions}22`,
        yAxisID: 'y1',
        tension: 0.3
      }
    ]
  }
})

const groupStorageChartData = computed(() => {
  const points = stats.value?.group_storage_breakdown || []
  if (!points.length) return null
  return {
    labels: points.map(point => point.group_name || `#${point.group_id}`),
    datasets: [
      {
        label: '空间',
        data: points.map(point => point.storage_bytes),
        backgroundColor: `${chartColors.value.group}88`,
        borderColor: chartColors.value.group,
        borderWidth: 1
      }
    ]
  }
})

const requestPathChartData = computed(() => {
  const points = stats.value?.request_path_breakdown || []
  if (!points.length) return null
  return {
    labels: points.map(point => point.request_path || '(unknown)'),
    datasets: [
      {
        label: 'Session',
        data: points.map(point => point.session_count),
        backgroundColor: points.map((_, index) => requestPathPalette[index % requestPathPalette.length]),
        borderWidth: 0
      }
    ]
  }
})

const lineChartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { intersect: false, mode: 'index' as const },
  scales: {
    x: { ticks: { color: chartColors.value.text }, grid: { color: chartColors.value.grid } },
    y: {
      ticks: { color: chartColors.value.text, callback: (value: string | number) => formatBytes(Number(value)) },
      grid: { color: chartColors.value.grid }
    },
    y1: {
      position: 'right' as const,
      ticks: { color: chartColors.value.text },
      grid: { drawOnChartArea: false }
    }
  },
  plugins: {
    legend: { labels: { color: chartColors.value.text } },
    tooltip: {
      callbacks: {
        label: (ctx: any) => ctx.dataset.yAxisID === 'y1'
          ? `${ctx.dataset.label}: ${formatNumber(ctx.raw)}`
          : `${ctx.dataset.label}: ${formatBytes(ctx.raw)}`
      }
    }
  }
}))

const barChartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  indexAxis: 'y' as const,
  scales: {
    x: {
      ticks: { color: chartColors.value.text, callback: (value: string | number) => formatBytes(Number(value)) },
      grid: { color: chartColors.value.grid }
    },
    y: { ticks: { color: chartColors.value.text }, grid: { color: chartColors.value.grid } }
  },
  plugins: {
    legend: { labels: { color: chartColors.value.text } },
    tooltip: {
      callbacks: {
        label: (ctx: any) => `${ctx.dataset.label}: ${formatBytes(ctx.raw)}`
      }
    }
  }
}))

const doughnutChartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { position: 'bottom' as const, labels: { color: chartColors.value.text } },
    tooltip: {
      callbacks: {
        label: (ctx: any) => {
          const point = stats.value?.request_path_breakdown?.[ctx.dataIndex]
          if (!point) return `${ctx.label}: ${formatNumber(ctx.raw)}`
          return `${ctx.label}: ${formatNumber(point.session_count)} · ${formatBytes(point.storage_bytes)} · ${formatNumber(point.total_tokens)} tokens`
        }
      }
    }
  }
}))

const requestPathOptions = computed(() => {
  const values = new Set<string>()
  for (const point of stats.value?.request_path_breakdown || []) {
    if (point.request_path && point.request_path !== '(unknown)') values.add(point.request_path)
  }
  for (const row of sessions.value) {
    if (row.request_path) values.add(row.request_path)
  }
  return [
    { value: 'all', label: '全部路径' },
    ...Array.from(values).sort().map(value => ({ value, label: value }))
  ]
})

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

let filterTimer: number | null = null

function buildFilters(): AdminDataShareSessionFilters {
  const out: AdminDataShareSessionFilters = {
    sort_by: sortState.sort_by,
    sort_order: sortState.sort_order
  }
  if (filters.search.trim()) out.search = filters.search.trim()
  if (filters.user_name.trim()) out.user_name = filters.user_name.trim()
  if (filters.api_key_name.trim()) out.api_key_name = filters.api_key_name.trim()
  if (filters.group_name.trim()) out.group_name = filters.group_name.trim()
  if (filters.request_path !== 'all') out.request_path = filters.request_path
  if (filters.quality_status !== 'all') out.quality_status = filters.quality_status
  if (filters.start_date) out.start_date = filters.start_date
  if (filters.end_date) out.end_date = filters.end_date
  return out
}

async function loadNotice() {
  try {
    notice.value = await adminDataSharingAPI.getNotice()
    noticeContent.value = notice.value.content
  } catch (error) {
    appStore.showError('加载数据共享须知失败')
  }
}

async function saveNotice() {
  savingNotice.value = true
  try {
    notice.value = await adminDataSharingAPI.updateNotice(noticeContent.value)
    noticeContent.value = notice.value.content
    appStore.showSuccess('数据共享须知已保存')
  } catch (error) {
    appStore.showError('保存数据共享须知失败')
  } finally {
    savingNotice.value = false
  }
}

async function loadStats() {
  statsLoading.value = true
  try {
    stats.value = await adminDataSharingAPI.getStats(buildFilters())
  } catch (error) {
    appStore.showError('加载数据共享统计失败')
  } finally {
    statsLoading.value = false
  }
}

async function loadSessions() {
  loading.value = true
  try {
    const res = await adminDataSharingAPI.listSessions(pagination.page, pagination.page_size, buildFilters())
    sessions.value = res.items
    pagination.total = res.total
    pagination.pages = res.pages
  } catch (error) {
    appStore.showError('加载数据共享记录失败')
  } finally {
    loading.value = false
  }
}

function refreshAll() {
  loadSessions()
  loadStats()
}

function handleFilterChange() {
  pagination.page = 1
  clearSelection()
  if (filterTimer) window.clearTimeout(filterTimer)
  filterTimer = window.setTimeout(refreshAll, 250)
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

function buildSelectionFilters(): AdminDataShareSessionFilters {
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
    selectedSession.value = await adminDataSharingAPI.getSession(row.id)
  } catch (error) {
    appStore.showError('加载详情失败')
  } finally {
    detailLoading.value = false
  }
}

async function deleteOne(row: DataShareSession) {
  if (!window.confirm(`确定删除 session ${row.session_id} 吗？`)) return
  try {
    await adminDataSharingAPI.deleteSession(row.id)
    if (!selectAllMatching.value) {
      selectedIds.value.delete(row.id)
    }
    appStore.showSuccess('数据已删除')
    refreshAll()
  } catch (error) {
    appStore.showError('删除失败')
  }
}

async function batchDelete() {
  if (selectedCount.value === 0) return
  const count = selectedCount.value
  const ids = selectAllMatching.value ? [] : Array.from(selectedIds.value)
  const params = selectAllMatching.value ? buildSelectionFilters() : buildFilters()
  if (!window.confirm(`确定删除已选 ${formatNumber(count)} 条数据吗？`)) return
  try {
    await adminDataSharingAPI.batchDeleteSessions(ids, params)
    clearSelection()
    appStore.showSuccess('已删除选中数据')
    refreshAll()
  } catch (error) {
    appStore.showError('批量删除失败')
  }
}

async function downloadSelected() {
  if (selectedCount.value === 0) return
  exporting.value = true
  try {
    const blob = await adminDataSharingAPI.exportSessions(buildSelectionFilters())
    dataSharingAPI.downloadBlob(blob, `admin-data-sharing-${Date.now()}.jsonl`)
  } catch (error) {
    appStore.showError('导出失败')
  } finally {
    exporting.value = false
  }
}

async function downloadOne(row: DataShareSession) {
  try {
    const blob = await adminDataSharingAPI.exportSessions({ ids: String(row.id) })
    dataSharingAPI.downloadBlob(blob, `admin-data-sharing-session-${row.id}.jsonl`)
  } catch (error) {
    appStore.showError('下载失败')
  }
}

function displayUser(row: DataShareSession) {
  return row.user_name || row.user_email || `#${row.user_id}`
}

function displayAPIKey(row: DataShareSession) {
  return row.api_key_name || `#${row.api_key_id}`
}

function displayGroup(row: DataShareSession) {
  return row.group_name || `#${row.group_id}`
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

onMounted(() => {
  loadNotice()
  refreshAll()
})
</script>
