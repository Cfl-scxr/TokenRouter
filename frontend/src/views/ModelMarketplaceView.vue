<template>
  <component
    :is="isAuthenticated ? AppLayout : 'div'"
    :class="isAuthenticated ? '' : 'ba-theme-shell relative min-h-screen overflow-hidden'"
  >
    <template v-if="isAuthenticated" #page-heading-actions>
      <div class="model-marketplace-toolbar flex w-[calc(100vw-2rem)] max-w-full min-w-0 items-center gap-2 sm:w-auto">
        <div class="min-w-0 flex-1 sm:w-80 sm:flex-none lg:w-[min(24rem,32vw)]">
          <SearchInput
            v-model="search"
            :placeholder="t('marketplace.searchPlaceholder')"
            :debounce-ms="120"
          />
        </div>

        <div ref="filterPanelRef" class="relative shrink-0">
          <button
            type="button"
            class="btn btn-secondary relative h-9 w-9 p-0"
            :aria-expanded="showFilterDropdown"
            :aria-label="t('common.filter')"
            :title="t('common.filter')"
            @click="showFilterDropdown = !showFilterDropdown"
          >
            <Icon name="filter" size="sm" />
            <span v-if="activeFilterCount > 0" class="absolute -right-1 -top-1 inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-primary-100 px-1.5 text-xs font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">
              {{ activeFilterCount }}
            </span>
          </button>
          <div v-show="showFilterDropdown" class="absolute right-0 top-full z-[60] mt-2 w-[min(34rem,calc(100vw-2rem))] max-w-[calc(100vw-2rem)] rounded-xl border border-gray-200 bg-white p-4 shadow-xl dark:border-dark-600 dark:bg-dark-900" @click.stop>
            <div class="mb-3 flex items-center justify-between">
              <div class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('common.filter') }}</div>
              <button v-if="activeFilterCount > 0" type="button" class="text-xs font-medium text-primary-600 dark:text-primary-400" @click="resetFilters">
                {{ t('common.reset') }}
              </button>
            </div>
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label class="input-label">{{ t('marketplace.allBrands') }}</label>
                <Select v-model="selectedBrand" :options="brandSelectOptions" />
              </div>
              <div>
                <label class="input-label">{{ t('marketplace.allTypes') }}</label>
                <Select v-model="selectedPricingMode" :options="pricingSelectOptions" />
              </div>
              <div>
                <label class="input-label">{{ t('marketplace.allGroups') }}</label>
                <Select v-model="selectedGroupId" :options="groupSelectOptions" searchable />
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>

    <template v-if="!isAuthenticated">
      <div class="ba-theme-backdrop pointer-events-none fixed inset-0"></div>

      <header class="glass relative z-20 border-b border-primary-900/10 px-4 dark:border-dark-600/80 sm:px-6">
        <nav class="mx-auto flex h-14 max-w-7xl items-center justify-between gap-4">
          <router-link to="/home" class="flex min-w-0 items-center gap-2.5">
            <span class="h-8 w-8 shrink-0 overflow-hidden rounded-lg shadow-sm">
              <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
            </span>
            <span class="truncate text-base font-semibold text-gray-950 dark:text-white">{{ siteName }}</span>
          </router-link>

          <div class="flex items-center gap-2 sm:gap-3">
            <div class="hidden items-center gap-5 text-sm font-medium text-gray-600 dark:text-dark-300 md:flex">
              <router-link to="/models" class="transition hover:text-gray-950 dark:hover:text-white">
                {{ t('home.nav.models') }}
              </router-link>
              <a
                v-if="docUrl"
                :href="docUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="transition hover:text-gray-950 dark:hover:text-white"
              >
                {{ t('home.docs') }}
              </a>
            </div>

            <LocaleSwitcher />

            <button
              type="button"
              @click="toggleTheme"
              class="flex h-9 w-9 items-center justify-center rounded-control text-primary-900/90 transition-colors hover:bg-primary-100 hover:text-primary-900 dark:text-dark-100/80 dark:hover:bg-dark-800 dark:hover:text-white"
              :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            >
              <Icon v-if="isDark" name="sun" size="md" />
              <Icon v-else name="moon" size="md" />
            </button>

            <router-link
              to="/login"
              class="inline-flex items-center rounded-full bg-gray-950 px-4 py-2 text-xs font-semibold text-white transition hover:bg-gray-800 dark:bg-white dark:text-dark-950 dark:hover:bg-dark-200"
            >
              {{ t('home.login') }}
            </router-link>
          </div>
        </nav>
      </header>
    </template>

    <section
      :class="isAuthenticated
        ? 'space-y-4'
        : 'relative z-10 px-4 pb-12 pt-6 sm:px-6 lg:px-8'"
    >
      <div :class="isAuthenticated ? 'space-y-4' : 'relative mx-auto max-w-[1400px] space-y-5'">
        <div v-if="!isAuthenticated" class="page-heading mb-4">
          <h1 class="page-title">{{ t('marketplace.title') }}</h1>
          <p class="page-description">{{ t('marketplace.subtitle') }}</p>
        </div>
        <div v-if="!isAuthenticated" class="flex min-w-0 items-center gap-2">
          <div class="min-w-0 flex-1 sm:w-80 sm:flex-none xl:w-96">
            <SearchInput
              v-model="search"
              :placeholder="t('marketplace.searchPlaceholder')"
              :debounce-ms="120"
            />
          </div>

          <div ref="filterPanelRef" class="relative shrink-0">
            <button
              type="button"
              class="btn btn-secondary relative h-9 w-9 p-0"
              :aria-expanded="showFilterDropdown"
              :aria-label="t('common.filter')"
              :title="t('common.filter')"
              @click="showFilterDropdown = !showFilterDropdown"
            >
              <Icon name="filter" size="sm" />
              <span v-if="activeFilterCount > 0" class="absolute -right-1 -top-1 inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-primary-100 px-1.5 text-xs font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">
                {{ activeFilterCount }}
              </span>
            </button>
            <div v-show="showFilterDropdown" class="absolute right-0 top-full z-[60] mt-2 w-[min(34rem,calc(100vw-2rem))] max-w-[calc(100vw-2rem)] rounded-xl border border-gray-200 bg-white p-4 shadow-xl dark:border-dark-600 dark:bg-dark-900" @click.stop>
              <div class="mb-3 flex items-center justify-between">
                <div class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('common.filter') }}</div>
                <button v-if="activeFilterCount > 0" type="button" class="text-xs font-medium text-primary-600 dark:text-primary-400" @click="resetFilters">
                  {{ t('common.reset') }}
                </button>
              </div>
              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <label class="input-label">{{ t('marketplace.allBrands') }}</label>
                  <Select v-model="selectedBrand" :options="brandSelectOptions" />
                </div>
                <div>
                  <label class="input-label">{{ t('marketplace.allTypes') }}</label>
                  <Select v-model="selectedPricingMode" :options="pricingSelectOptions" />
                </div>
                <div>
                  <label class="input-label">{{ t('marketplace.allGroups') }}</label>
                  <Select v-model="selectedGroupId" :options="groupSelectOptions" searchable />
                </div>
              </div>
            </div>
          </div>
        </div>

        <RoutingHealthOverview
          v-if="isAdmin && routingHealth"
          :snapshot="routingHealth"
          :load-state="routingHealthLoadState"
        />

        <div v-if="loading" class="card px-6 py-14 text-center">
          <LoadingSpinner size="lg" />
          <p class="mt-4 text-sm text-gray-500 dark:text-dark-400">{{ t('common.loading') }}</p>
        </div>

        <div v-else-if="errorMessage" class="card border-red-200 p-6 dark:border-red-500/30">
          <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('common.error') }}</h2>
              <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ errorMessage }}</p>
            </div>
            <button class="btn btn-primary" type="button" @click="fetchMarketplace">
              {{ t('common.refresh') }}
            </button>
          </div>
        </div>

        <div v-else-if="!hasMarketplaceResults" class="card px-6 py-14">
          <div class="mx-auto flex h-16 w-16 items-center justify-center rounded-3xl bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-300">
            <Icon name="inbox" size="xl" />
          </div>
          <h2 class="mt-6 text-center text-2xl font-semibold text-gray-950 dark:text-white">{{ t('marketplace.emptyTitle') }}</h2>
          <p class="mx-auto mt-3 max-w-xl text-center text-sm leading-7 text-gray-600 dark:text-dark-300">
            {{ t('marketplace.emptyDescription') }}
          </p>
          <div class="mt-6 text-center">
            <button class="btn btn-secondary" type="button" @click="resetFilters">
              {{ t('common.reset') }}
            </button>
          </div>
        </div>

        <div v-else class="space-y-4">
          <section
            v-for="group in filteredGroups"
            :key="group.id"
            class="card overflow-hidden"
            data-testid="marketplace-group-section"
          >
            <div class="card-header flex flex-col gap-4 px-4 py-4 md:px-5 xl:flex-row xl:items-center xl:justify-between">
              <div class="min-w-0 flex-1 space-y-3">
                <div class="flex flex-wrap items-center gap-2">
                  <span :class="brandBadgeClass(group)">
                    <ProviderIcon :brand="groupBrandSource(group)" size="14px" />
                    {{ groupBrandLabel(group) }}
                  </span>
                  <!-- 分组头部展示相对官方价的最高优惠，无有效折扣数据时不渲染该标签；点击/悬停查看说明。 -->
                  <HelpTooltip
                    v-if="formatMaxDiscountOff(group.official_price_ratio)"
                    trigger="both"
                    width-class="w-72"
                    :closable="false"
                    :content="t('marketplace.maxDiscountHint')"
                  >
                    <template #trigger>
                      <span
                        data-testid="group-max-discount-tag"
                        class="rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1 text-xs font-semibold text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-200"
                      >
                        {{ formatMaxDiscountOff(group.official_price_ratio) }}
                      </span>
                    </template>
                  </HelpTooltip>
                  <HelpTooltip
                    trigger="both"
                    width-class="w-72"
                    :closable="false"
                    :content="t('marketplace.rateMultiplierHint')"
                  >
                    <template #trigger>
                      <span
                        data-testid="group-rate-multiplier-tag"
                        class="rounded-full border border-gray-200 bg-gray-100 px-3 py-1 text-xs font-semibold text-gray-700 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-200"
                      >
                        {{ formatRateMultiplierLabel(group.rate_multiplier) }}
                      </span>
                    </template>
                  </HelpTooltip>
                  <span
                    v-if="hasIndependentImageRate(group)"
                    class="rounded-full border border-fuchsia-200 bg-fuchsia-50 px-3 py-1 text-xs font-semibold text-fuchsia-700 dark:border-fuchsia-500/30 dark:bg-fuchsia-500/10 dark:text-fuchsia-200"
                  >
                    {{ formatImageRateMultiplierLabel(group.image_rate_multiplier) }}
                  </span>
                  <!-- 数据共享分组需要醒目标记，避免用户在模型广场忽略采集属性。 -->
                  <span
                    v-if="group.data_sharing_enabled"
                    class="rounded-full border border-blue-200 bg-blue-50 px-3 py-1 text-xs font-semibold text-blue-700 dark:border-blue-500/30 dark:bg-blue-500/10 dark:text-blue-200"
                  >
                    {{ t('marketplace.dataSharingTag') }}
                  </span>
                </div>

                <div class="flex items-start gap-3">
                  <span class="mt-0.5 flex h-12 w-12 shrink-0 items-center justify-center rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-950">
                    <ModelIcon :model="groupBrandIconModel(group)" size="28px" />
                  </span>
                  <div class="min-w-0">
                    <h2 class="text-lg font-semibold text-gray-950 dark:text-white">{{ group.name }}</h2>
                    <p v-if="group.description" class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">
                      {{ group.description }}
                    </p>
                  </div>
                </div>
              </div>
              <div
                v-if="group.availability || isAdmin"
                class="w-full xl:ml-6 xl:w-[560px] xl:shrink-0"
                data-testid="marketplace-group-availability"
              >
                <!-- 用户侧只展示可用率，并利用释放出的空间将状态条靠右放置。 -->
                <GroupAvailabilityBar
                  v-if="group.availability || isAdmin"
                  :availability="group.availability"
                  :current-status="routingHealth?.available ? routingHealth.providers.find(provider => provider.names.group === group.name)?.probeStatus : undefined"
                  :upstream-assets="isAdmin ? upstreamAssetsByGroup[group.name] : undefined"
                  class="min-w-0"
                />
                <GroupBusinessUsage v-if="isAdmin" v-bind="businessUsage[group.id] || {}" />
              </div>
            </div>

            <div class="grid items-start gap-3 p-4 md:grid-cols-2 lg:grid-cols-3 md:p-5">
              <!-- 大屏固定三列展示，避免宽屏下只排两列造成右侧留白。 -->
              <article
                v-for="model in group.models"
                :key="`${group.id}-${model.id}`"
                class="group rounded-xl border border-gray-100 bg-gray-50/80 p-4 transition hover:-translate-y-0.5 hover:border-black/20 hover:shadow-sm dark:border-dark-700 dark:bg-dark-950/80 dark:hover:border-primary-500/50"
              >
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <h3 class="truncate text-base font-semibold text-gray-950 dark:text-white">{{ model.display_name }}</h3>
                    <ModelIdLabel :model-id="model.id" class="mt-1" />
                  </div>
                  <ModelCapabilityTags :model="model" />
                </div>

                <!-- 价格预览改为无边框列表，避免卡片里再嵌套一层卡片。 -->
                <div class="mt-4">
                  <template v-if="compactPricingRows(model.pricing).length > 0">
                    <dl class="space-y-2">
                      <div
                        v-for="row in compactPricingRows(model.pricing)"
                        :key="row.key"
                        class="flex items-baseline justify-between gap-3 text-sm"
                      >
                        <dt class="shrink-0 text-gray-500 dark:text-dark-400">{{ row.label }}</dt>
                        <dd class="min-w-0 text-right font-medium tabular-nums text-gray-900 dark:text-white">{{ row.value }}</dd>
                      </div>
                    </dl>
                  </template>
                  <p v-else class="text-sm text-gray-400 dark:text-dark-500">
                    {{ t('marketplace.pricingUnavailable') }}
                  </p>

                  <!-- 完整定价改为卡片内抽屉式浮窗，展开/收起与区间、fast mode 切换收敛在组件内部。 -->
                  <ModelPricingPanel :model="model" />
                </div>
              </article>
            </div>
          </section>
        </div>
      </div>
    </section>
  </component>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import GroupAvailabilityBar from '@/components/marketplace/GroupAvailabilityBar.vue'
import GroupBusinessUsage from '@/components/marketplace/GroupBusinessUsage.vue'
import { getStats as getBusinessUsageStats, type AdminUsageStatsResponse } from '@/api/admin/usage'
import ModelCapabilityTags from '@/components/marketplace/ModelCapabilityTags.vue'
import ModelPricingPanel from '@/components/marketplace/ModelPricingPanel.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import ProviderIcon from '@/components/common/ProviderIcon.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Select from '@/components/common/Select.vue'
import ModelIdLabel from '@/components/common/ModelIdLabel.vue'
import RoutingHealthOverview from '@/components/marketplace/RoutingHealthOverview.vue'
import type { ChannelUpstreamAsset } from '@/components/marketplace/channelUpstreamAsset'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import { initTheme, useTheme } from '@/composables/useTheme'
import { getMarketplaceModels, getMarketplaceRoutingHealth } from '@/api/marketplace'
import { list as listAccounts, queryBatchUpstreamUsage } from '@/api/admin/accounts'
import { providerBrandDisplayName, providerBrandFilterKey, resolveProviderBrand, resolveProviderBrandKey } from '@/utils/providerBrand'
import { sanitizeUrl } from '@/utils/url'
import type { Account, MarketplaceGroup, MarketplaceModelPricing, MarketplacePricingInterval, MarketplaceRoutingHealthSnapshot, UpstreamUsageQueryResult } from '@/types'
import { useAppStore, useAuthStore } from '@/stores'

type VisibleMarketplaceGroup = MarketplaceGroup
type PricingFilter = 'all' | 'token' | 'image' | 'unpriced'

interface PricingRow {
  key: string
  label: string
  value: string
}

const { t } = useI18n()
const { balanceUnitName } = useBalanceDisplay()

const appStore = useAppStore()
const authStore = useAuthStore()
const { isDark, toggleTheme } = useTheme()

const groups = ref<MarketplaceGroup[]>([])
const upstreamAccounts = ref<Account[]>([])
const upstreamUsageByAccountId = ref<Record<string, UpstreamUsageQueryResult | undefined>>({})
const businessUsage = ref<Record<number, { stats?: AdminUsageStatsResponse; error?: boolean; updatedAt?: string }>>({})
let businessUsageInFlight = false
let businessUsageLastAttempt = 0
let upstreamAssetsInFlight = false
let upstreamAssetsLastAttempt = 0
let disposed = false
const routingHealth = ref<MarketplaceRoutingHealthSnapshot | null>(null)
const loading = ref(true)
const errorMessage = ref('')
const search = ref('')
const selectedBrand = ref<string | 'all'>('all')
const selectedPricingMode = ref<PricingFilter>('all')
const selectedGroupId = ref<number | 'all'>('all')
const showFilterDropdown = ref(false)
const filterPanelRef = ref<HTMLElement | null>(null)
let marketplaceRequestInFlight = false
let routingHealthRequestInFlight = false
let marketplaceRefreshTimer: ReturnType<typeof setInterval> | null = null
type RoutingHealthLoadState = 'ready' | 'source_unavailable' | 'auth_required' | 'forbidden' | 'network_error' | 'unknown_error'
const routingHealthLoadState = ref<RoutingHealthLoadState>('ready')
const UPSTREAM_ASSET_REFRESH_MS = 5 * 60 * 1000
const UPSTREAM_ASSET_BATCH_SIZE = 100
const UPSTREAM_ASSET_CACHE_PREFIX = 'tokenrouter:admin:marketplace-upstream-assets:v1:'

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))

const upstreamAssetsByGroup = computed<Record<string, ChannelUpstreamAsset[]>>(() => {
  const groupNames = new Map(groups.value.map(group => [group.id, group.name]))
  const result: Record<string, ChannelUpstreamAsset[]> = {}
  for (const account of upstreamAccounts.value) {
    for (const groupId of account.group_ids ?? []) {
      const groupName = groupNames.get(groupId)
      if (!groupName) continue
      const assets = result[groupName] ?? (result[groupName] = [])
      assets.push({
        accountId: account.id,
        accountName: account.name,
        rateMultiplier: account.rate_multiplier,
        groupRateMultiplier: groups.value.find(group => group.id === groupId)?.rate_multiplier,
        usage: upstreamUsageByAccountId.value[String(account.id)],
      })
    }
  }
  for (const assets of Object.values(result)) assets.sort((left, right) => left.accountId - right.accountId)
  return result
})

const normalizedSearch = computed(() => search.value.trim().toLowerCase())
const activeFilterCount = computed(() => [
  selectedBrand.value !== 'all' ? selectedBrand.value : '',
  selectedPricingMode.value !== 'all' ? selectedPricingMode.value : '',
  selectedGroupId.value !== 'all' ? selectedGroupId.value : '',
].filter(Boolean).length)

const sortedGroups = computed(() =>
  [...groups.value].sort((left, right) => {
    const sortDiff = (left.sort_order ?? 0) - (right.sort_order ?? 0)
    if (sortDiff !== 0) {
      return sortDiff
    }
    return left.id - right.id
  })
)

const availableBrands = computed(() => {
  const seen = new Set<string>()
  const brands: string[] = []
  for (const group of sortedGroups.value) {
    const brand = groupBrandLabel(group)
    const key = brandKey(brand)
    if (seen.has(key)) {
      continue
    }
    seen.add(key)
    brands.push(brand)
  }
  return brands
})

const brandSelectOptions = computed(() => [
  { value: 'all', label: t('marketplace.allBrands') },
  ...availableBrands.value.map((brand) => ({
    value: brand,
    label: brand,
  })),
])

const pricingSelectOptions = computed(() => [
  { value: 'all', label: t('marketplace.allTypes') },
  { value: 'token', label: t('marketplace.tokenPricing') },
  { value: 'image', label: t('marketplace.imagePricing') },
  { value: 'unpriced', label: t('marketplace.unpriced') },
])

const groupSelectOptions = computed(() => [
  { value: 'all', label: t('marketplace.allGroups') },
  ...sortedGroups.value.map((group) => ({
    value: group.id,
    label: group.name,
  })),
])

const filteredGroups = computed<VisibleMarketplaceGroup[]>(() => {
  const keyword = normalizedSearch.value

  return sortedGroups.value.flatMap((group) => {
    if (selectedBrand.value !== 'all' && brandKey(groupBrandLabel(group)) !== brandKey(selectedBrand.value)) {
      return []
    }

    if (selectedGroupId.value !== 'all' && group.id !== selectedGroupId.value) {
      return []
    }

    const groupMatchesKeyword = !keyword || [group.name, group.description, groupBrandSource(group), groupBrandLabel(group)]
      .filter(Boolean)
      .some((value) => value.toLowerCase().includes(keyword))

    const models = group.models.filter((model) => {
      if (selectedPricingMode.value !== 'all' && pricingKind(model.pricing) !== selectedPricingMode.value) {
        return false
      }

      if (!keyword || groupMatchesKeyword) {
        return true
      }

      return [model.id, model.display_name].some((value) => value.toLowerCase().includes(keyword))
    })

    if (models.length === 0) {
      return []
    }

    return [{
      ...group,
      model_count: models.length,
      models,
    }]
  })
})

const hasMarketplaceResults = computed(() => filteredGroups.value.length > 0)

function hasPositiveValue(value?: number | null): value is number {
  return typeof value === 'number' && value > 0
}

function hasContextIntervalPricing(pricing: MarketplaceModelPricing): boolean {
  return pricing.context_intervals?.some((interval) => [
    interval.input_price_per_token,
    interval.image_input_price_per_token,
    interval.output_price_per_token,
    interval.cache_write_price_per_token,
    interval.cache_write_1h_price_per_token,
    interval.cache_read_price_per_token,
    interval.image_output_price_per_token,
    interval.fast_input_price_per_token,
    interval.fast_image_input_price_per_token,
    interval.fast_output_price_per_token,
    interval.fast_cache_write_price_per_token,
    interval.fast_cache_write_1h_price_per_token,
    interval.fast_cache_read_price_per_token,
    interval.fast_image_output_price_per_token,
  ].some(hasPositiveValue)) ?? false
}

function hasImagePricing(pricing: MarketplaceModelPricing): boolean {
  return [
    pricing.image_price_1k,
    pricing.image_price_2k,
    pricing.image_price_4k,
  ].some(hasPositiveValue)
}

function pricingKind(pricing: MarketplaceModelPricing): Exclude<PricingFilter, 'all'> {
  if (pricing.price_status !== 'priced') {
    return 'unpriced'
  }
  if (pricing.pricing_mode === 'image' && hasImagePricing(pricing)) {
    return 'image'
  }
  if (pricing.pricing_mode === 'token') {
    return 'token'
  }
  return 'unpriced'
}

function resetFilters() {
  search.value = ''
  selectedBrand.value = 'all'
  selectedPricingMode.value = 'all'
  selectedGroupId.value = 'all'
  showFilterDropdown.value = false
}

function handleFilterClickOutside(event: MouseEvent) {
  const target = event.target
  if (target instanceof Node && filterPanelRef.value?.contains(target)) return
  if (target instanceof Element && target.closest('.select-dropdown-portal')) return
  showFilterDropdown.value = false
}

function formatMultiplier(multiplier: number): string {
  return `x${multiplier.toFixed(multiplier % 1 === 0 ? 0 : 2)}`
}

// 分组倍率文案交给 i18n 拼接，避免不同语言的空格规则写死在模板里。
function formatRateMultiplierLabel(multiplier: number): string {
  return t('marketplace.rateMultiplierValue', { multiplier: formatMultiplier(multiplier) })
}

function hasIndependentImageRate(group: Pick<MarketplaceGroup, 'image_rate_independent'>): boolean {
  return Boolean(group.image_rate_independent)
}

// 相对官方价的最高优惠文案（分组级），口径与首页精选卡片一致：比例缺失、非法或不低于 1（无折扣）时返回 null。
function formatMaxDiscountOff(ratio?: number): string | null {
  if (typeof ratio !== 'number' || !Number.isFinite(ratio) || ratio <= 0 || ratio >= 1) {
    return null
  }
  const percent = new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 1,
  }).format((1 - ratio) * 100)
  return t('marketplace.maxDiscountOff', { percent })
}

function formatImageRateMultiplierLabel(multiplier: number): string {
  return t('marketplace.imageRateMultiplierValue', { multiplier: formatMultiplier(multiplier) })
}

function formatPrice(value: number): string {
  return `${formatPriceNumber(value)} ${balanceUnitName.value}`
}

function formatPriceNumber(value: number): string {
  const abs = Math.abs(value)
  const maximumFractionDigits = abs >= 1 ? 2 : abs >= 0.01 ? 4 : 6
  const minimumFractionDigits = abs >= 1 ? 2 : 4

  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits,
    maximumFractionDigits,
  }).format(value)
}

function formatPerMillion(value: number): string {
  return `${formatPrice(value * 1_000_000)} ${t('usage.perMillionTokens')}`
}

function formatCompactPerMillion(value: number): string {
  return formatPriceNumber(value * 1_000_000)
}

function formatPerImage(value: number): string {
  return `${formatPrice(value)} ${t('marketplace.perImage')}`
}

function formatTokenCount(value: number): string {
  return new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 0,
  }).format(value)
}

function formatCompactNumber(value: number): string {
  return new Intl.NumberFormat(undefined, {
    maximumFractionDigits: value >= 100 ? 0 : 1,
  }).format(value)
}

function formatCompactTokenCount(value: number): string {
  if (value >= 1_000_000) {
    return `${formatCompactNumber(value / 1_000_000)}m`
  }
  if (value >= 1_000) {
    return `${formatCompactNumber(value / 1_000)}k`
  }
  return formatTokenCount(value)
}

// 最大 token 为空表示无上限，用 ∞ 和渠道配置页保持一致。
// 卡片预览空间有限，用紧凑区间避免上下文数字换行。
function formatCompactTokenRange(minTokens: number, maxTokens?: number | null): string {
  if (typeof maxTokens !== 'number') {
    return `${formatCompactTokenCount(minTokens)}+`
  }
  return `${formatCompactTokenCount(minTokens)}-${formatCompactTokenCount(maxTokens)}`
}

function groupBrandSource(group: Pick<MarketplaceGroup, 'display_brand' | 'name'>): string {
  return group.display_brand?.trim() || group.name
}

function groupBrandLabel(group: Pick<MarketplaceGroup, 'display_brand' | 'name'>): string {
  return providerBrandDisplayName(groupBrandSource(group))
}

function brandKey(label: string): string {
  return providerBrandFilterKey(label)
}

function brandBadgeClass(group: MarketplaceGroup): string {
  const base = 'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold ring-1 ring-inset'
  return `${base} ${resolveProviderBrand(groupBrandSource(group)).badgeClass}`
}

function groupBrandIconModel(group: MarketplaceGroup): string {
  const brandKey = resolveProviderBrandKey(groupBrandSource(group))

  // 大图标使用模型图标体系，避免 ProviderIcon 的品牌色和模型卡片图标不一致。
  switch (brandKey) {
    case 'anthropic':
      return 'claude'
    case 'openai':
      return 'gpt'
    case 'google':
      return 'gemini'
    case 'alibaba':
      return 'qwen'
    // xAI 是品牌名，需转换为 ModelIcon 能识别的 Grok 模型标识。
    case 'xai':
      return 'grok'
    case 'baidu':
      return 'ernie'
    case 'iflytek':
      return 'spark'
    case 'tencent':
      return 'hunyuan'
    case 'zeroone':
      return 'yi'
    case 'xiaomi':
      return 'mimo'
    default:
      return groupBrandSource(group)
  }
}

function tokenPricingRowsFromValues(pricing: MarketplaceModelPricing | MarketplacePricingInterval): PricingRow[] {
  const rows: PricingRow[] = []

  if (hasPositiveValue(pricing.input_price_per_token)) {
    rows.push({ key: 'input', label: t('marketplace.input'), value: formatPerMillion(pricing.input_price_per_token) })
  }
  if (hasPositiveValue(pricing.image_input_price_per_token)) {
    rows.push({ key: 'image_input', label: t('marketplace.imageInput'), value: formatPerMillion(pricing.image_input_price_per_token) })
  }
  if (hasPositiveValue(pricing.output_price_per_token)) {
    rows.push({ key: 'output', label: t('marketplace.output'), value: formatPerMillion(pricing.output_price_per_token) })
  }
  if (hasPositiveValue(pricing.cache_write_price_per_token)) {
    rows.push({ key: 'cache_write', label: t('marketplace.cacheWrite'), value: formatPerMillion(pricing.cache_write_price_per_token) })
  }
  if (hasPositiveValue(pricing.cache_write_1h_price_per_token)) {
    rows.push({ key: 'cache_write_1h', label: t('marketplace.cacheWrite1h'), value: formatPerMillion(pricing.cache_write_1h_price_per_token) })
  }
  if (hasPositiveValue(pricing.cache_read_price_per_token)) {
    rows.push({ key: 'cache_read', label: t('marketplace.cacheRead'), value: formatPerMillion(pricing.cache_read_price_per_token) })
  }
  if (hasPositiveValue(pricing.image_output_price_per_token)) {
    rows.push({ key: 'image_output', label: t('marketplace.imageOutput'), value: formatPerMillion(pricing.image_output_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_input_price_per_token)) {
    rows.push({ key: 'fast_input', label: t('marketplace.fastInput'), value: formatPerMillion(pricing.fast_input_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_image_input_price_per_token)) {
    rows.push({ key: 'fast_image_input', label: t('marketplace.fastImageInput'), value: formatPerMillion(pricing.fast_image_input_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_output_price_per_token)) {
    rows.push({ key: 'fast_output', label: t('marketplace.fastOutput'), value: formatPerMillion(pricing.fast_output_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_cache_write_price_per_token)) {
    rows.push({ key: 'fast_cache_write', label: t('marketplace.fastCacheWrite'), value: formatPerMillion(pricing.fast_cache_write_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_cache_write_1h_price_per_token)) {
    rows.push({ key: 'fast_cache_write_1h', label: t('marketplace.fastCacheWrite1h'), value: formatPerMillion(pricing.fast_cache_write_1h_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_cache_read_price_per_token)) {
    rows.push({ key: 'fast_cache_read', label: t('marketplace.fastCacheRead'), value: formatPerMillion(pricing.fast_cache_read_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_image_output_price_per_token)) {
    rows.push({ key: 'fast_image_output', label: t('marketplace.fastImageOutput'), value: formatPerMillion(pricing.fast_image_output_price_per_token) })
  }

  return rows
}

function compactTokenPricingRows(pricing: MarketplaceModelPricing | MarketplacePricingInterval): PricingRow[] {
  const primaryRows: PricingRow[] = []
  if (hasPositiveValue(pricing.input_price_per_token)) {
    primaryRows.push({ key: 'input', label: t('marketplace.input'), value: formatPerMillion(pricing.input_price_per_token) })
  }
  if (hasPositiveValue(pricing.output_price_per_token)) {
    primaryRows.push({ key: 'output', label: t('marketplace.output'), value: formatPerMillion(pricing.output_price_per_token) })
  }
  if (primaryRows.length > 0) {
    return primaryRows
  }

  const rows = tokenPricingRowsFromValues(pricing)
  if (rows.length === 0) {
    return zeroTokenPricingRows()
  }

  return rows
    .filter((row) => !row.key.startsWith('fast_'))
    .slice(0, 2)
}

function zeroTokenPricingRows(): PricingRow[] {
  return [
    { key: 'input', label: t('marketplace.input'), value: formatPerMillion(0) },
    { key: 'output', label: t('marketplace.output'), value: formatPerMillion(0) },
  ]
}

function compactContextIntervalRows(pricing: MarketplaceModelPricing): PricingRow[] {
  return pricing.context_intervals?.flatMap((interval, index) => {
    const rows = compactIntervalTokenPricingRows(interval)
    if (rows.length === 0) {
      return []
    }
    return [{
      key: `compact-${interval.min_tokens}-${interval.max_tokens ?? 'up'}-${index}`,
      label: formatCompactTokenRange(interval.min_tokens, interval.max_tokens),
      value: rows.map((row) => `${row.label} ${row.value}`).join(' / '),
    }]
  }) ?? []
}

function compactIntervalTokenPricingRows(pricing: MarketplacePricingInterval): PricingRow[] {
  const rows: PricingRow[] = []
  if (hasPositiveValue(pricing.input_price_per_token)) {
    rows.push({ key: 'input', label: t('marketplace.input'), value: formatCompactPerMillion(pricing.input_price_per_token) })
  }
  if (hasPositiveValue(pricing.output_price_per_token)) {
    rows.push({ key: 'output', label: t('marketplace.output'), value: formatCompactPerMillion(pricing.output_price_per_token) })
  }
  if (rows.length > 0) {
    return rows
  }

  return tokenPricingRowsFromValues(pricing)
    .filter((row) => !row.key.startsWith('fast_'))
    .slice(0, 2)
}

function compactPricingRows(pricing: MarketplaceModelPricing): PricingRow[] {
  const kind = pricingKind(pricing)
  if (kind === 'token' && hasContextIntervalPricing(pricing)) {
    return compactContextIntervalRows(pricing)
  }
  if (kind === 'token') {
    return compactTokenPricingRows(pricing)
  }
  if (kind === 'image') {
    return imagePricingRows(pricing)
  }
  return []
}

function imagePricingRows(pricing: MarketplaceModelPricing): PricingRow[] {
  const values = [
    { key: '1k', label: '1K', price: pricing.image_price_1k },
    { key: '2k', label: '2K', price: pricing.image_price_2k },
    { key: '4k', label: '4K', price: pricing.image_price_4k },
  ]

  return values.flatMap((item) => {
    if (!hasPositiveValue(item.price)) {
      return []
    }

    return [{
      key: item.key,
      label: item.label,
      value: formatPerImage(item.price),
    }]
  })
}

async function fetchMarketplace() {
  return loadMarketplace(false)
}

async function loadMarketplace(silent: boolean) {
  if (marketplaceRequestInFlight) {
    return
  }
  marketplaceRequestInFlight = true
  if (!silent) {
    loading.value = true
    errorMessage.value = ''
  }

  try {
    groups.value = await getMarketplaceModels()
    errorMessage.value = ''
  } catch (error) {
    console.error('Failed to load marketplace models:', error)
    if (!silent) {
      errorMessage.value =
        typeof error === 'object' && error !== null && 'message' in error
          ? String(error.message)
          : t('common.unknownError')
    }
  } finally {
    marketplaceRequestInFlight = false
    if (!silent) {
      loading.value = false
    }
  }
}

async function loadRoutingHealth() {
  if (!isAdmin.value || routingHealthRequestInFlight) {
    return
  }
  routingHealthRequestInFlight = true
  try {
    routingHealth.value = await getMarketplaceRoutingHealth()
    routingHealthLoadState.value = routingHealth.value.available ? 'ready' : 'source_unavailable'
  } catch (error) {
    // 综合观察面独立降级，不能阻断模型广场主体，也不重复污染浏览器控制台。
    const status = typeof error === 'object' && error !== null && 'status' in error
      ? Number(error.status)
      : null
    if (status === 401) routingHealthLoadState.value = 'auth_required'
    else if (status === 403) routingHealthLoadState.value = 'forbidden'
    else if (status === 0) routingHealthLoadState.value = 'network_error'
    else routingHealthLoadState.value = 'unknown_error'
    routingHealth.value = {
      available: false,
      schemaVersion: 1,
      state: 'unavailable',
      providers: [],
    }
  } finally {
    routingHealthRequestInFlight = false
  }
}

async function loadBusinessUsage() {
  if (disposed || !isAdmin.value || document.visibilityState === 'hidden' || businessUsageInFlight || Date.now() - businessUsageLastAttempt < 60_000 || !groups.value.length) return
  businessUsageInFlight = true
  businessUsageLastAttempt = Date.now()
  const end = new Date()
  const start = new Date(end.getTime() - 86_400_000)
  const pending = groups.value.map(group => group.id)
  // 复用业务统计接口，限制并发且不触发任何上游探测。
  try {
    await Promise.all(Array.from({ length: Math.min(3, pending.length) }, async () => {
      while (pending.length && !disposed && isAdmin.value && document.visibilityState !== 'hidden') {
        const id = pending.shift()!
        try {
          const stats = await getBusinessUsageStats({ group_id: id, start_date: start.toISOString(), end_date: end.toISOString() })
          if (!disposed && isAdmin.value) businessUsage.value[id] = { stats, updatedAt: end.toISOString() }
        } catch {
          if (!disposed && isAdmin.value) businessUsage.value[id] = { ...businessUsage.value[id], error: true }
        }
      }
    }))
  } finally {
    businessUsageInFlight = false
  }
}

function isQueryableUpstreamAccount(account: Account): boolean {
  if (account.type !== 'apikey') return false
  if (account.platform === 'zhipu' && account.credentials?.account_mode !== 'coding') return false
  const config = account.extra?.upstream_usage_query as Record<string, unknown> | undefined
  return config?.enabled !== false
}

function isMarketplaceDocumentHidden(): boolean {
  return document.visibilityState === 'hidden'
}

function upstreamAssetCacheKey(account: Account): string | null {
  const adminId = authStore.user?.id
  if (typeof adminId !== 'number' || !Number.isSafeInteger(adminId) || adminId <= 0) return null
  return `${UPSTREAM_ASSET_CACHE_PREFIX}${adminId}:${account.id}:${encodeURIComponent(account.updated_at || '')}`
}

function readUpstreamAssetCache(account: Account): UpstreamUsageQueryResult | null {
  const key = upstreamAssetCacheKey(account)
  if (!key) return null
  try {
    const raw = sessionStorage.getItem(key)
    if (!raw) return null
    const parsed = JSON.parse(raw) as { data?: UpstreamUsageQueryResult; ts?: number }
    const valid = parsed.data && parsed.data.account_id === account.id &&
      typeof parsed.data.observed_at === 'string' && Number.isFinite(Date.parse(parsed.data.observed_at)) &&
      typeof parsed.ts === 'number' && Number.isFinite(parsed.ts) &&
      parsed.ts <= Date.now() + 60_000 && Date.now() - parsed.ts < UPSTREAM_ASSET_REFRESH_MS
    if (!valid) {
      sessionStorage.removeItem(key)
      return null
    }
    return parsed.data!
  } catch {
    try { sessionStorage.removeItem(key) } catch { /* session storage may be unavailable */ }
    return null
  }
}

function writeUpstreamAssetCache(account: Account, usage: UpstreamUsageQueryResult): void {
  const key = upstreamAssetCacheKey(account)
  if (!key || usage.account_id !== account.id) return
  try {
    sessionStorage.setItem(key, JSON.stringify({ data: usage, ts: Date.now() }))
  } catch {
    // The current page can still use the in-memory result when storage is unavailable.
  }
}

async function listAllAccounts(): Promise<Account[]> {
  const accounts: Account[] = []
  let page = 1
  let pages = 1
  do {
    const response = await listAccounts(page, 500)
    accounts.push(...response.items)
    pages = Math.max(response.pages || 1, 1)
    page += 1
  } while (page <= pages)
  return accounts
}

async function loadUpstreamAssets(): Promise<void> {
  if (disposed || !isAdmin.value || isMarketplaceDocumentHidden() || upstreamAssetsInFlight ||
    Date.now() - upstreamAssetsLastAttempt < UPSTREAM_ASSET_REFRESH_MS) return

  upstreamAssetsInFlight = true
  upstreamAssetsLastAttempt = Date.now()
  try {
    const accounts = await listAllAccounts()
    if (disposed || !isAdmin.value) return
    upstreamAccounts.value = accounts

    const pending: Account[] = []
    const nextUsage = { ...upstreamUsageByAccountId.value }
    for (const account of accounts.filter(isQueryableUpstreamAccount)) {
      const cached = readUpstreamAssetCache(account)
      if (cached) nextUsage[String(account.id)] = cached
      else pending.push(account)
    }
    upstreamUsageByAccountId.value = nextUsage

    for (let index = 0; index < pending.length; index += UPSTREAM_ASSET_BATCH_SIZE) {
      if (disposed || !isAdmin.value || isMarketplaceDocumentHidden()) break
      const chunk = pending.slice(index, index + UPSTREAM_ASSET_BATCH_SIZE)
      try {
        const response = await queryBatchUpstreamUsage(chunk.map(account => account.id))
        const merged = { ...upstreamUsageByAccountId.value }
        for (const account of chunk) {
          const usage = response.usage[String(account.id)]
          if (!usage || usage.account_id !== account.id) continue
          merged[String(account.id)] = usage
          writeUpstreamAssetCache(account, usage)
        }
        if (!disposed && isAdmin.value) upstreamUsageByAccountId.value = merged
      } catch {
        // Keep the previous successful values; unsupported and failed accounts stay unobtrusive.
      }
    }
  } catch {
    // Upstream assets are supplementary and must not block the marketplace or health snapshot.
  } finally {
    upstreamAssetsInFlight = false
  }
}

function refreshMarketplaceSilently() {
  if (disposed) return
  void loadMarketplace(true).then(() => {
    void loadBusinessUsage()
    void loadUpstreamAssets()
  })
  void loadRoutingHealth()
}

function handleMarketplaceVisibilityChange() {
  if (document.visibilityState === 'visible') {
    refreshMarketplaceSilently()
  }
}

onMounted(async () => {
  document.addEventListener('click', handleFilterClickOutside)
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    await appStore.fetchPublicSettings()
  }
  await fetchMarketplace()
  void loadBusinessUsage()
  void loadUpstreamAssets()
  await loadRoutingHealth()
  if (disposed) return
  marketplaceRefreshTimer = setInterval(refreshMarketplaceSilently, 30_000)
  document.addEventListener('visibilitychange', handleMarketplaceVisibilityChange)
  window.addEventListener('focus', refreshMarketplaceSilently)
})

onUnmounted(() => {
  disposed = true
  document.removeEventListener('click', handleFilterClickOutside)
  document.removeEventListener('visibilitychange', handleMarketplaceVisibilityChange)
  window.removeEventListener('focus', refreshMarketplaceSilently)
  if (marketplaceRefreshTimer !== null) {
    clearInterval(marketplaceRefreshTimer)
    marketplaceRefreshTimer = null
  }
})
</script>
