<template>
  <div ref="cardRef" class="card relative p-4">
    <!-- 加载遮罩，与图表卡片保持一致 -->
    <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-white/50 backdrop-blur-sm dark:bg-dark-800/50">
      <LoadingSpinner size="md" />
    </div>

    <div class="mb-4 flex items-center justify-between gap-2">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('dashboard.activityHeatmap') }}</h3>
      <!-- 色阶图例 -->
      <div class="flex shrink-0 items-center gap-1 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('dashboard.heatmapLess') }}</span>
        <span v-for="level in 5" :key="level" class="h-3 w-3 rounded-sm" :class="levelClass(level - 1)" />
        <span>{{ t('dashboard.heatmapMore') }}</span>
      </div>
    </div>

    <!-- 小屏横向滚动；格子按列弹性铺满卡片宽度 -->
    <div class="overflow-x-auto">
      <!--
        统一网格：第 1 列是星期标签，第 1 行是月份标签，其余为日期格子。
        格子显式指定 gridColumn/gridRow，保证标签与格子严格对齐。
      -->
      <div
        class="grid w-full min-w-[720px]"
        :style="{
          gridTemplateColumns: `auto repeat(${weekCount}, minmax(0, 1fr))`,
          gap: CELL_GAP,
        }"
      >
        <!-- 月份标签：本周首格月份与上一列不同才显示 -->
        <div
          v-for="m in monthItems"
          :key="`m-${m.weekIndex}`"
          class="overflow-visible whitespace-nowrap text-[10px] leading-4 text-gray-400 dark:text-gray-500"
          :style="{ gridColumn: m.weekIndex + 2, gridRow: 1 }"
        >{{ m.label }}</div>

        <!-- 星期标签：只标周一/周三/周五 -->
        <div
          v-for="w in weekdayLabels"
          :key="`w-${w.row}`"
          class="flex items-center pr-1 text-[10px] leading-none text-gray-400 dark:text-gray-500"
          :style="{ gridColumn: 1, gridRow: w.row + 2 }"
        >{{ w.label }}</div>

        <!-- 日期格子：aspect-square 让行高跟随列宽，整网格自动铺满 -->
        <div
          v-for="day in days"
          :key="day.date"
          data-testid="heatmap-cell"
          class="aspect-square w-full rounded-sm"
          :class="day.future ? 'invisible' : levelClass(day.level)"
          :style="{ gridColumn: day.weekIndex + 2, gridRow: day.dayOfWeek + 2 }"
          @mouseenter="onCellHover(day, $event)"
          @mouseleave="hoveredDay = null"
        />
      </div>
    </div>

    <!--
      悬停提示：放在滚动容器外、卡片内绝对定位，避免被 overflow-x-auto 裁剪。
      前两行格子改为下方弹出，避免超出卡片顶部。
    -->
    <div
      v-if="hoveredDay"
      data-testid="heatmap-tooltip"
      class="pointer-events-none absolute z-20 whitespace-nowrap rounded-md bg-gray-900 px-2 py-1 text-xs text-white shadow-lg dark:bg-dark-600"
      :style="tooltipStyle"
    >
      <div class="font-medium">{{ formatDayLabel(hoveredDay.date) }}</div>
      <template v-if="hoveredDay.requests > 0">
        <div>{{ t('dashboard.requests') }}: {{ formatNumber(hoveredDay.requests) }}</div>
        <div>{{ t('dashboard.tokens') }}: {{ formatTokens(hoveredDay.tokens) }}</div>
        <div>{{ t('dashboard.heatmapCost') }}: {{ formatBalanceAmount(hoveredDay.actualCost, { fractionDigits: 4 }) }}</div>
      </template>
      <div v-else>{{ t('dashboard.heatmapNoUsage') }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { usageAPI } from '@/api/usage'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import { formatDateLocalInput, formatNumberLocaleString as formatNumber, formatTokensK as formatTokens } from '@/utils/format'

const CELL_GAP = '3px'
// 格子分档色：0 为无用量，1-4 按用量分位递增
const LEVEL_CLASSES = [
  'bg-gray-100 dark:bg-dark-700',
  'bg-green-200 dark:bg-green-800',
  'bg-green-300 dark:bg-green-600',
  'bg-green-500 dark:bg-green-400',
  'bg-green-700 dark:bg-green-300',
]

interface HeatmapDay {
  date: string // YYYY-MM-DD（本地时区）
  weekIndex: number // 所在列
  dayOfWeek: number // 0-6，周日为 0
  tokens: number
  requests: number
  actualCost: number
  level: number
  future?: boolean // 补齐最后一周的未来占位格，不展示不响应悬停
}

const { t, locale } = useI18n()
const { formatBalanceAmount } = useBalanceDisplay()

const loading = ref(false)
const days = ref<HeatmapDay[]>([])
const hoveredDay = ref<HeatmapDay | null>(null)
const cardRef = ref<HTMLElement | null>(null)
// 悬停格子中心相对卡片左上角的坐标，用于 tooltip 定位
const hoverPos = ref({ left: 0, top: 0 })

const levelClass = (level: number) => LEVEL_CLASSES[level] ?? LEVEL_CLASSES[0]

// 近一年的日期序列：从今天向前 364 天，再向前对齐到周日，保证整周列
// start 从 end 克隆，保证两者时分秒一致，最后一天比较不会出现毫秒级漂移
const buildDateRange = () => {
  const end = new Date()
  const start = new Date(end)
  start.setDate(start.getDate() - 364)
  start.setDate(start.getDate() - start.getDay())
  return { start, end }
}

// 按非零用量的 25/50/75 分位划分 1-4 档，无用量为 0 档；峰值日固定为最高档
const computeLevel = (tokens: number, sortedNonZero: number[]): number => {
  if (tokens <= 0 || sortedNonZero.length === 0) return 0
  if (tokens >= sortedNonZero[sortedNonZero.length - 1]) return 4
  const percentile = (p: number) => sortedNonZero[Math.min(sortedNonZero.length - 1, Math.floor(sortedNonZero.length * p))]
  if (tokens <= percentile(0.25)) return 1
  if (tokens <= percentile(0.5)) return 2
  if (tokens <= percentile(0.75)) return 3
  return 4
}

const load = async () => {
  loading.value = true
  try {
    const { start, end } = buildDateRange()
    const res = await usageAPI.getDashboardTrend({
      start_date: formatDateLocalInput(start),
      end_date: formatDateLocalInput(end),
      granularity: 'day',
    })
    // 趋势接口只返回有用量的日期，按日期建索引后补零
    const byDate = new Map((res.trend || []).map((p) => [p.date, p]))
    const sortedNonZero = (res.trend || [])
      .map((p) => p.total_tokens)
      .filter((v) => v > 0)
      .sort((a, b) => a - b)

    const result: HeatmapDay[] = []
    const cursor = new Date(start)
    while (cursor <= end) {
      const date = formatDateLocalInput(cursor)
      const point = byDate.get(date)
      const tokens = point?.total_tokens ?? 0
      result.push({
        date,
        weekIndex: Math.floor(result.length / 7),
        dayOfWeek: cursor.getDay(),
        tokens,
        requests: point?.requests ?? 0,
        actualCost: point?.actual_cost ?? 0,
        level: computeLevel(tokens, sortedNonZero),
      })
      cursor.setDate(cursor.getDate() + 1)
    }
    // 用不可见的未来占位格补齐最后一周，保证网格按整周列对齐
    while (cursor.getDay() !== 0) {
      result.push({
        date: formatDateLocalInput(cursor),
        weekIndex: Math.floor(result.length / 7),
        dayOfWeek: cursor.getDay(),
        tokens: 0,
        requests: 0,
        actualCost: 0,
        level: 0,
        future: true,
      })
      cursor.setDate(cursor.getDate() + 1)
    }
    days.value = result
  } catch (error) {
    console.error('Failed to load usage heatmap:', error)
  } finally {
    loading.value = false
  }
}

// 网格列数 = 周数（含占位格）
const weekCount = computed(() => Math.ceil(days.value.length / 7))

// 月份标签：每周首格（周日）月份与上一列不同才显示
const monthItems = computed(() => {
  const items: { weekIndex: number; label: string }[] = []
  let prevMonth = -1
  for (const day of days.value) {
    if (day.dayOfWeek !== 0) continue
    const month = new Date(`${day.date}T00:00:00`).getMonth()
    if (month !== prevMonth) {
      items.push({ weekIndex: day.weekIndex, label: formatMonth(day.date) })
    }
    prevMonth = month
  }
  return items
})

// 星期标签：只标周一/周三/周五，用窄格式（如英文 M/W/F、中文 一/三/五）
const weekdayLabels = computed(() =>
  [1, 3, 5].map((dayOfWeek) => ({
    row: dayOfWeek,
    // 2024-01-01 是周一，依次偏移得到对应星期
    label: new Date(2024, 0, dayOfWeek).toLocaleDateString(locale.value, { weekday: 'narrow' }),
  }))
)

const formatMonth = (date: string) =>
  new Date(`${date}T00:00:00`).toLocaleDateString(locale.value, { month: 'short' })

const formatDayLabel = (date: string) =>
  new Date(`${date}T00:00:00`).toLocaleDateString(locale.value, { year: 'numeric', month: 'short', day: 'numeric' })

// 以格子中心相对卡片的位置定位 tooltip，兼容弹性格宽与横向滚动
const onCellHover = (day: HeatmapDay, event: MouseEvent) => {
  if (day.future) return
  const cell = event.currentTarget as HTMLElement
  const card = cardRef.value
  if (card) {
    const rect = cell.getBoundingClientRect()
    const cardRect = card.getBoundingClientRect()
    hoverPos.value = {
      left: rect.left - cardRect.left + rect.width / 2,
      top: rect.top - cardRect.top,
    }
  }
  hoveredDay.value = day
}

const tooltipStyle = computed(() => {
  const day = hoveredDay.value
  if (!day) return {}
  const above = day.dayOfWeek >= 2
  return {
    left: `${hoverPos.value.left}px`,
    top: above ? `${hoverPos.value.top - 6}px` : `${hoverPos.value.top + 18}px`,
    transform: above ? 'translate(-50%, -100%)' : 'translateX(-50%)',
  }
})

onMounted(() => {
  void load()
})

// 供仪表盘刷新按钮联动调用
defineExpose({ reload: load })
</script>
