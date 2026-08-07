<template>
  <section class="border-t border-gray-200 pt-4 dark:border-dark-400">
    <div class="mb-3">
      <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.groups.clientProtocols.title') }}
      </h4>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.groups.clientProtocols.hint') }}
      </p>
    </div>

    <div class="divide-y divide-gray-100 rounded-md border border-gray-200 dark:divide-dark-700 dark:border-dark-600">
      <div
        v-for="protocol in protocols"
        :key="protocol"
        class="flex min-h-14 items-center justify-between gap-4 px-3 py-2.5"
      >
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium text-gray-800 dark:text-gray-200">
              {{ t(`admin.groups.clientProtocols.labels.${protocol}`) }}
            </span>
            <span
              v-if="isRequired(protocol)"
              class="inline-flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400"
            >
              <Icon name="lock" size="xs" />
              {{ t('admin.groups.clientProtocols.required') }}
            </span>
          </div>
        </div>

        <button
          type="button"
          role="switch"
          :data-protocol="protocol"
          :aria-checked="isEnabled(protocol)"
          :aria-label="t(`admin.groups.clientProtocols.labels.${protocol}`)"
          :title="isRequired(protocol) ? t('admin.groups.clientProtocols.requiredHint') : undefined"
          :disabled="isRequired(protocol)"
          class="relative inline-flex h-6 w-12 flex-shrink-0 rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-70"
          :class="isEnabled(protocol) ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'"
          @click="toggle(protocol)"
        >
          <span
            class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
            :class="isEnabled(protocol) ? 'translate-x-6' : 'translate-x-1'"
          />
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { GroupClientProtocol, GroupPlatform } from '@/types'
import {
  hasGroupClientProtocol,
  requiredGroupClientProtocols,
  setGroupClientProtocol,
  supportedGroupClientProtocols
} from '@/utils/groupClientProtocols'

const props = defineProps<{
  modelValue: GroupClientProtocol[]
  platform: GroupPlatform
}>()

const emit = defineEmits<{
  (event: 'update:modelValue', value: GroupClientProtocol[]): void
}>()

const { t } = useI18n()
const protocols = computed(() => supportedGroupClientProtocols(props.platform))
const requiredProtocols = computed(() => new Set(requiredGroupClientProtocols(props.platform)))

const isRequired = (protocol: GroupClientProtocol) => requiredProtocols.value.has(protocol)
const isEnabled = (protocol: GroupClientProtocol) => hasGroupClientProtocol(props.modelValue, protocol)

const toggle = (protocol: GroupClientProtocol) => {
  emit(
    'update:modelValue',
    setGroupClientProtocol(props.platform, props.modelValue, protocol, !isEnabled(protocol))
  )
}
</script>
