<template>
  <div class="ba-theme-shell min-h-screen">
    <!-- Background Decoration -->
    <div class="ba-theme-backdrop pointer-events-none fixed inset-0"></div>

    <!-- 全局顶栏横跨侧栏和内容区，页面标题由内容区承载。 -->
    <AppHeader />

    <!-- Sidebar and Main Content Area -->
    <AppSidebar v-if="!hideSidebar" />

    <div
      class="relative z-10 min-h-screen min-w-0 pt-14 transition-all duration-300"
      :class="[hideSidebar ? '' : sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-56']"
    >
      <!-- Main Content -->
      <main
        class="app-main min-w-0 px-4 pb-4 pt-4 md:px-6 md:pb-6 md:pt-5 lg:px-8 lg:pb-8 lg:pt-4"
        :class="{ 'has-page-heading': pageTitle }"
      >
        <div v-if="pageTitle" class="page-heading mb-4 flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 class="page-title">{{ pageTitle }}</h1>
            <p v-if="pageDescription" class="page-description">{{ pageDescription }}</p>
          </div>
          <div v-if="$slots['page-heading-actions']" class="shrink-0">
            <slot name="page-heading-actions" />
          </div>
        </div>
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { usePageMeta } from '@/composables/usePageMeta'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const route = useRoute()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
// 全屏工作区页面（如创作台）通过路由 meta 隐藏侧栏并取消内容区缩进。
const hideSidebar = computed(() => route.meta.hideSidebar === true)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour, startTeamTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()
const { pageTitle, pageDescription } = usePageMeta()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
  onboardingStore.setTeamGuideCallback(startTeamTour)
})

defineExpose({ replayTour })
</script>

<style scoped>
/* 表格页需要知道内容区标题占用的固定空间，避免滚动区域向视口底部溢出。 */
.app-main {
  --page-heading-space: 0px;
}

.app-main.has-page-heading {
  --page-heading-space: 5rem;
}
</style>
