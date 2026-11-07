<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { ApprovalItem } from '~/stores/approvals';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import { useToast } from '#base/components/ui/toast';
import { useDates } from '#base/composables/useDates';
import ApprovalCard from '~/components/approval/ApprovalCard.vue';
import { decisionLabel } from '~/components/approval/decision';
import { shownName } from '~/components/enduser/forget';
import PreviewNote from '~/components/layout/PreviewNote.vue';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useProject } from '~/components/project/useProject';
import { chatTime } from '~/components/try/chatTime';
import { useCan } from '~/composables/useCan';
import { useNow } from '~/composables/useNow';
import { useApprovalsStore } from '~/stores/approvals';
import { useAuthStore } from '~/stores/auth';

type Tab = 'pending' | 'decided' | 'expired';

const { t, locale } = useI18n();
const route = useRoute();
const { when, moment } = useDates();
const { project } = useProject();
const canDecide = useCan('agent:write');
const store = useApprovalsStore();
const auth = useAuthStore();
const toast = useToast();
const now = useNow();
const listId = useId();
if (project.value) {
  void store.ensureLoaded(project.value.id);
}

const tabs: Tab[] = ['pending', 'decided', 'expired'];
const tab = computed<Tab>(() => (tabs.includes(route.query.tab as Tab) ? route.query.tab as Tab : 'pending'));
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}`);
const byTab = computed<Record<Tab, ApprovalItem[]>>(() => ({ pending: store.pending, decided: store.decided, expired: store.expired }));
const me = computed(() => auth.me?.name ?? '');
const threadLink = (i: ApprovalItem) => `${base.value}/agents/${i.agentSlug}/threads/${i.threadId}`;

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

async function decide(i: ApprovalItem, approve: boolean, reason = '') {
  const at = byTab.value.pending.indexOf(i);
  const title = i.approval.title;
  const ok = await store.decide(i.approval.id, approve, me.value, reason);
  if (ok) {
    toast.push('success', t(approve ? 'approvals.approved_toast' : 'approvals.rejected_toast', { title }));
  } else {
    toast.push('error', t('approvals.expired_toast', { title }));
  }
  await nextTick();
  const left = byTab.value.pending;
  const next = left[Math.min(at, left.length - 1)];
  document.getElementById(next ? `${listId}-${next.approval.id}` : listId)?.focus();
}

useHead({ title: () => `${t('approvals.title')} · ${project.value?.name ?? ''} · OAgent` });

await auth.load();
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('approvals.title')" :description="t('approvals.subtitle')" :preview="t('preview.badge')">
      <template #status>
        <PreviewNote :text="t('preview.local_note')" />
      </template>
    </PageHeader>
    <ProjectReadOnlyNote v-if="!canDecide" :text="t('approvals.cant_decide')" />

    <LoadError v-if="store.error" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(project!.id)" />
    <template v-else>
      <nav class="tabs tabs-border tabs-sm" :aria-label="t('approvals.tabs')">
        <NuxtLink
          v-for="x in tabs" :key="x" :to="{ query: x === 'pending' ? {} : { tab: x } }" class="tab"
          :class="tab === x ? 'tab-active' : 'text-base-content/70'" :aria-current="tab === x ? 'page' : undefined"
        >
          {{ t(`approvals.tab_${x}`) }}
          <span v-if="x === 'pending'" class="badge badge-sm badge-ghost ms-1.5 tabular-nums">{{ byTab.pending.length }}</span>
        </NuxtLink>
      </nav>
      <h2 class="sr-only">
        {{ t(`approvals.tab_${tab}`) }}
      </h2>

      <template v-if="tab === 'pending'">
        <div :id="listId" tabindex="-1" class="focus:outline-none">
          <EmptyState v-if="!store.loading && !byTab.pending.length" icon="lucide:hand" :title="t('approvals.empty_pending')" />
          <ul v-else class="flex flex-col gap-2" :aria-label="t('approvals.tab_pending')">
            <li v-for="i in byTab.pending" :key="i.approval.id">
              <details class="group collapse-arrow collapse rounded-box border border-base-300 bg-base-100" :open="byTab.pending.length === 1">
                <summary :id="`${listId}-${i.approval.id}`" class="collapse-title flex flex-wrap items-center gap-x-4 gap-y-1 py-3 text-ui">
                  <span class="font-medium group-open:hidden">{{ i.approval.title }}</span>
                  <span class="font-mono text-xs group-open:hidden">{{ i.approval.tool }}</span>
                  <span class="text-base-content/70">{{ i.agentName }} · {{ shownName(i.approval.requestedBy, i.approval.requesterDeleted, t) }} · {{ t(`threads.kind_${i.channel}`) }}</span>
                  <span class="text-xs text-base-content/70">
                    {{ t('approvals.asked', { when: when(i.askedAt) }) }}<template v-if="i.approval.expiresAt"> · {{ t('approvals.expires', { when: moment(i.approval.expiresAt) }) }}</template>
                  </span>
                </summary>
                <div class="collapse-content flex flex-col gap-2">
                  <ApprovalCard class="max-w-xl" :approval="i.approval" :read-only="!canDecide" @approve="decide(i, true)" @reject="decide(i, false, $event)" />
                  <NuxtLink :to="threadLink(i)" class="link text-ui">
                    {{ t('approvals.open_thread') }}
                  </NuxtLink>
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
          <NuxtLink :to="threadLink(row as ApprovalItem)" class="link link-hover font-medium">
            {{ row.approval.title }}
          </NuxtLink>
        </template>
        <template #cell-tool="{ row }">
          {{ row.approval.tool }}
        </template>
        <template #cell-asked="{ row }">
          {{ shownName(row.approval.requestedBy, row.approval.requesterDeleted, t) }} · {{ t(`threads.kind_${row.channel}`) }}
        </template>
        <template #cell-time="{ row }">
          <span class="tabular-nums">{{ row.approval.decidedAt ? t('approvals.decided', { at: moment(row.approval.decidedAt) }) : t('approvals.asked', { when: when(row.askedAt) }) }}</span>
        </template>
        <template #cell-decision="{ row }">
          {{ decision(row as ApprovalItem) }}
          <span v-if="row.approval.reason" class="text-base-content/70">· “{{ row.approval.reason }}”</span>
        </template>
      </DataTable>
    </template>
  </div>
</template>
