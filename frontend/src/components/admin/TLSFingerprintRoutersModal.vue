<template>
  <BaseDialog
    :show="show"
    :title="t('admin.tlsFingerprintRouters.title')"
    width="wide"
    @close="$emit('close')"
  >
    <div class="space-y-4">
      <!-- 头部操作区 -->
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.tlsFingerprintRouters.description') }}
        </p>
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="loadRouters">
            <Icon name="refresh" size="sm" :class="['mr-1', loading ? 'animate-spin' : '']" />
            {{ t('common.refresh') }}
          </button>
          <button type="button" class="btn btn-primary btn-sm" @click="openCreateModal">
            <Icon name="plus" size="sm" class="mr-1" />
            {{ t('admin.tlsFingerprintRouters.createRouter') }}
          </button>
        </div>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-8">
        <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
      </div>

      <div v-else-if="routers.length === 0" class="py-8 text-center">
        <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700">
          <Icon name="swap" size="lg" class="text-gray-400" />
        </div>
        <h4 class="mb-1 text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.tlsFingerprintRouters.noRouters') }}
        </h4>
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.tlsFingerprintRouters.createFirstRouter') }}
        </p>
      </div>

      <div v-else class="max-h-96 overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="sticky top-0 bg-gray-50 dark:bg-dark-700">
            <tr>
              <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintRouters.columns.name') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintRouters.columns.description') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintRouters.columns.rules') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintRouters.columns.status') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintRouters.columns.actions') }}
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
            <tr v-for="router in routers" :key="router.id" class="hover:bg-gray-50 dark:hover:bg-dark-700">
              <td class="px-3 py-2">
                <div class="font-medium text-gray-900 dark:text-white text-sm">{{ router.name }}</div>
              </td>
              <td class="px-3 py-2">
                <div v-if="router.description" class="max-w-xs truncate text-sm text-gray-500 dark:text-gray-400">
                  {{ router.description }}
                </div>
                <div v-else class="text-xs text-gray-400 dark:text-gray-600">-</div>
              </td>
              <td class="px-3 py-2 text-sm text-gray-700 dark:text-gray-300">
                {{ router.rules?.length ?? 0 }}
              </td>
              <td class="px-3 py-2">
                <button
                  type="button"
                  :class="[
                    'rounded-full px-2 py-0.5 text-xs font-medium',
                    router.enabled
                      ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
                      : 'bg-gray-200 text-gray-600 dark:bg-dark-600 dark:text-gray-300'
                  ]"
                  @click="toggleRouter(router)"
                >
                  {{ router.enabled ? t('common.enabled') : t('common.disabled') }}
                </button>
              </td>
              <td class="px-3 py-2">
                <div class="flex items-center gap-1">
                  <button
                    type="button"
                    class="p-1 text-gray-500 hover:text-primary-600 dark:hover:text-primary-400"
                    :title="t('common.edit')"
                    @click="handleEdit(router)"
                  >
                    <Icon name="edit" size="sm" />
                  </button>
                  <button
                    type="button"
                    class="p-1 text-gray-500 hover:text-red-600 dark:hover:text-red-400"
                    :title="t('common.delete')"
                    @click="handleDelete(router)"
                  >
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary" @click="$emit('close')">
          {{ t('common.close') }}
        </button>
      </div>
    </template>

    <BaseDialog
      :show="showCreateModal || showEditModal"
      :title="showEditModal ? t('admin.tlsFingerprintRouters.editRouter') : t('admin.tlsFingerprintRouters.createRouter')"
      width="wide"
      :z-index="60"
      @close="closeFormModal"
    >
      <form class="space-y-4" @submit.prevent="handleSubmit">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.tlsFingerprintRouters.form.name') }}</label>
            <input v-model="form.name" type="text" required class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.tlsFingerprintRouters.form.description') }}</label>
            <input v-model="form.description" type="text" class="input" />
          </div>
        </div>

        <div class="flex items-center justify-between rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <div>
            <label class="input-label mb-0">{{ t('admin.tlsFingerprintRouters.form.enabled') }}</label>
          </div>
          <button
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              form.enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            @click="form.enabled = !form.enabled"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                form.enabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>

        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('admin.tlsFingerprintRouters.form.rules') }}</label>
            <button type="button" class="btn btn-secondary btn-sm" @click="addRule">
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('admin.tlsFingerprintRouters.form.addRule') }}
            </button>
          </div>

          <div v-if="form.rules.length === 0" class="rounded-lg border border-dashed border-gray-300 px-3 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
            {{ t('admin.tlsFingerprintRouters.form.noRules') }}
          </div>

          <div
            v-for="(rule, index) in form.rules"
            :key="index"
            class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="text-sm font-medium text-gray-900 dark:text-white">
                {{ t('admin.tlsFingerprintRouters.form.ruleTitle', { index: index + 1 }) }}
              </div>
              <div class="flex items-center gap-1">
                <button type="button" class="p-1 text-gray-500 hover:text-gray-900 dark:hover:text-white" :disabled="index === 0" @click="moveRule(index, -1)">
                  <Icon name="arrowUp" size="sm" />
                </button>
                <button type="button" class="p-1 text-gray-500 hover:text-gray-900 dark:hover:text-white" :disabled="index === form.rules.length - 1" @click="moveRule(index, 1)">
                  <Icon name="arrowDown" size="sm" />
                </button>
                <button type="button" class="p-1 text-gray-500 hover:text-red-600 dark:hover:text-red-400" @click="removeRule(index)">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div>
                <label class="input-label text-xs">{{ t('admin.tlsFingerprintRouters.form.ruleName') }}</label>
                <input v-model="rule.name" type="text" class="input" />
              </div>
              <div>
                <label class="input-label text-xs">{{ t('admin.tlsFingerprintRouters.form.profile') }}</label>
                <select v-model.number="rule.tls_fingerprint_profile_id" class="input">
                  <option :value="0">{{ t('admin.accounts.quotaControl.tlsFingerprint.defaultProfile') }}</option>
                  <option v-if="profiles.length > 0" :value="-1">{{ t('admin.accounts.quotaControl.tlsFingerprint.randomProfile') }}</option>
                  <option v-for="profile in profiles" :key="profile.id" :value="profile.id">
                    {{ profile.name }}
                  </option>
                </select>
              </div>
              <div>
                <label class="input-label text-xs">{{ t('admin.tlsFingerprintRouters.form.matchType') }}</label>
                <select v-model="rule.match_type" class="input">
                  <option value="contains">{{ t('admin.tlsFingerprintRouters.matchTypes.contains') }}</option>
                  <option value="prefix">{{ t('admin.tlsFingerprintRouters.matchTypes.prefix') }}</option>
                  <option value="exact">{{ t('admin.tlsFingerprintRouters.matchTypes.exact') }}</option>
                  <option value="regex">{{ t('admin.tlsFingerprintRouters.matchTypes.regex') }}</option>
                </select>
              </div>
              <div>
                <label class="input-label text-xs">{{ t('admin.tlsFingerprintRouters.form.pattern') }}</label>
                <input v-model="rule.pattern" type="text" required class="input font-mono text-sm" />
              </div>
              <div>
                <label class="input-label text-xs">{{ t('admin.tlsFingerprintRouters.form.upstreamUserAgent') }}</label>
                <input v-model="rule.upstream_user_agent" type="text" class="input font-mono text-sm" />
                <p class="input-hint">{{ t('admin.tlsFingerprintRouters.form.upstreamUserAgentHint') }}</p>
              </div>
              <div>
                <label class="input-label text-xs">{{ t('admin.tlsFingerprintRouters.form.upstreamOriginator') }}</label>
                <input v-model="rule.upstream_originator" type="text" class="input font-mono text-sm" />
                <p class="input-hint">{{ t('admin.tlsFingerprintRouters.form.upstreamOriginatorHint') }}</p>
              </div>
            </div>

            <div class="flex flex-wrap items-center gap-4 text-sm text-gray-700 dark:text-gray-300">
              <label class="inline-flex items-center gap-2">
                <input v-model="rule.enabled" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                <span>{{ t('admin.tlsFingerprintRouters.form.ruleEnabled') }}</span>
              </label>
              <label class="inline-flex items-center gap-2">
                <input v-model="rule.case_sensitive" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                <span>{{ t('admin.tlsFingerprintRouters.form.caseSensitive') }}</span>
              </label>
            </div>
          </div>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeFormModal">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="submitting" @click="handleSubmit">
            <Icon v-if="submitting" name="refresh" size="sm" class="mr-1 animate-spin" />
            {{ showEditModal ? t('common.update') : t('common.create') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.tlsFingerprintRouters.deleteRouter')"
      :message="t('admin.tlsFingerprintRouters.deleteConfirmMessage', { name: deletingRouter?.name })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { TLSFingerprintProfile } from '@/api/admin/tlsFingerprintProfile'
import type {
  TLSFingerprintRouter,
  TLSFingerprintRouterMatchType,
  TLSFingerprintRouterRule
} from '@/api/admin/tlsFingerprintRouter'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

// eslint-disable-next-line @typescript-eslint/no-unused-vars
void emit // 模板中通过 $emit 使用，脚本侧保留引用避免类型告警。

const { t } = useI18n()
const appStore = useAppStore()

const routers = ref<TLSFingerprintRouter[]>([])
const profiles = ref<TLSFingerprintProfile[]>([])
const loading = ref(false)
const submitting = ref(false)
const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const editingRouter = ref<TLSFingerprintRouter | null>(null)
const deletingRouter = ref<TLSFingerprintRouter | null>(null)

const form = reactive({
  name: '',
  description: '',
  enabled: true,
  rules: [] as TLSFingerprintRouterRule[]
})

watch(() => props.show, (visible) => {
  if (visible) {
    void loadRouters()
    void loadProfiles()
  }
}, { immediate: true })

function normalizeRules(rules: TLSFingerprintRouterRule[] | undefined): TLSFingerprintRouterRule[] {
  return (rules || []).map(rule => ({
    name: rule.name || '',
    enabled: rule.enabled !== false,
    match_type: normalizeMatchType(rule.match_type),
    pattern: rule.pattern || '',
    case_sensitive: rule.case_sensitive === true,
    tls_fingerprint_profile_id: Number(rule.tls_fingerprint_profile_id ?? 0),
    upstream_user_agent: rule.upstream_user_agent?.trim() || '',
    upstream_originator: rule.upstream_originator?.trim() || ''
  }))
}

function normalizeMatchType(value: unknown): TLSFingerprintRouterMatchType {
  return value === 'prefix' || value === 'exact' || value === 'regex' ? value : 'contains'
}

async function loadRouters() {
  loading.value = true
  try {
    routers.value = await adminAPI.tlsFingerprintRouters.list()
  } catch (error) {
    appStore.showError(t('admin.tlsFingerprintRouters.loadFailed'))
    console.error('Error loading TLS fingerprint routers:', error)
  } finally {
    loading.value = false
  }
}

async function loadProfiles() {
  try {
    profiles.value = await adminAPI.tlsFingerprintProfiles.list()
  } catch {
    profiles.value = []
  }
}

function resetForm() {
  form.name = ''
  form.description = ''
  form.enabled = true
  form.rules = []
}

function openCreateModal() {
  resetForm()
  editingRouter.value = null
  showCreateModal.value = true
}

function handleEdit(router: TLSFingerprintRouter) {
  editingRouter.value = router
  form.name = router.name
  form.description = router.description || ''
  form.enabled = router.enabled
  form.rules = normalizeRules(router.rules)
  showEditModal.value = true
}

function closeFormModal() {
  showCreateModal.value = false
  showEditModal.value = false
  editingRouter.value = null
  resetForm()
}

function addRule() {
  form.rules.push({
    name: '',
    enabled: true,
    match_type: 'contains',
    pattern: '',
    case_sensitive: false,
    tls_fingerprint_profile_id: 0,
    upstream_user_agent: '',
    upstream_originator: ''
  })
}

function removeRule(index: number) {
  form.rules.splice(index, 1)
}

function moveRule(index: number, delta: number) {
  const target = index + delta
  if (target < 0 || target >= form.rules.length) return
  const [rule] = form.rules.splice(index, 1)
  form.rules.splice(target, 0, rule)
}

function validateRules(): boolean {
  for (const rule of form.rules) {
    if (!rule.pattern.trim()) {
      appStore.showError(t('admin.tlsFingerprintRouters.form.patternRequired'))
      return false
    }
    if (rule.match_type === 'regex') {
      try {
        new RegExp(rule.pattern)
      } catch {
        appStore.showError(t('admin.tlsFingerprintRouters.form.regexInvalid'))
        return false
      }
    }
  }
  return true
}

async function handleSubmit() {
  if (!form.name.trim() || !validateRules()) return
  submitting.value = true
  try {
    const payload = {
      name: form.name.trim(),
      description: form.description.trim() || null,
      enabled: form.enabled,
      rules: normalizeRules(form.rules)
    }
    if (editingRouter.value) {
      await adminAPI.tlsFingerprintRouters.update(editingRouter.value.id, payload)
      appStore.showSuccess(t('admin.tlsFingerprintRouters.updateSuccess'))
    } else {
      await adminAPI.tlsFingerprintRouters.create(payload)
      appStore.showSuccess(t('admin.tlsFingerprintRouters.createSuccess'))
    }
    closeFormModal()
    await loadRouters()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.tlsFingerprintRouters.saveFailed'))
  } finally {
    submitting.value = false
  }
}

async function toggleRouter(router: TLSFingerprintRouter) {
  try {
    await adminAPI.tlsFingerprintRouters.update(router.id, { enabled: !router.enabled })
    await loadRouters()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.tlsFingerprintRouters.toggleFailed'))
  }
}

function handleDelete(router: TLSFingerprintRouter) {
  deletingRouter.value = router
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingRouter.value) return
  try {
    await adminAPI.tlsFingerprintRouters.delete(deletingRouter.value.id)
    appStore.showSuccess(t('admin.tlsFingerprintRouters.deleteSuccess'))
    showDeleteDialog.value = false
    deletingRouter.value = null
    await loadRouters()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.tlsFingerprintRouters.deleteFailed'))
  }
}
</script>
