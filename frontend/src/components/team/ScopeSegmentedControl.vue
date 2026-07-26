<template>
  <div class="inline-grid h-10 grid-cols-2 rounded-md border border-gray-200 bg-gray-100 p-1 dark:border-dark-600 dark:bg-dark-800" role="tablist">
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      role="tab"
      :aria-selected="scope === option.value"
      class="min-w-24 rounded px-4 text-sm font-medium transition-colors"
      :class="scope === option.value
        ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
        : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
      @click="setScope(option.value)"
    >
      {{ option.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

export type DataScope = 'personal' | 'team'

const props = defineProps<{ modelValue: DataScope }>()
const emit = defineEmits<{ (event: 'update:modelValue', value: DataScope): void; (event: 'change', value: DataScope): void }>()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const scope = computed(() => props.modelValue)
const options = computed(() => [
  { value: 'personal' as const, label: t('team.personal') },
  { value: 'team' as const, label: t('team.teamScope') }
])

// 作用域写入 URL，保证刷新和分享链接后仍保持当前视图。
const setScope = async (value: DataScope) => {
  if (value === scope.value) return
  emit('update:modelValue', value)
  await router.replace({ query: { ...route.query, scope: value } })
  emit('change', value)
}
</script>
