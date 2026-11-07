<script setup lang="ts">
import type { EndUserThread } from '~/stores/endUsers';
import AppButton from '#base/components/ui/AppButton.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import KeyValue from '#base/components/ui/KeyValue.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import LoadingStatus from '#base/components/ui/LoadingStatus.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import { useDates } from '#base/composables/useDates';
import { identityKindLabel, personLabel } from '~/components/enduser/label';
import { useProject } from '~/components/project/useProject';
import { threadKind } from '~/components/thread/fromWire';
import { useAgentsStore } from '~/stores/agents';
import { useEndUsersStore } from '~/stores/endUsers';

const { t } = useI18n();
const route = useRoute();
const { when, moment } = useDates();
const { project } = useProject();
const store = useEndUsersStore();
const agents = useAgentsStore();
void agents.ensureLoaded(project.value!.id);

const id = computed(() => route.params.endUser as string);
watch(id, next => void store.loadOne(next), { immediate: true });

const detail = computed(() => (store.detail?.user.id === id.value ? store.detail : null));
const name = computed(() => (detail.value ? personLabel(detail.value.user, t) : ''));
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}`);
const list = computed(() => `${base.value}/end-users`);
const facts = computed(() => {
  const u = detail.value?.user;
  if (!u) {
    return [];
  }
  return [
    { label: t('endusers.identity'), value: `${identityKindLabel(u.identityKind, t)}${u.identityHint ? ` …${u.identityHint}` : ''}` },
    { label: t('endusers.first_seen'), value: u.firstSeenAt ? moment(u.firstSeenAt) : '—' },
    { label: t('endusers.last_seen'), value: u.lastSeenAt ? moment(u.lastSeenAt) : '—' },
  ];
});
const agentOf = (th: EndUserThread) => agents.items.find(a => a.id === th.agentId);

useHead({ title: () => `${name.value || t('endusers.title')} · ${t('endusers.title')} · OAgent` });
</script>

<template>
  <div v-if="store.detailError">
    <h1 class="sr-only">
      {{ t('endusers.title') }}
    </h1>
    <LoadError :error="store.detailError" :busy="store.detailLoading" :retry-label="t('common.retry')" @retry="store.loadOne(id)" />
  </div>
  <LoadingStatus v-else-if="store.detailLoading && !detail" :label="t('common.loading')" />
  <EmptyState v-else-if="!detail" icon="lucide:users-round" :title="t('error.end_user_not_found')" :heading-level="1">
    <template #actions>
      <AppButton variant="outline" size="sm" :to="list">
        {{ t('endusers.title') }}
      </AppButton>
    </template>
  </EmptyState>
  <div v-else class="flex flex-col gap-4">
    <PageHeader :title="name" />

    <Section :title="t('endusers.about')" :description="t('endusers.identities_desc')" flush>
      <KeyValue :items="facts" :copy-label="t('common.copy_label', { label: '{label}' })" />
    </Section>

    <Section :title="t('endusers.threads')" flush>
      <p v-if="!detail.threads.length" class="px-4 py-3 text-ui text-base-content/70">
        {{ t('endusers.threads_empty') }}
      </p>
      <ul v-else class="divide-y divide-base-300">
        <li v-for="th in detail.threads" :key="th.id" class="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2 text-ui">
          <NuxtLink v-if="agentOf(th)" :to="`${base}/agents/${agentOf(th)!.slug}/threads/${th.id}`" class="link link-hover font-mono text-xs">
            {{ th.id }}
          </NuxtLink>
          <span v-else class="font-mono text-xs">{{ th.id }}</span>
          <span class="badge badge-sm badge-ghost">{{ t(`threads.kind_${threadKind(th.kind)}`) }}</span>
          <span class="text-base-content/70">{{ agentOf(th)?.name ?? t('endusers.agent_gone') }}<template v-if="th.lastActivityAt"> · {{ when(th.lastActivityAt) }}</template></span>
        </li>
      </ul>
    </Section>
  </div>
</template>
