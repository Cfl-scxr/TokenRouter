<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">数据概览</h2>
        <button class="btn btn-secondary btn-sm self-start sm:self-auto" :disabled="statsLoading || storageLimitLoading" title="刷新统计图表" @click="refreshStats">
          <Icon name="refresh" size="sm" :class="statsLoading ? 'animate-spin' : ''" />
          <span class="ml-1">刷新统计</span>
        </button>
      </div>

      <div class="grid gap-4 md:grid-cols-3 xl:grid-cols-7">
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
        <div class="card p-4">
          <div class="flex items-center justify-between gap-2">
            <p class="text-xs text-gray-500 dark:text-gray-400">采集队列</p>
            <span :class="['badge', captureWorkerBadgeClass]">{{ captureWorkerStatusText }}</span>
          </div>
          <p class="mt-2 text-2xl font-semibold text-sky-600 dark:text-sky-400">{{ captureWorkerQueueText }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            失败 {{ formatNumber(captureWorkerFailedTotal) }} · 超时 {{ formatNumber(captureWorkerTimeoutTotal) }} · 丢弃 {{ formatNumber(captureWorkerDroppedTotal) }}
          </p>
          <p v-if="stats?.capture_worker?.last_error" class="mt-1 truncate text-xs text-red-600 dark:text-red-400" :title="stats.capture_worker.last_error">
            {{ stats.capture_worker.last_error }}
          </p>
        </div>
      </div>

      <div class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.7fr)_minmax(320px,0.65fr)]">
        <div class="card p-4">
          <h2 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">空间增长趋势</h2>
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

      <div class="grid gap-6 lg:grid-cols-2 xl:grid-cols-3">
        <div class="card p-4">
          <h2 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">模型分布</h2>
          <div class="h-64">
            <div v-if="statsLoading" class="flex h-full items-center justify-center">
              <LoadingSpinner />
            </div>
            <Doughnut v-else-if="modelChartData" :data="modelChartData" :options="modelDoughnutChartOptions" />
            <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">暂无模型数据</div>
          </div>
        </div>

        <div class="card p-4">
          <h2 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">User Agent 分布</h2>
          <div class="h-64">
            <div v-if="statsLoading" class="flex h-full items-center justify-center">
              <LoadingSpinner />
            </div>
            <Doughnut v-else-if="userAgentChartData" :data="userAgentChartData" :options="userAgentDoughnutChartOptions" />
            <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">暂无 User Agent 数据</div>
          </div>
        </div>

        <div class="card p-4">
          <h2 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.dataSharing.qualityErrorDistribution') }}</h2>
          <div class="h-64">
            <div v-if="statsLoading" class="flex h-full items-center justify-center">
              <LoadingSpinner />
            </div>
            <Doughnut v-else-if="qualityErrorChartData" :data="qualityErrorChartData" :options="qualityErrorDoughnutChartOptions" />
            <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.dataSharing.noQualityErrorData') }}</div>
          </div>
        </div>
      </div>

      <div class="card p-4">
        <div class="mb-4 flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">采集空间保护</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">超过阈值后停止记录新的数据共享采集，已有记录仍可查看和导出。</p>
          </div>
          <button class="btn btn-primary btn-sm" :disabled="savingStorageLimit || storageLimitLoading" @click="saveStorageLimit">
            <Icon name="check" size="sm" class="mr-1" />
            保存阈值
          </button>
        </div>
        <div v-if="storageLimitLoading" class="flex h-24 items-center justify-center">
          <LoadingSpinner />
        </div>
        <div v-else class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(260px,0.7fr)]">
          <div>
            <div class="mb-2 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
              <span>当前占用 {{ formatBytes(storageLimit?.current_storage_bytes) }}</span>
              <span>{{ storageLimitStatusText }}</span>
            </div>
            <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-gray-800">
              <div
                class="h-full rounded-full transition-all"
                :class="storageLimit?.exceeded ? 'bg-red-500' : 'bg-primary-500'"
                :style="{ width: `${storageLimitProgress}%` }"
              ></div>
            </div>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
              阈值按压缩后的 session payload 统计；设置为 0 表示不限制。
            </p>
          </div>
          <div class="grid gap-3 sm:grid-cols-[minmax(0,1fr)_120px]">
            <div>
              <label class="input-label">空间阈值</label>
              <input v-model="storageLimitInput" type="number" min="0" step="0.01" class="input" placeholder="0" />
            </div>
            <div>
              <label class="input-label">单位</label>
              <select v-model="storageLimitUnit" class="input">
                <option value="MB">MB</option>
                <option value="GB">GB</option>
                <option value="TB">TB</option>
              </select>
            </div>
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

      <div class="card p-4">
        <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <button
              type="button"
              class="inline-flex items-center gap-2 text-left text-sm font-semibold text-gray-900 dark:text-white"
              :aria-expanded="skipRulesExpanded"
              aria-controls="data-sharing-skip-rules"
              @click="toggleSkipRulesExpanded"
            >
              <span>采集跳过规则</span>
              <span class="badge badge-gray">{{ skipRulesSummary }}</span>
            </button>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">命中启用规则的辅助请求不会进入数据共享 session。</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button class="btn btn-secondary btn-sm" @click="toggleSkipRulesExpanded">
              <Icon :name="skipRulesExpanded ? 'chevronUp' : 'chevronDown'" size="sm" class="mr-1" />
              {{ skipRulesExpanded ? '收起' : '展开' }}
            </button>
            <button class="btn btn-secondary btn-sm" :disabled="skipRulesLoading || savingSkipRules" @click="restoreDefaultSkipRules">
              恢复默认规则
            </button>
            <button class="btn btn-secondary btn-sm" :disabled="skipRulesLoading || savingSkipRules" @click="addSkipRule">
              <Icon name="plus" size="sm" class="mr-1" />
              新增规则
            </button>
            <button class="btn btn-primary btn-sm" :disabled="skipRulesLoading || savingSkipRules" @click="saveSkipRules">
              <Icon name="check" size="sm" class="mr-1" />
              保存规则
            </button>
          </div>
        </div>

        <div v-if="skipRulesExpanded" id="data-sharing-skip-rules" class="mt-4">
          <div v-if="skipRulesLoading" class="flex h-32 items-center justify-center">
            <LoadingSpinner />
          </div>
          <div v-else-if="skipRules.length === 0" class="rounded-lg border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400">
            暂无跳过规则
          </div>
          <div v-else class="space-y-3">
            <div
              v-for="(rule, index) in skipRules"
              :key="rule.id || index"
              class="rounded-lg border border-gray-200 p-4 dark:border-gray-700"
            >
              <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                <label class="inline-flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-300">
                  <input v-model="rule.enabled" type="checkbox" class="rounded border-gray-300 text-primary-600" />
                  启用规则
                </label>
                <button class="btn btn-ghost btn-sm text-red-600 hover:text-red-700" @click="removeSkipRule(index)">
                  <Icon name="trash" size="sm" class="mr-1" />
                  删除
                </button>
              </div>

              <div class="mt-3 grid gap-3 md:grid-cols-2 xl:grid-cols-5">
                <div>
                  <label class="input-label">规则 ID</label>
                  <input v-model="rule.id" type="text" class="input font-mono text-sm" placeholder="custom_rule" />
                </div>
                <div>
                  <label class="input-label">规则名称</label>
                  <input v-model="rule.name" type="text" class="input" placeholder="辅助请求跳过规则" />
                </div>
                <div>
                  <label class="input-label">客户端</label>
                  <input
                    :value="joinList(rule.client_families)"
                    type="text"
                    class="input"
                    placeholder="opencode, claude-cli"
                    @input="setSkipRuleList(rule, 'client_families', eventValue($event))"
                  />
                </div>
                <div>
                  <label class="input-label">模型</label>
                  <input
                    :value="joinList(rule.models)"
                    type="text"
                    class="input font-mono text-sm"
                    placeholder="gpt-5.4-mini, codex-auto-review"
                    @input="setSkipRuleList(rule, 'models', eventValue($event))"
                  />
                </div>
                <div>
                  <label class="input-label">请求路径</label>
                  <div class="relative" :ref="el => setSkipRulePathMenuRef(rule.id || String(index), el)">
                    <button type="button" class="input flex items-center justify-between gap-2 text-left" @click="toggleSkipRulePathMenu(rule.id || String(index))">
                      <span class="truncate">{{ formatSkipRulePaths(rule.request_paths) }}</span>
                      <Icon name="chevronDown" size="sm" class="text-gray-400" />
                    </button>
                    <div
                      v-if="openSkipRulePathMenu === (rule.id || String(index))"
                      class="absolute z-30 mt-1 w-full rounded-lg border border-gray-200 bg-white p-2 shadow-lg dark:border-gray-700 dark:bg-gray-900"
                    >
                      <label
                        v-for="option in skipRuleRequestPathOptions"
                        :key="option.value"
                        class="flex cursor-pointer items-center gap-2 rounded px-2 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-800"
                      >
                        <input
                          :checked="rule.request_paths.includes(option.value)"
                          type="checkbox"
                          class="rounded border-gray-300 text-primary-600"
                          @change="toggleSkipRulePath(rule, option.value, $event)"
                        />
                        <span>{{ option.label }}</span>
                      </label>
                    </div>
                  </div>
                </div>
              </div>

              <div class="mt-3 grid gap-3 lg:grid-cols-[minmax(0,1fr)_180px_180px]">
                <div>
                  <label class="input-label">匹配字段</label>
                  <div class="flex flex-wrap gap-2">
                    <label
                      v-for="scope in skipRuleScopeOptions"
                      :key="scope.value"
                      class="inline-flex items-center gap-2 rounded border border-gray-200 px-3 py-2 text-sm text-gray-700 dark:border-gray-700 dark:text-gray-300"
                    >
                      <input
                        :checked="rule.field_scopes.includes(scope.value)"
                        type="checkbox"
                        class="rounded border-gray-300 text-primary-600"
                        @change="toggleSkipRuleScope(rule, scope.value, $event)"
                      />
                      {{ scope.label }}
                    </label>
                  </div>
                </div>
                <div>
                  <label class="input-label">匹配方式</label>
                  <select v-model="rule.match_mode" class="input">
                    <option value="contains">包含</option>
                    <option value="equals">等于</option>
                  </select>
                </div>
                <div>
                  <label class="input-label">大小写</label>
                  <label class="flex h-10 items-center gap-2 rounded border border-gray-200 px-3 text-sm text-gray-700 dark:border-gray-700 dark:text-gray-300">
                    <input v-model="rule.case_sensitive" type="checkbox" class="rounded border-gray-300 text-primary-600" />
                    区分大小写
                  </label>
                </div>
              </div>

              <div class="mt-3">
                <label class="input-label">关键词（一行一条）</label>
                <textarea
                  :value="joinLines(rule.patterns)"
                  rows="3"
                  class="input font-mono text-sm"
                  placeholder="Generate a title for this conversation:"
                  @input="setSkipRulePatterns(rule, eventValue($event))"
                ></textarea>
              </div>
            </div>
          </div>
        </div>
      </div>

      <TablePageLayout>
        <template #filters>
          <div class="flex flex-col gap-4">
            <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
              <div class="flex flex-1 flex-wrap items-center gap-3">
                <div class="relative w-full sm:w-64">
                  <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                  <input v-model="filters.search" type="text" class="input pl-10" placeholder="搜索 session、轨迹、模型或 UA" @input="handleFilterChange" />
                </div>
                <input v-model="filters.user_name" type="text" class="input w-36" placeholder="用户名称" @input="handleFilterChange" />
                <input v-model="filters.api_key_name" type="text" class="input w-36" placeholder="Key 名称" @input="handleFilterChange" />
                <input v-model="filters.group_name" type="text" class="input w-36" placeholder="分组名称" @input="handleFilterChange" />
                <Select v-model="filters.model" :options="modelOptions" class="w-56" searchable @change="handleFilterChange" />
                <Select v-model="filters.request_path" :options="requestPathOptions" class="w-52" @change="handleFilterChange" />
                <Select v-model="filters.user_agent" :options="userAgentOptions" class="w-56" searchable @change="handleFilterChange" />
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
              <div class="flex min-w-[3.5rem] flex-col items-start gap-1 normal-case tracking-normal">
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
            <template #cell-user_agent="{ value }">
              <span v-if="value" class="block max-w-[260px] truncate text-sm text-gray-600 dark:text-gray-400" :title="value">{{ formatUserAgent(value) }}</span>
              <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
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
          <span :class="['badge', qualityBadgeClass(selectedSession.quality_status)]">
            {{ qualityLabel(selectedSession.quality_status) }}
          </span>
          <span v-if="!selectedSession.is_final_snapshot" class="badge badge-warning">非最终快照</span>
        </div>
        <div class="grid gap-3 md:grid-cols-3 xl:grid-cols-6">
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">用户</p>
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white" :title="displayUser(selectedSession)">{{ displayUser(selectedSession) }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">Key</p>
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white" :title="displayAPIKey(selectedSession)">{{ displayAPIKey(selectedSession) }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">分组</p>
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white" :title="displayGroup(selectedSession)">{{ displayGroup(selectedSession) }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">Session</p>
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white" :title="selectedSession.session_id">{{ selectedSession.session_id }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">模型</p>
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white" :title="selectedSession.model">{{ selectedSession.model || '-' }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">请求路径</p>
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white" :title="selectedSession.request_path">{{ selectedSession.request_path || '-' }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <p class="text-xs text-gray-500">User Agent</p>
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white" :title="selectedSession.user_agent">{{ formatUserAgent(selectedSession.user_agent) }}</p>
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
        <div
          v-if="selectedSession.quality_errors?.length"
          class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
        >
          <p class="mb-2 text-xs font-semibold uppercase tracking-wide text-amber-700 dark:text-amber-300">错误类型</p>
          <div class="flex flex-wrap gap-2">
            <span
              v-for="code in selectedSession.quality_errors"
              :key="code"
              class="rounded-md bg-amber-100 px-2 py-1 text-xs font-medium text-amber-900 dark:bg-amber-950/50 dark:text-amber-100"
            >
              {{ qualityErrorLabel(code) }}
            </span>
          </div>
        </div>
        <pre class="max-h-[60vh] overflow-auto rounded-lg bg-gray-950 p-4 text-xs leading-relaxed text-gray-100">{{ prettySession }}</pre>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, type ComponentPublicInstance } from 'vue'
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
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  adminDataSharingAPI,
  type AdminDataShareSessionFilters,
  type DataShareCaptureSkipRule,
  type DataShareCaptureSkipRuleFieldScope,
  type DataShareStorageLimit,
  type DataShareStats
} from '@/api/admin/dataSharing'
import { dataSharingAPI, type DataShareNotice, type DataShareSession, type DataShareSessionFilterOptions } from '@/api/dataSharing'
import { useAppStore } from '@/stores/app'
import type { Column } from '@/components/common/types'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, BarElement, ArcElement, Tooltip, Legend, Filler)

const appStore = useAppStore()
const { t, te } = useI18n()

const notice = ref<DataShareNotice | null>(null)
const noticeContent = ref('')
const skipRules = ref<DataShareCaptureSkipRule[]>([])
const storageLimit = ref<DataShareStorageLimit | null>(null)
const storageLimitInput = ref('')
const storageLimitUnit = ref<'MB' | 'GB' | 'TB'>('GB')
const stats = ref<DataShareStats | null>(null)
const sessions = ref<DataShareSession[]>([])
const selectedSession = ref<DataShareSession | null>(null)
const filterOptions = ref<DataShareSessionFilterOptions>({ models: [], request_paths: [], user_agents: [] })
const openSkipRulePathMenu = ref<string | null>(null)
const skipRulesExpanded = ref(false)
const skipRulePathMenuRefs = new Map<string, HTMLElement>()
// 选中状态支持两种模式：显式 ID 列表，以及“当前筛选条件全集 + 排除列表”。
const selectedIds = ref<Set<number>>(new Set())
const excludedIds = ref<Set<number>>(new Set())
const selectAllMatching = ref(false)

const loading = ref(false)
const statsLoading = ref(false)
const savingNotice = ref(false)
const skipRulesLoading = ref(false)
const savingSkipRules = ref(false)
const storageLimitLoading = ref(false)
const savingStorageLimit = ref(false)
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
  user_agent: 'all',
  model: 'all',
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

const skipRuleScopeOptions: Array<{ value: DataShareCaptureSkipRuleFieldScope; label: string }> = [
  { value: 'system', label: 'System' },
  { value: 'messages', label: 'Messages' },
  { value: 'input', label: 'Input' },
  { value: 'instructions', label: 'Instructions' }
]

const skipRuleRequestPathOptions = [
  { value: '/v1/messages', label: '/v1/messages' },
  { value: '/v1/chat/completions', label: '/v1/chat/completions' },
  { value: '/v1/responses', label: '/v1/responses' }
]

const defaultSkipRules: DataShareCaptureSkipRule[] = [
  {
    id: 'claude_code_title',
    name: 'Claude Code 标题生成',
    enabled: true,
    client_families: ['claude-cli'],
    request_paths: ['/v1/messages'],
    models: [],
    field_scopes: ['system'],
    patterns: ['Generate a concise, sentence-case title'],
    case_sensitive: false,
    match_mode: 'contains'
  },
  {
    id: 'opencode_title_system',
    name: 'opencode 标题生成系统提示',
    enabled: true,
    client_families: ['opencode'],
    request_paths: ['/v1/messages', '/v1/chat/completions', '/v1/responses'],
    models: [],
    field_scopes: ['system'],
    patterns: [
      'You are a title generator. You output ONLY a thread title. Nothing else.',
      'Generate a brief title that would help the user find this conversation later.',
      'NEVER respond to questions, just generate a title for the conversation'
    ],
    case_sensitive: false,
    match_mode: 'contains'
  },
  {
    id: 'opencode_title_user_prompt',
    name: 'opencode 标题生成用户提示',
    enabled: true,
    client_families: ['opencode'],
    request_paths: ['/v1/messages', '/v1/chat/completions', '/v1/responses'],
    models: [],
    field_scopes: ['messages', 'input'],
    patterns: ['Generate a title for this conversation:'],
    case_sensitive: false,
    match_mode: 'contains'
  },
  {
    id: 'agent_title_from_messages',
    name: 'Agent 会话标题生成',
    enabled: true,
    client_families: [],
    request_paths: ['/v1/messages', '/v1/chat/completions', '/v1/responses'],
    models: [],
    field_scopes: ['messages', 'input'],
    patterns: ['Please write a 5-10 word title for the following conversation:'],
    case_sensitive: false,
    match_mode: 'contains'
  },
  {
    id: 'agent_topic_title',
    name: 'Agent 主题标题提取',
    enabled: true,
    client_families: [],
    request_paths: ['/v1/messages', '/v1/chat/completions', '/v1/responses'],
    models: [],
    field_scopes: ['system', 'instructions'],
    patterns: ['extract a 2-3 word title'],
    case_sensitive: false,
    match_mode: 'contains'
  },
  {
    id: 'agent_warmup',
    name: 'Agent 预热请求',
    enabled: true,
    client_families: [],
    request_paths: ['/v1/messages', '/v1/chat/completions', '/v1/responses'],
    models: [],
    field_scopes: ['messages', 'input'],
    patterns: ['Warmup'],
    case_sensitive: false,
    match_mode: 'equals'
  },
  {
    id: 'excluded_models',
    name: '默认排除模型',
    enabled: true,
    client_families: [],
    request_paths: [],
    models: ['gpt-5.4-mini', 'codex-auto-review'],
    field_scopes: [],
    patterns: [],
    case_sensitive: false,
    match_mode: 'equals'
  }
]

const columns: Column[] = [
  { key: 'select', label: '' },
  { key: 'session_id', label: 'Session', sortable: true },
  { key: 'provider', label: 'Provider', sortable: true },
  { key: 'request_path', label: '请求路径', sortable: true },
  { key: 'model', label: '模型', sortable: true },
  { key: 'user_agent', label: 'User Agent', sortable: true },
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

const doughnutPalette = ['#2563eb', '#10b981', '#f59e0b', '#ef4444', '#7c3aed', '#0891b2', '#db2777', '#65a30d']

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
        backgroundColor: points.map((_, index) => doughnutPalette[index % doughnutPalette.length]),
        borderWidth: 0
      }
    ]
  }
})

const modelChartData = computed(() => buildBreakdownChartData(stats.value?.model_breakdown || [], point => point.model || '(unknown)'))

const userAgentChartData = computed(() => buildBreakdownChartData(
  stats.value?.user_agent_breakdown || [],
  point => formatUserAgent(point.user_agent || '(unknown)')
))

const qualityErrorChartData = computed(() => buildBreakdownChartData(
  stats.value?.quality_error_breakdown || [],
  point => qualityErrorLabel(point.error_code)
))

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

const modelDoughnutChartOptions = computed(() => buildDoughnutChartOptions(stats.value?.model_breakdown || []))
const userAgentDoughnutChartOptions = computed(() => buildDoughnutChartOptions(stats.value?.user_agent_breakdown || []))
const qualityErrorDoughnutChartOptions = computed(() => buildSessionCountDoughnutChartOptions(stats.value?.quality_error_breakdown || []))
const captureWorkerQueueText = computed(() => {
  const worker = stats.value?.capture_worker
  if (!worker) return '-'
  return `${formatNumber(worker.queue_depth)}/${formatNumber(worker.queue_capacity)}`
})
const captureWorkerFailedTotal = computed(() => stats.value?.capture_worker?.failed_total || 0)
const captureWorkerTimeoutTotal = computed(() => stats.value?.capture_worker?.timeout_total || 0)
const captureWorkerDroppedTotal = computed(() => stats.value?.capture_worker?.dropped_total || 0)
const captureWorkerStatusText = computed(() => {
  const worker = stats.value?.capture_worker
  if (!worker) return '未启用'
  if (worker.dropped_total > 0) return '有丢弃'
  if (worker.timeout_total > 0) return '有超时'
  if (worker.failed_total > 0) return '有失败'
  return '正常'
})
const captureWorkerBadgeClass = computed(() => {
  const worker = stats.value?.capture_worker
  if (!worker) return 'badge-gray'
  if (worker.dropped_total > 0 || worker.timeout_total > 0) return 'badge-danger'
  if (worker.failed_total > 0) return 'badge-warning'
  return 'badge-success'
})
const skipRulesSummary = computed(() => {
  const total = skipRules.value.length
  const enabled = skipRules.value.filter(rule => rule.enabled).length
  return `${enabled}/${total} 启用`
})

const storageLimitProgress = computed(() => {
  if (!storageLimit.value?.enabled) return 0
  return Math.min(Math.max(storageLimit.value.usage_ratio * 100, 0), 100)
})

const storageLimitStatusText = computed(() => {
  if (!storageLimit.value?.enabled) return '未设置阈值'
  const limit = formatBytes(storageLimit.value.limit_bytes)
  if (storageLimit.value.exceeded) return `已超过 ${limit}`
  return `阈值 ${limit} · ${storageLimitProgress.value.toFixed(1)}%`
})

const requestPathOptions = computed(() => {
  return [
    { value: 'all', label: '全部路径' },
    ...filterOptions.value.request_paths.map(value => ({ value, label: value }))
  ]
})

const modelOptions = computed(() => {
  return [
    { value: 'all', label: '全部模型' },
    ...filterOptions.value.models.map(value => ({ value, label: value }))
  ]
})

const userAgentOptions = computed(() => {
  return [
    { value: 'all', label: '全部 User Agent' },
    ...filterOptions.value.user_agents.map(value => ({ value, label: formatUserAgent(value) }))
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
    return `已选择当前筛选条件下 ${formatNumber(selectedCount.value)} 条数据`
  }
  return `已选择 ${formatNumber(selectedCount.value)} 条数据`
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
  if (filters.model !== 'all') out.model = filters.model
  if (filters.request_path !== 'all') out.request_path = filters.request_path
  if (filters.user_agent !== 'all') out.user_agent = filters.user_agent
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

async function loadSkipRules() {
  skipRulesLoading.value = true
  try {
    skipRules.value = cloneSkipRules(await adminDataSharingAPI.getSkipRules())
  } catch (error) {
    appStore.showError('加载采集跳过规则失败')
  } finally {
    skipRulesLoading.value = false
  }
}

async function loadStorageLimit() {
  storageLimitLoading.value = true
  try {
    storageLimit.value = await adminDataSharingAPI.getStorageLimit()
    applyStorageLimitToForm(storageLimit.value.limit_bytes)
  } catch (error) {
    appStore.showError('加载采集空间阈值失败')
  } finally {
    storageLimitLoading.value = false
  }
}

async function saveStorageLimit() {
  savingStorageLimit.value = true
  try {
    storageLimit.value = await adminDataSharingAPI.updateStorageLimit(storageLimitBytesFromForm())
    applyStorageLimitToForm(storageLimit.value.limit_bytes)
    await loadStats()
    appStore.showSuccess('采集空间阈值已保存')
  } catch (error) {
    appStore.showError('保存采集空间阈值失败')
  } finally {
    savingStorageLimit.value = false
  }
}

function applyStorageLimitToForm(limitBytes: number) {
  if (!limitBytes || limitBytes <= 0) {
    storageLimitInput.value = ''
    storageLimitUnit.value = 'GB'
    return
  }
  const tb = 1024 ** 4
  const gb = 1024 ** 3
  const mb = 1024 ** 2
  if (limitBytes % tb === 0) {
    storageLimitUnit.value = 'TB'
    storageLimitInput.value = String(limitBytes / tb)
  } else if (limitBytes % gb === 0) {
    storageLimitUnit.value = 'GB'
    storageLimitInput.value = String(limitBytes / gb)
  } else {
    storageLimitUnit.value = 'MB'
    storageLimitInput.value = trimDecimal(limitBytes / mb)
  }
}

function storageLimitBytesFromForm() {
  const raw = Number(storageLimitInput.value)
  if (!Number.isFinite(raw) || raw <= 0) return 0
  const multiplier = storageLimitUnit.value === 'TB'
    ? 1024 ** 4
    : storageLimitUnit.value === 'GB'
      ? 1024 ** 3
      : 1024 ** 2
  return Math.round(raw * multiplier)
}

function trimDecimal(value: number) {
  return value.toFixed(2).replace(/\.?0+$/, '')
}

async function saveSkipRules() {
  savingSkipRules.value = true
  try {
    skipRules.value = cloneSkipRules(await adminDataSharingAPI.updateSkipRules(normalizeSkipRulesForSave()))
    appStore.showSuccess('采集跳过规则已保存')
  } catch (error) {
    appStore.showError('保存采集跳过规则失败')
  } finally {
    savingSkipRules.value = false
  }
}

function toggleSkipRulesExpanded() {
  skipRulesExpanded.value = !skipRulesExpanded.value
  if (!skipRulesExpanded.value) {
    openSkipRulePathMenu.value = null
  }
}

function restoreDefaultSkipRules() {
  skipRulesExpanded.value = true
  skipRules.value = cloneSkipRules(defaultSkipRules)
}

function addSkipRule() {
  skipRulesExpanded.value = true
  skipRules.value = [
    ...skipRules.value,
    {
      id: `custom_${Date.now()}`,
      name: '自定义跳过规则',
      enabled: true,
      client_families: [],
      request_paths: [],
      models: [],
      field_scopes: ['messages'],
      patterns: [],
      case_sensitive: false,
      match_mode: 'contains'
    }
  ]
}

function removeSkipRule(index: number) {
  skipRules.value = skipRules.value.filter((_, i) => i !== index)
}

function toggleSkipRuleScope(rule: DataShareCaptureSkipRule, scope: DataShareCaptureSkipRuleFieldScope, event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  const values = new Set(rule.field_scopes)
  if (checked) {
    values.add(scope)
  } else {
    values.delete(scope)
  }
  rule.field_scopes = Array.from(values)
}

function toggleSkipRulePathMenu(key: string) {
  openSkipRulePathMenu.value = openSkipRulePathMenu.value === key ? null : key
}

function setSkipRulePathMenuRef(key: string, el: Element | ComponentPublicInstance | null) {
  if (el instanceof HTMLElement) {
    skipRulePathMenuRefs.set(key, el)
  } else {
    skipRulePathMenuRefs.delete(key)
  }
}

function closeSkipRulePathMenuOnOutsideClick(event: MouseEvent) {
  const key = openSkipRulePathMenu.value
  if (!key) return
  const container = skipRulePathMenuRefs.get(key)
  if (container && event.target instanceof Node && container.contains(event.target)) {
    return
  }
  openSkipRulePathMenu.value = null
}

function toggleSkipRulePath(rule: DataShareCaptureSkipRule, path: string, event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  const values = new Set(rule.request_paths)
  if (checked) {
    values.add(path)
  } else {
    values.delete(path)
  }
  rule.request_paths = Array.from(values)
}

function formatSkipRulePaths(paths: string[]) {
  if (!paths.length) return '不限路径'
  return paths.join(', ')
}

function eventValue(event: Event) {
  return (event.target as HTMLInputElement | HTMLTextAreaElement).value
}

function joinList(values: string[]) {
  return values.join(', ')
}

function joinLines(values: string[]) {
  return values.join('\n')
}

function splitList(value: string) {
  return value.split(',').map(item => item.trim()).filter(Boolean)
}

function splitLines(value: string) {
  return value.split(/\r?\n/).map(item => item.trim()).filter(Boolean)
}

function setSkipRuleList(rule: DataShareCaptureSkipRule, key: 'client_families' | 'request_paths' | 'models', value: string) {
  rule[key] = splitList(value)
}

function setSkipRulePatterns(rule: DataShareCaptureSkipRule, value: string) {
  rule.patterns = splitLines(value)
}

function cloneSkipRules(rules: DataShareCaptureSkipRule[]) {
  return rules.map(rule => ({
    ...rule,
    client_families: [...(rule.client_families || [])],
    request_paths: [...(rule.request_paths || [])],
    models: [...(rule.models || [])],
    field_scopes: [...(rule.field_scopes || [])],
    patterns: [...(rule.patterns || [])]
  }))
}

function normalizeSkipRulesForSave() {
  return cloneSkipRules(skipRules.value).map(rule => ({
    ...rule,
    id: rule.id.trim(),
    name: rule.name.trim(),
    client_families: rule.client_families.map(item => item.trim()).filter(Boolean),
    request_paths: rule.request_paths.map(item => item.trim()).filter(Boolean),
    models: rule.models.map(item => item.trim()).filter(Boolean),
    field_scopes: rule.field_scopes.filter(scope => skipRuleScopeOptions.some(option => option.value === scope)),
    patterns: rule.patterns.map(item => item.trim()).filter(Boolean)
  }))
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

function refreshStats() {
  loadStats()
  loadStorageLimit()
}

async function loadFilterOptions() {
  try {
    filterOptions.value = await adminDataSharingAPI.getFilterOptions()
  } catch (error) {
    appStore.showError('加载数据共享筛选项失败')
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
  loadStorageLimit()
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
  selectedSession.value = row
  try {
    selectedSession.value = { ...row, ...(await adminDataSharingAPI.getSession(row.id)) }
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
    const ticket = await adminDataSharingAPI.createExportTicket(buildSelectionFilters())
    dataSharingAPI.startTicketDownload(ticket)
    appStore.showSuccess('下载已开始')
  } catch (error) {
    appStore.showError('导出失败')
  } finally {
    exporting.value = false
  }
}

async function downloadOne(row: DataShareSession) {
  try {
    const ticket = await adminDataSharingAPI.createSessionExportTicket(row.id)
    dataSharingAPI.startTicketDownload(ticket)
    appStore.showSuccess('下载已开始')
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

function formatUserAgent(value?: string | null) {
  const userAgent = (value || '').trim()
  if (!userAgent || userAgent === '(unknown)') return userAgent || '-'
  return userAgent.length > 56 ? `${userAgent.slice(0, 56)}...` : userAgent
}

function buildBreakdownChartData<T extends { session_count: number }>(points: T[], labelOf: (point: T) => string) {
  if (!points.length) return null
  return {
    labels: points.map(labelOf),
    datasets: [
      {
        label: 'Session',
        data: points.map(point => point.session_count),
        backgroundColor: points.map((_, index) => doughnutPalette[index % doughnutPalette.length]),
        borderWidth: 0
      }
    ]
  }
}

function buildDoughnutChartOptions(points: Array<{ storage_bytes: number; session_count: number; total_tokens: number }>) {
  return {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: { position: 'bottom' as const, labels: { color: chartColors.value.text } },
      tooltip: {
        callbacks: {
          label: (ctx: any) => {
            const point = points[ctx.dataIndex]
            if (!point) return `${ctx.label}: ${formatNumber(ctx.raw)}`
            return `${ctx.label}: ${formatNumber(point.session_count)} · ${formatBytes(point.storage_bytes)} · ${formatNumber(point.total_tokens)} tokens`
          }
        }
      }
    }
  }
}

function buildSessionCountDoughnutChartOptions(points: Array<{ session_count: number }>) {
  return {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: { position: 'bottom' as const, labels: { color: chartColors.value.text } },
      tooltip: {
        callbacks: {
          label: (ctx: any) => {
            const point = points[ctx.dataIndex]
            const count = point?.session_count ?? Number(ctx.raw || 0)
            return `${ctx.label}: ${formatNumber(count)} ${t('admin.dataSharing.sessions')}`
          }
        }
      }
    }
  }
}

function qualityErrorLabel(code?: string | null) {
  const raw = (code || '').trim()
  const normalized = !raw || raw === '(unknown)' ? 'unknown' : raw
  const key = `admin.dataSharing.qualityErrors.${normalized}`
  return te(key) ? t(key) : normalized
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
  document.addEventListener('click', closeSkipRulePathMenuOnOutsideClick)
  loadFilterOptions()
  loadNotice()
  loadSkipRules()
  refreshAll()
})

onUnmounted(() => {
  document.removeEventListener('click', closeSkipRulePathMenuOnOutsideClick)
})
</script>
