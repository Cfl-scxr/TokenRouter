<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="invitationToken || transferToken" class="border-b border-gray-200 pb-6 dark:border-dark-700">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ invitationToken ? t('team.inviteActionTitle') : t('team.transferActionTitle') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ invitationToken ? t('team.inviteActionDescription') : t('team.transferActionDescription') }}
        </p>
        <div class="mt-4 flex gap-3">
          <button class="btn btn-primary" :disabled="resolvingToken" @click="resolvePendingToken('accepted')">
            <Icon name="check" size="sm" />
            {{ t('team.accept') }}
          </button>
          <button class="btn btn-secondary" :disabled="resolvingToken" @click="resolvePendingToken('declined')">
            <Icon name="x" size="sm" />
            {{ t('team.decline') }}
          </button>
        </div>
      </div>

      <div v-if="loading" class="flex justify-center py-16">
        <LoadingSpinner />
      </div>

      <div v-else-if="!teamContext" class="mx-auto max-w-xl py-10">
        <div class="mb-8 text-center">
          <div class="mx-auto flex h-14 w-14 items-center justify-center rounded-md bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400">
            <Icon name="users" size="xl" />
          </div>
          <h1 class="mt-5 text-2xl font-semibold text-gray-900 dark:text-white">{{ t('team.createTitle') }}</h1>
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('team.createDescription') }}</p>
        </div>
        <form class="space-y-4" @submit.prevent="createTeam">
          <div>
            <label class="input-label">{{ t('team.name') }}</label>
            <input v-model.trim="createName" class="input" maxlength="100" required />
          </div>
          <button type="submit" class="btn btn-primary w-full" :disabled="submitting">
            <Icon name="plus" size="sm" />
            {{ t('team.create') }}
          </button>
        </form>
      </div>

      <template v-else>
        <header class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <div class="flex flex-wrap items-center gap-3">
              <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ teamContext.team.name }}</h1>
              <span class="badge" :class="teamContext.team.status === 'active' ? 'badge-success' : 'badge-danger'">
                {{ teamContext.team.status === 'active' ? t('team.statusActive') : t('team.statusSuspended') }}
              </span>
            </div>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ isOwner ? t('team.owner') : t('team.member') }} · {{ t('team.memberCount', { count: teamContext.team.member_count + 1 }) }}
            </p>
          </div>
          <button class="btn btn-secondary" :disabled="refreshing" @click="refreshAll">
            <Icon name="refresh" size="sm" />
            {{ t('common.refresh') }}
          </button>
        </header>

        <nav class="flex gap-1 overflow-x-auto border-b border-gray-200 dark:border-dark-700" aria-label="Team sections">
          <button
            v-for="tab in visibleTabs"
            :key="tab.value"
            type="button"
            class="shrink-0 border-b-2 px-4 py-3 text-sm font-medium transition-colors"
            :class="activeTab === tab.value
              ? 'border-primary-500 text-primary-600 dark:text-primary-400'
              : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
            @click="activeTab = tab.value"
          >
            {{ tab.label }}
          </button>
        </nav>

        <section v-if="activeTab === 'overview'" class="space-y-6">
          <div v-if="!isOwner" class="card p-5">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('team.limitProgress') }}</h2>
            <div class="mt-5 grid gap-5 md:grid-cols-3">
              <div v-for="limit in memberLimits" :key="limit.label">
                <div class="flex items-center justify-between gap-3 text-sm">
                  <span class="text-gray-600 dark:text-gray-300">{{ limit.label }}</span>
                  <span class="font-medium text-gray-900 dark:text-white">
                    <template v-if="limit.limit > 0">
                      <BalanceAmount :amount="limit.used" /> / <BalanceAmount :amount="limit.limit" />
                    </template>
                    <template v-else>{{ t('team.unlimited') }}</template>
                  </span>
                </div>
                <div class="mt-2 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                  <div class="h-full rounded-full" :class="limit.percent >= 100 ? 'bg-red-500' : limit.percent >= 80 ? 'bg-amber-500' : 'bg-emerald-500'" :style="{ width: `${limit.percent}%` }" />
                </div>
              </div>
            </div>
          </div>

          <section v-if="isOwner" class="card overflow-hidden">
            <div class="flex items-center justify-between gap-4 border-b border-gray-200 px-5 py-4 dark:border-dark-700">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('team.members') }}</h2>
              <span class="text-sm text-gray-500 dark:text-gray-400">{{ t('team.memberCount', { count: members.length }) }}</span>
            </div>
            <div v-if="members.length === 0" class="py-12 text-center text-sm text-gray-500">{{ t('team.noMembers') }}</div>
            <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
              <div v-for="member in members" :key="member.user_id" class="flex flex-wrap items-center justify-between gap-4 p-5">
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <p class="truncate font-medium text-gray-900 dark:text-white">{{ member.username || member.email }}</p>
                    <span class="badge" :class="member.role === 'owner' ? 'badge-primary' : 'badge-gray'">{{ member.role === 'owner' ? t('team.owner') : t('team.member') }}</span>
                  </div>
                  <p class="mt-1 truncate text-sm text-gray-500">{{ member.email }}</p>
                  <p v-if="member.role === 'member'" class="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500">
                    <span class="inline-flex items-center gap-1">{{ t('team.daily') }} <BalanceAmount :amount="member.daily_usage_usd" /> / <BalanceAmount v-if="member.daily_limit_usd > 0" :amount="member.daily_limit_usd" /><span v-else>{{ t('team.unlimited') }}</span></span>
                    <span aria-hidden="true">·</span>
                    <span class="inline-flex items-center gap-1">{{ t('team.weekly') }} <BalanceAmount :amount="member.weekly_usage_usd" /> / <BalanceAmount v-if="member.weekly_limit_usd > 0" :amount="member.weekly_limit_usd" /><span v-else>{{ t('team.unlimited') }}</span></span>
                    <span aria-hidden="true">·</span>
                    <span class="inline-flex items-center gap-1">{{ t('team.monthly') }} <BalanceAmount :amount="member.monthly_usage_usd" /> / <BalanceAmount v-if="member.monthly_limit_usd > 0" :amount="member.monthly_limit_usd" /><span v-else>{{ t('team.unlimited') }}</span></span>
                  </p>
                </div>
                <div v-if="member.role === 'member'" class="flex flex-wrap gap-2">
                  <button class="btn btn-secondary btn-sm" @click="openLimitEditor(member)"><Icon name="edit" size="sm" />{{ t('team.editLimits') }}</button>
                  <button class="btn btn-secondary btn-sm" @click="askTransfer(member)"><Icon name="swap" size="sm" />{{ t('team.transfer') }}</button>
                  <button class="btn btn-danger btn-sm" @click="askRemove(member)"><Icon name="trash" size="sm" />{{ t('team.remove') }}</button>
                </div>
              </div>
            </div>
          </section>

          <section v-if="isOwner" class="space-y-5">
            <div class="flex items-center justify-between gap-4">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('team.invitations') }}</h2>
              <span class="text-sm text-gray-500 dark:text-gray-400">{{ invitations.length }}</span>
            </div>
            <form class="flex flex-col gap-3 sm:flex-row" @submit.prevent="sendInvitation">
              <input v-model.trim="inviteEmail" type="email" class="input flex-1" :placeholder="t('team.inviteEmail')" required />
              <button class="btn btn-primary" :disabled="submitting"><Icon name="mail" size="sm" />{{ t('team.sendInvite') }}</button>
            </form>
            <div class="card divide-y divide-gray-100 dark:divide-dark-700">
              <div v-if="invitations.length === 0" class="py-12 text-center text-sm text-gray-500">{{ t('team.noInvitations') }}</div>
              <div v-for="invitation in invitations" :key="invitation.id" class="flex flex-wrap items-center justify-between gap-4 p-5">
                <div><p class="font-medium text-gray-900 dark:text-white">{{ invitation.email }}</p><p class="mt-1 text-sm text-gray-500">{{ statusLabel(invitation.status) }} · {{ t('team.expiresAt') }} {{ formatDateTime(invitation.expires_at) }}</p></div>
                <div v-if="invitation.status === 'pending'" class="flex gap-2"><button class="btn btn-secondary btn-sm" @click="reissueInvitation(invitation.id)"><Icon name="refresh" size="sm" />{{ t('team.reissue') }}</button><button class="btn btn-danger btn-sm" @click="revokeInvitation(invitation.id)"><Icon name="x" size="sm" />{{ t('team.revoke') }}</button></div>
              </div>
            </div>
          </section>
        </section>

        <section v-else class="space-y-6">
          <div v-if="isOwner" class="card p-5">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('team.name') }}</h2>
            <form class="mt-4 flex flex-col gap-3 sm:flex-row" @submit.prevent="renameTeam"><input v-model.trim="renameName" class="input flex-1" required maxlength="100" /><button class="btn btn-primary" :disabled="submitting">{{ t('team.rename') }}</button></form>
          </div>
          <div v-if="isOwner" class="card p-5">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('team.defaultMemberLimits') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('team.defaultMemberLimitsDescription') }}</p>
            <form class="mt-5" @submit.prevent="saveDefaultMemberLimits">
              <div class="grid gap-4 md:grid-cols-3">
                <div v-for="field in defaultLimitFields" :key="field.key">
                  <label class="input-label">{{ field.label }}</label>
                  <div class="relative">
                    <span class="pointer-events-none absolute left-3 top-1/2 inline-flex -translate-y-1/2 items-center text-sm text-gray-400">
                      <BalanceIcon v-if="hasCustomBalanceIcon" size="sm" />
                      <span v-else>{{ balanceUnitSymbol }}</span>
                    </span>
                    <input v-model.number="defaultLimitForm[field.key]" type="number" min="0" step="0.01" class="input pl-10" required />
                  </div>
                </div>
              </div>
              <div class="mt-5 flex justify-end">
                <button class="btn btn-primary" :disabled="submitting">{{ t('team.saveDefaultLimits') }}</button>
              </div>
            </form>
          </div>
          <div v-if="isOwner" class="card flex flex-wrap items-center justify-between gap-4 p-5">
            <div><h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('team.teamStatus') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('team.teamStatusDescription') }}</p></div>
            <button class="btn btn-secondary" @click="askSetStatus"><Icon :name="teamContext.team.status === 'active' ? 'ban' : 'play'" size="sm" />{{ teamContext.team.status === 'active' ? t('team.pause') : t('team.resume') }}</button>
          </div>
          <div class="border-t border-red-200 pt-6 dark:border-red-900/60">
            <button v-if="isOwner" class="btn btn-danger" @click="askDissolve"><Icon name="trash" size="sm" />{{ t('team.dissolve') }}</button>
            <button v-else class="btn btn-danger" @click="askLeave"><Icon name="arrowRight" size="sm" />{{ t('team.leave') }}</button>
          </div>
        </section>
      </template>
    </div>

    <BaseDialog :show="Boolean(limitTarget)" :title="t('team.editLimits')" width="narrow" @close="limitTarget = null">
      <form id="team-limit-form" class="space-y-4" @submit.prevent="saveLimits">
        <div v-for="field in limitFields" :key="field.key"><label class="input-label">{{ field.label }}</label><input v-model.number="limitForm[field.key]" type="number" min="0" step="0.01" class="input" /></div>
        <div class="border-t border-gray-200 pt-4 dark:border-dark-700"><p class="input-label">{{ t('team.resetUsage') }}</p><div class="flex flex-wrap gap-4 text-sm text-gray-600 dark:text-gray-300"><label v-for="period in resetPeriods" :key="period.key" class="flex items-center gap-2"><input v-model="resetForm[period.key]" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />{{ period.label }}</label></div></div>
      </form>
      <template #footer><div class="flex justify-end gap-3"><button class="btn btn-secondary" @click="limitTarget = null">{{ t('common.cancel') }}</button><button form="team-limit-form" type="submit" class="btn btn-primary">{{ t('team.saveLimits') }}</button></div></template>
    </BaseDialog>

    <ConfirmDialog :show="Boolean(confirmAction)" :title="confirmAction?.title || ''" :message="confirmAction?.message || ''" :danger="confirmAction?.danger" @cancel="confirmAction = null" @confirm="runConfirmedAction" />
    <TotpStepUpDialog :controller="stepUp" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import BalanceAmount from '@/components/common/BalanceAmount.vue'
import BalanceIcon from '@/components/common/BalanceIcon.vue'
import { teamAPI, type TeamContext, type TeamInvitation, type TeamMembership } from '@/api/team'
import { useAppStore } from '@/stores/app'
import { useStepUp, isStepUpCancelled } from '@/composables/useStepUp'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import { formatDateTime } from '@/utils/format'

type TeamTab = 'overview' | 'settings'
type LimitKey = 'daily_limit_usd' | 'weekly_limit_usd' | 'monthly_limit_usd'
type ResetKey = 'daily' | 'weekly' | 'monthly'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const stepUp = useStepUp()
const { balanceUnitSymbol, hasCustomBalanceIcon } = useBalanceDisplay()
const loading = ref(true)
const refreshing = ref(false)
const submitting = ref(false)
const resolvingToken = ref(false)
const teamContext = ref<TeamContext | null>(null)
const members = ref<TeamMembership[]>([])
const invitations = ref<TeamInvitation[]>([])
const activeTab = ref<TeamTab>('overview')
const createName = ref('')
const renameName = ref('')
const inviteEmail = ref('')
const defaultLimitForm = reactive<Record<LimitKey, number>>({ daily_limit_usd: 0, weekly_limit_usd: 0, monthly_limit_usd: 0 })
const invitationToken = computed(() => typeof route.query.invitation === 'string' ? route.query.invitation : '')
const transferToken = computed(() => typeof route.query.transfer === 'string' ? route.query.transfer : '')
const isOwner = computed(() => teamContext.value?.membership.role === 'owner')

const visibleTabs = computed(() => [
  { value: 'overview' as const, label: t('team.overview') },
  { value: 'settings' as const, label: t('team.settings') }
])
const defaultLimitFields = computed(() => ([
  { key: 'daily_limit_usd' as const, label: t('team.defaultDailyLimit') },
  { key: 'weekly_limit_usd' as const, label: t('team.defaultWeeklyLimit') },
  { key: 'monthly_limit_usd' as const, label: t('team.defaultMonthlyLimit') }
]))

const statusLabel = (status: string) => {
  const labels: Record<string, string> = {
    active: t('team.statusActive'),
    inactive: t('team.statusInactive'),
    disabled: t('team.statusDisabled'),
    pending: t('team.statusPending'),
    accepted: t('team.statusAccepted'),
    declined: t('team.statusDeclined'),
    revoked: t('team.statusRevoked'),
    expired: t('team.statusExpired')
  }
  return labels[status] || status
}
const memberLimits = computed(() => {
  const membership = teamContext.value?.membership
  if (!membership) return []
  return [
    { label: t('team.daily'), used: membership.daily_usage_usd, limit: membership.daily_limit_usd },
    { label: t('team.weekly'), used: membership.weekly_usage_usd, limit: membership.weekly_limit_usd },
    { label: t('team.monthly'), used: membership.monthly_usage_usd, limit: membership.monthly_limit_usd }
  ].map((item) => ({ ...item, percent: item.limit > 0 ? Math.min(100, (item.used / item.limit) * 100) : 0 }))
})
const isNoTeamError = (error: any) => error?.reason === 'TEAM_NOT_FOUND' || error?.reason === 'TEAM_MEMBERSHIP_REQUIRED' || error?.response?.status === 404
const loadContext = async () => {
  try {
    teamContext.value = await teamAPI.current()
    renameName.value = teamContext.value.team.name
    defaultLimitForm.daily_limit_usd = teamContext.value.team.default_daily_limit_usd
    defaultLimitForm.weekly_limit_usd = teamContext.value.team.default_weekly_limit_usd
    defaultLimitForm.monthly_limit_usd = teamContext.value.team.default_monthly_limit_usd
  } catch (error) {
    if (isNoTeamError(error)) teamContext.value = null
    else throw error
  }
}
const loadTeamData = async () => {
  if (!teamContext.value) return
  if (teamContext.value.team.status !== 'active') {
    members.value = []
    invitations.value = []
    return
  }
  const requests: Promise<unknown>[] = []
  if (isOwner.value) {
    requests.push(teamAPI.members().then((value) => { members.value = value }))
    requests.push(teamAPI.invitations().then((value) => { invitations.value = value }))
  }
  await Promise.all(requests)
}
const refreshAll = async () => {
  refreshing.value = true
  try { await loadContext(); await loadTeamData() } catch (error: any) { appStore.showError(error?.message || t('team.loadFailed')) } finally { refreshing.value = false }
}

const createTeam = async () => {
  submitting.value = true
  try { teamContext.value = await teamAPI.create(createName.value); appStore.showSuccess(t('team.created')); await loadTeamData() } catch (error: any) { appStore.showError(error?.message || t('common.error')) } finally { submitting.value = false }
}
const renameTeam = async () => {
  submitting.value = true
  try { teamContext.value = await teamAPI.rename(renameName.value); appStore.showSuccess(t('team.updated')) } catch (error: any) { appStore.showError(error?.message || t('common.error')) } finally { submitting.value = false }
}
const saveDefaultMemberLimits = async () => {
  submitting.value = true
  try {
    teamContext.value = await teamAPI.updateDefaultMemberLimits({
      default_daily_limit_usd: defaultLimitForm.daily_limit_usd,
      default_weekly_limit_usd: defaultLimitForm.weekly_limit_usd,
      default_monthly_limit_usd: defaultLimitForm.monthly_limit_usd
    })
    appStore.showSuccess(t('team.defaultLimitsUpdated'))
  } catch (error: any) {
    appStore.showError(error?.message || t('common.error'))
  } finally {
    submitting.value = false
  }
}
const sendInvitation = async () => {
  submitting.value = true
  try { await teamAPI.invite(inviteEmail.value); inviteEmail.value = ''; invitations.value = await teamAPI.invitations(); appStore.showSuccess(t('team.operationSuccess')) } catch (error: any) { appStore.showError(error?.message || t('common.error')) } finally { submitting.value = false }
}
const reissueInvitation = async (id: number) => { try { await teamAPI.reissueInvitation(id); invitations.value = await teamAPI.invitations(); appStore.showSuccess(t('team.operationSuccess')) } catch (error: any) { appStore.showError(error?.message || t('common.error')) } }
const revokeInvitation = async (id: number) => { try { await teamAPI.revokeInvitation(id); invitations.value = await teamAPI.invitations(); appStore.showSuccess(t('team.operationSuccess')) } catch (error: any) { appStore.showError(error?.message || t('common.error')) } }

const limitTarget = ref<TeamMembership | null>(null)
const limitForm = reactive<Record<LimitKey, number>>({ daily_limit_usd: 0, weekly_limit_usd: 0, monthly_limit_usd: 0 })
const resetForm = reactive<Record<ResetKey, boolean>>({ daily: false, weekly: false, monthly: false })
const limitFields = computed(() => ([{ key: 'daily_limit_usd' as const, label: t('team.daily') }, { key: 'weekly_limit_usd' as const, label: t('team.weekly') }, { key: 'monthly_limit_usd' as const, label: t('team.monthly') }]))
const resetPeriods = computed(() => ([{ key: 'daily' as const, label: t('team.daily') }, { key: 'weekly' as const, label: t('team.weekly') }, { key: 'monthly' as const, label: t('team.monthly') }]))
const openLimitEditor = (member: TeamMembership) => { limitTarget.value = member; limitForm.daily_limit_usd = member.daily_limit_usd; limitForm.weekly_limit_usd = member.weekly_limit_usd; limitForm.monthly_limit_usd = member.monthly_limit_usd; resetForm.daily = resetForm.weekly = resetForm.monthly = false }
const saveLimits = async () => {
  if (!limitTarget.value) return
  try { await teamAPI.updateLimits(limitTarget.value.user_id, { ...limitForm }); if (resetForm.daily || resetForm.weekly || resetForm.monthly) await teamAPI.resetUsage(limitTarget.value.user_id, { ...resetForm }); limitTarget.value = null; await refreshAll(); appStore.showSuccess(t('team.operationSuccess')) } catch (error: any) { appStore.showError(error?.message || t('common.error')) }
}

type ConfirmAction = { title: string; message: string; danger?: boolean; run: () => Promise<void> }
const confirmAction = ref<ConfirmAction | null>(null)
const runConfirmedAction = async () => { const action = confirmAction.value; confirmAction.value = null; if (!action) return; try { await action.run(); appStore.showSuccess(t('team.operationSuccess')) } catch (error: any) { if (!isStepUpCancelled(error)) appStore.showError(error?.message || t('common.error')) } }
const askRemove = (member: TeamMembership) => { confirmAction.value = { title: t('team.removeTitle'), message: t('team.removeMessage'), danger: true, run: async () => { await teamAPI.removeMember(member.user_id); await refreshAll() } } }
const askTransfer = (member: TeamMembership) => { confirmAction.value = { title: t('team.transferTitle'), message: t('team.transferMessage'), run: async () => { await stepUp.run(() => teamAPI.startTransfer(member.user_id)) } } }
const askLeave = () => { confirmAction.value = { title: t('team.leaveTitle'), message: t('team.leaveMessage'), danger: true, run: async () => { await teamAPI.leave(); teamContext.value = null } } }
const askDissolve = () => { confirmAction.value = { title: t('team.dissolveTitle'), message: t('team.dissolveMessage'), danger: true, run: async () => { await stepUp.run(() => teamAPI.dissolve()); teamContext.value = null } } }
const askSetStatus = () => {
  if (!teamContext.value) return
  const nextStatus = teamContext.value.team.status === 'active' ? 'suspended' : 'active'
  confirmAction.value = {
    title: nextStatus === 'suspended' ? t('team.pauseTitle') : t('team.resumeTitle'),
    message: nextStatus === 'suspended' ? t('team.pauseMessage') : t('team.resumeMessage'),
    danger: nextStatus === 'suspended',
    run: async () => { teamContext.value = await stepUp.run(() => teamAPI.setStatus(nextStatus)); await loadTeamData() }
  }
}

const resolvePendingToken = async (resolution: 'accepted' | 'declined') => {
  resolvingToken.value = true
  try {
    if (invitationToken.value) await teamAPI.resolveInvitation(invitationToken.value, resolution)
    else if (transferToken.value) await stepUp.run(() => teamAPI.resolveTransfer(transferToken.value, resolution))
    await router.replace({ query: {} })
    await refreshAll()
    appStore.showSuccess(t('team.operationSuccess'))
  } catch (error: any) {
    if (!isStepUpCancelled(error)) appStore.showError(error?.message || t('common.error'))
  } finally { resolvingToken.value = false }
}

onMounted(async () => {
  try { await loadContext(); await loadTeamData() } catch (error: any) { appStore.showError(error?.message || t('team.loadFailed')) } finally { loading.value = false }
})
</script>
