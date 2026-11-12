<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { ApprovalItem, ApprovalTab } from '~/stores/approvals';
import AppButton from '#base/components/ui/AppButton.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import { useToast } from '#base/components/ui/toast';
import { useDates } from '#base/composables/useDates';
import ApprovalCard from '~/components/approval/ApprovalCard.vue';
import { decisionLabel, expiresIn } from '~/components/approval/decision';
import { shownName } from '~/components/enduser/forget';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useProject } from '~/components/project/useProject';
import { chatTime } from '~/components/try/chatTime';
import { useCan } from '~/composables/useCan';
import { tick, useNow } from '~/composables/useNow';
import { useApprovalsStore } from '~/stores/approvals';
import { useAuthStore } from '~/stores/auth';

const refreshMs = 15_000;

const { t, locale } = useI18n();
const route = useRoute();
const { when, moment } = useDates();
const { project } = useProject();
const canDecide = useCan('approval:decide');
const store = useApprovalsStore();
const auth = useAuthStore();
const toast = useToast();
const now = useNow();
const listId = useId();

const tabs: ApprovalTab[] = ['pending', 'decided', 'expired'];
const tab = computed<ApprovalTab>(() => (tabs.includes(route.query.tab as ApprovalTab) ? route.query.tab as ApprovalTab : 'pending'));
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}`);
const byTab = computed<Record<ApprovalTab, ApprovalItem[]>>(() => ({ pending: store.pending, decided: store.decided, expired: store.expired }));
const threadLink = (i: ApprovalItem) => `${base.value}/agents/${i.agentSlug}/threads/${i.threadId}`;
const runLink = (i: ApprovalItem) => `${base.value}/agents/${i.agentSlug}/runs/${i.runId}`;
const deciding = ref<string | null>(null);
const focused = ref<string | null>(null);
const notes = ref<Record<string, string>>({});
const announced = ref('');

watch([tab, () => project.value?.id], ([x, id]) => {
  if (id) {
    void store.ensureLoaded(id, x);
  }
}, { immediate: true });

const columns = computed<Column[]>(() => [
  { key: 'request', label: t('approvals.col_request') },
  { key: 'tool', label: t('approvals.col_tool'), mono: true },
  { key: 'agentName', label: t('approvals.col_agent') },
  { key: 'asked', label: t('approvals.col_asked_by') },
  { key: 'time', label: t('approvals.col_time'), muted: true },
  { key: 'decision', label: t('approvals.col_decision') },
]);
const rows = computed(() => byTab.value[tab.value].map(i => ({ ...i, id: i.approval.id })));

function decision(i: ApprovalItem) {
  return decisionLabel(i.approval, t, d => chatTime(d, locale.value), now.value);
}

function countdown(i: ApprovalItem) {
  const c = expiresIn(i.approval, now.value);
  return c ? t(c.key, { n: c.n }) : '';
}

async function decide(i: ApprovalItem, approve: boolean, reason = '') {
  const id = i.approval.id;
  const title = i.approval.title;
  const at = byTab.value.pending.indexOf(i);
  deciding.value = id;
  const res = await store.decide(id, approve, reason);
  deciding.value = null;
  if (res.outcome === 'elsewhere' || res.outcome === 'expired') {
    notes.value[id] = res.outcome === 'expired' ? t('approvals.expired_before') : reason.trim() ? t('approvals.reason_not_sent') : '';
    return;
  }
  if (res.outcome === 'app' || res.outcome === 'error') {
    toast.push('error', res.error?.message ?? t('error.decide_approval'));
    return;
  }
  toast.push(res.outcome === 'gone' ? 'info' : 'success', t(res.outcome === 'gone' ? 'approvals.gone_toast' : approve ? 'approvals.approved_toast' : 'approvals.rejected_toast', { title }));
  await nextTick();
  const left = byTab.value.pending;
  const next = left[Math.min(at, left.length - 1)];
  document.getElementById(next ? `${listId}-${next.approval.id}` : listId)?.focus();
}

async function refresh() {
  if (!project.value || document.visibilityState === 'hidden') {
    return;
  }
  tick();
  const keep = new Set([...store.noted, ...[deciding.value, focused.value].filter((x): x is string => !!x)]);
  const added = await store.refreshPending(keep);
  if (added) {
    announced.value = t('approvals.new_count', { n: added }, added);
  }
}

let timer: ReturnType<typeof setInterval> | undefined;
function onVisible() {
  if (document.visibilityState === 'visible') {
    void refresh();
  }
}
onMounted(() => {
  timer = setInterval(() => void refresh(), refreshMs);
  document.addEventListener('visibilitychange', onVisible);
  window.addEventListener('focus', onVisible);
});
onBeforeUnmount(() => {
  clearInterval(timer);
  document.removeEventListener('visibilitychange', onVisible);
  window.removeEventListener('focus', onVisible);
});

useHead({ title: () => `${t('approvals.title')} · ${project.value?.name ?? ''} · OAgent` });

await auth.load();
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('approvals.title')" :description="t('approvals.subtitle')" />
    <ProjectReadOnlyNote v-if="!canDecide" :text="t('approvals.cant_decide')" />
    <p role="status" class="sr-only">
      {{ announced }}
    </p>

    <LoadError v-if="store.error" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(project!.id, tab)" />
    <template v-else>
      <nav class="tabs tabs-border tabs-sm" :aria-label="t('approvals.tabs')">
        <NuxtLink
          v-for="x in tabs" :key="x" :to="{ query: x === 'pending' ? {} : { tab: x } }" class="tab"
          :class="tab === x ? 'tab-active' : 'text-base-content/70'" :aria-current="tab === x ? 'page' : undefined"
        >
          {{ t(`approvals.tab_${x}`) }}
          <span v-if="x === 'pending'" class="badge badge-sm badge-ghost ms-1.5 tabular-nums">{{ store.pendingCount }}</span>
        </NuxtLink>
      </nav>
      <h2 class="sr-only">
        {{ t(`approvals.tab_${tab}`) }}
      </h2>

      <template v-if="tab === 'pending'">
        <div :id="listId" tabindex="-1" class="focus:outline-none">
          <EmptyState v-if="!store.loading && !byTab.pending.length" icon="lucide:hand" :title="t('approvals.empty_pending')" />
          <ul v-else class="flex flex-col gap-2" :aria-label="t('approvals.tab_pending')">
            <li v-for="i in byTab.pending" :key="i.approval.id" :data-approval="i.approval.id" @focusin="focused = i.approval.id" @focusout="focused = null">
              <details class="group collapse-arrow collapse rounded-box border border-base-300 bg-base-100" :open="byTab.pending.length === 1 || store.noted.has(i.approval.id)">
                <summary :id="`${listId}-${i.approval.id}`" class="collapse-title flex flex-wrap items-center gap-x-4 gap-y-1 py-3 text-ui">
                  <span class="font-medium group-open:hidden">{{ i.approval.title }}</span>
                  <span class="text-base-content/70">{{ [i.agentName, shownName(i.approval.requestedBy, i.approval.requesterDeleted, t), t(`approvals.channel_${i.channel}`)].filter(Boolean).join(' · ') }}</span>
                  <span class="text-xs text-base-content/70">
                    {{ t('approvals.asked', { when: when(i.askedAt) }) }}<template v-if="countdown(i)"> · {{ countdown(i) }}</template>
                  </span>
                </summary>
                <div class="collapse-content flex flex-col gap-2">
                  <ApprovalCard
                    class="max-w-xl" :approval="i.approval" :read-only="!canDecide" :answered-by-app="i.channel === 'api'" :busy="deciding === i.approval.id"
                    :note="notes[i.approval.id]" @approve="decide(i, true)" @reject="decide(i, false, $event)"
                  />
                  <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-ui">
                    <template v-if="i.agentSlug">
                      <NuxtLink :to="threadLink(i)" class="link">
                        {{ t('approvals.open_thread') }}
                      </NuxtLink>
                      <NuxtLink :to="runLink(i)" class="link">
                        {{ t('approvals.open_run') }}
                      </NuxtLink>
                    </template>
                    <AppButton v-if="store.noted.has(i.approval.id)" variant="plain" size="xs" @click="store.dismiss(i.approval.id)">
                      {{ t('approvals.dismiss') }}
                    </AppButton>
                  </div>
                </div>
              </details>
            </li>
          </ul>
        </div>
      </template>

      <DataTable v-else stacked :caption="t(`approvals.tab_${tab}`)" row-key="id" :columns="columns" :rows="rows" :loading="store.loading">
        <template #empty>
          <EmptyState icon="lucide:inbox" :title="t(`approvals.empty_${tab}`)" />
        </template>
        <template #cell-request="{ row }">
          <NuxtLink v-if="row.agentSlug" :to="threadLink(row as ApprovalItem)" class="link link-hover font-medium">
            {{ row.approval.title }}
          </NuxtLink>
          <span v-else class="font-medium">{{ row.approval.title }}</span>
        </template>
        <template #cell-tool="{ row }">
          {{ row.approval.tool }}
        </template>
        <template #cell-asked="{ row }">
          {{ [shownName(row.approval.requestedBy, row.approval.requesterDeleted, t), t(`approvals.channel_${row.channel}`)].filter(Boolean).join(' · ') }}
        </template>
        <template #cell-time="{ row }">
          <span class="tabular-nums">{{ row.approval.status === 'expired' ? t('approvals.expired_at', { at: moment(row.approval.expiresAt ?? row.approval.decidedAt ?? row.askedAt) }) : row.approval.decidedAt ? t('approvals.decided', { at: moment(row.approval.decidedAt) }) : t('approvals.asked', { when: when(row.askedAt) }) }}</span>
        </template>
        <template #cell-decision="{ row }">
          {{ decision(row as ApprovalItem) }}
          <span v-if="row.approval.reason" class="text-base-content/70">· “{{ row.approval.reason }}”</span>
        </template>
      </DataTable>
      <AppButton v-if="store.more[tab]" variant="outline" size="sm" class="self-start" :disabled="store.loading" @click="store.loadMore(tab)">
        {{ t('approvals.load_more') }}
      </AppButton>
    </template>
  </div>
</template>
