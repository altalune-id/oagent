<script setup lang="ts">
import type { EndUser, Memory } from '~/stores/endUsers';
import AppButton from '#base/components/ui/AppButton.vue';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import KeyValue from '#base/components/ui/KeyValue.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import { useToast } from '#base/components/ui/toast';
import { useConfirm } from '#base/composables/useConfirm';
import { useDates } from '#base/composables/useDates';
import { nameMatches } from '~/components/enduser/endUser';
import PreviewNote from '~/components/layout/PreviewNote.vue';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useProject } from '~/components/project/useProject';
import { useCan } from '~/composables/useCan';
import { useAuthStore } from '~/stores/auth';
import { useEndUsersStore } from '~/stores/endUsers';
import { useMcpServersStore } from '~/stores/mcpServers';

const { t } = useI18n();
const route = useRoute();
const toast = useToast();
const { when, moment } = useDates();
const { project } = useProject();
const canEdit = useCan('project:write');
const store = useEndUsersStore();
const servers = useMcpServersStore();
void servers.ensureLoaded(project.value!.id);
await store.ensureLoaded(project.value!.id);

const user = computed(() => store.byId(route.params.endUser as string));
if (!user.value && !store.error) {
  throw createError({ statusCode: 404, statusMessage: t('error.end_user_not_found'), fatal: true });
}

const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}`);
const list = computed(() => `${base.value}/end-users`);
const identities = computed(() => (user.value?.identities ?? []).map(i => ({ label: t(`endusers.kind_${i.kind}`), value: i.value, mono: i.kind === 'api' })));
const threadTitle = (th: EndUser['threads'][number]) => (th.kind === 'api' ? t('threads.api_user', { id: th.title }) : th.title);
const threadLink = (agentSlug: string, id: string) => `${base.value}/agents/${agentSlug}/threads/${id}`;
const threadOf = (m: Memory) => user.value?.threads.find(th => th.id === m.threadId);
const serverNames = computed(() => servers.items.map(s => s.name).join(', '));
const kept = computed(() => (serverNames.value ? t('endusers.delete_stays', { servers: serverNames.value }) : t('endusers.delete_stays_none')));
const confirmBody = computed(() => (serverNames.value ? t('endusers.delete_confirm_body', { servers: serverNames.value }) : t('endusers.delete_confirm_body_none')));

const forgetting = useConfirm<Memory>(async (m) => {
  store.forgetMemory(user.value!.id, m.id);
}, 'error.delete_memory');

const typed = ref('');
const deleting = useConfirm<EndUser>(async (u) => {
  await navigateTo(list.value);
  store.remove(u.id);
  toast.push('success', t('endusers.deleted', { name: u.name }));
}, 'error.delete_end_user');

function askDelete() {
  typed.value = '';
  deleting.ask(user.value!);
}

useHead({ title: () => `${user.value?.name ?? ''} · ${t('endusers.title')} · OAgent` });

await useAuthStore().load();
</script>

<template>
  <LoadError v-if="store.error" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(project!.id)" />
  <EmptyState v-else-if="!user" icon="lucide:users-round" :title="t('error.end_user_not_found')">
    <template #actions>
      <AppButton variant="outline" size="sm" :to="list">
        {{ t('endusers.title') }}
      </AppButton>
    </template>
  </EmptyState>
  <div v-else class="flex flex-col gap-4">
    <PageHeader :title="user.name" :preview="t('preview.badge')">
      <template #status>
        <PreviewNote :text="t('preview.local_note')" />
      </template>
    </PageHeader>

    <Section :title="t('endusers.identities')" :description="t('endusers.identities_desc')" flush>
      <KeyValue :items="identities" :copy-label="t('common.copy_label', { label: '{label}' })">
        <template v-for="(id, i) in user.identities" :key="`${id.kind}:${id.value}`" #[`value-${i}`]>
          <span :class="{ 'font-mono text-xs': id.kind === 'api' }">{{ id.value }}</span>
          <span class="text-base-content/70">{{ t('endusers.linked_since', { when: moment(id.linkedAt) }) }}</span>
        </template>
      </KeyValue>
    </Section>

    <Section :title="t('endusers.threads')" flush>
      <p v-if="!user.threads.length" class="px-4 py-3 text-ui text-base-content/70">
        {{ t('endusers.threads_empty') }}
      </p>
      <ul v-else class="divide-y divide-base-300">
        <li v-for="th in user.threads" :key="th.id" class="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2 text-ui">
          <NuxtLink :to="threadLink(th.agentSlug, th.id)" class="link link-hover font-medium">
            {{ threadTitle(th) }}
          </NuxtLink>
          <span class="badge badge-sm badge-ghost">{{ t(`threads.kind_${th.kind}`) }}</span>
          <span class="text-base-content/70">{{ th.agentName }} · {{ when(th.updatedAt) }}</span>
        </li>
      </ul>
    </Section>

    <Section :title="t('memories.title')" :description="t('memories.desc')" flush>
      <p v-if="!store.memoryOn" class="px-4 py-3 text-ui text-base-content/70">
        {{ t('memories.off') }}
      </p>
      <p v-else-if="!user.memories.length" class="px-4 py-3 text-ui text-base-content/70">
        {{ t('memories.none') }}
      </p>
      <ul v-else class="list">
        <li v-for="m in user.memories" :key="m.id" class="list-row items-center">
          <Icon name="lucide:brain" size="16" class="shrink-0 text-base-content/70" aria-hidden="true" />
          <div class="min-w-0 text-ui">
            <p>{{ m.text }}</p>
            <p class="text-xs text-base-content/70">
              {{ t('memories.learned', { when: moment(m.learnedAt) }) }}
              <template v-if="threadOf(m)">
                ·
                <NuxtLink :to="threadLink(m.agentSlug, m.threadId)" class="link link-hover">
                  {{ threadTitle(threadOf(m)!) }}
                </NuxtLink>
              </template>
            </p>
          </div>
          <AppButton v-if="canEdit" variant="ghost" size="sm" square :aria-label="t('memories.delete_label', { text: m.text })" @click="forgetting.ask(m)">
            <Icon name="lucide:trash-2" size="16" aria-hidden="true" />
          </AppButton>
        </li>
      </ul>
    </Section>

    <Section :title="t('endusers.delete_title')" tone="danger">
      <ul class="flex flex-col gap-1 text-ui">
        <li class="flex gap-2">
          <Icon name="lucide:trash-2" size="16" class="mt-0.5 shrink-0 text-error" aria-hidden="true" />
          {{ t('endusers.delete_goes') }}
        </li>
        <li class="flex gap-2">
          <Icon name="lucide:archive" size="16" class="mt-0.5 shrink-0 text-base-content/70" aria-hidden="true" />
          {{ kept }}
        </li>
      </ul>
      <div v-if="canEdit">
        <AppButton variant="error" size="sm" soft @click="askDelete()">
          {{ t('endusers.delete_button', { name: user.name }) }}
        </AppButton>
      </div>
      <ProjectReadOnlyNote v-else :text="t('endusers.read_only')" />
    </Section>

    <ConfirmModal
      :handle="forgetting"
      :title="t('memories.delete_title')"
      :body="t('memories.delete_body', { text: forgetting.target.value?.text ?? '' })"
      :confirm-label="t('common.delete')"
      :cancel-label="t('common.cancel')"
    />
    <ConfirmModal
      :handle="deleting"
      :title="t('endusers.delete_confirm_title', { name: user.name })"
      :body="confirmBody"
      :confirm-label="t('endusers.delete_button', { name: user.name })"
      :cancel-label="t('common.cancel')"
      :confirm-disabled="!nameMatches(typed, user.name)"
    >
      <label class="flex flex-col gap-1 text-ui">
        {{ t('endusers.type_name', { name: user.name }) }}
        <input v-model="typed" type="text" class="input input-sm w-full" autocomplete="off" spellcheck="false">
      </label>
    </ConfirmModal>
  </div>
</template>
