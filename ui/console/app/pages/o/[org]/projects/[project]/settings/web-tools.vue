<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { HttpSecret } from '~/stores/webTools';
import AppBadge from '#base/components/ui/AppBadge.vue';
import AppButton from '#base/components/ui/AppButton.vue';
import AppDropdown from '#base/components/ui/AppDropdown.vue';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useDates } from '#base/composables/useDates';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useProject } from '~/components/project/useProject';
import HttpSecretModal from '~/components/webtools/HttpSecretModal.vue';
import { useHttpSecretForm } from '~/components/webtools/useHttpSecretForm';
import { searchProviders, useSearchProviderForm } from '~/components/webtools/useSearchProviderForm';
import { useCan } from '~/composables/useCan';
import { useUnits } from '~/composables/useUnits';
import { focusIfLost, focusWhenShown } from '~/lib/focus/focus';
import { describedBy } from '~/lib/names/names';
import { useAgentsStore } from '~/stores/agents';
import { useAuthStore } from '~/stores/auth';
import { useWebToolsStore } from '~/stores/webTools';

const route = useRoute();
const { t } = useI18n();
const { date } = useDates();
const { project } = useProject();
const canEdit = useCan('provider:write');
const store = useWebToolsStore();
const agents = useAgentsStore();
const id = useId();
const projectId = project.value!.id;

await Promise.all([store.load(projectId), agents.ensureLoaded(projectId), useAuthStore().load()]);

const search = useSearchProviderForm(projectId);
const { kind, apiKey, price, replacing, editing, formShown, busy, error, errors, current, keyShown, removing } = search;
const secretForm = useHttpSecretForm(projectId);
const { deleting } = secretForm;

const currentLabel = computed(() => searchProviders.find(p => p.kind === current.value?.kind)?.labelKey ?? '');
const agentBase = `/o/${route.params.org}/projects/${route.params.project}/agents`;
const usedBy = computed(() => store.usedBy.map(u => ({ ...u, slug: agents.items.find(a => a.id === u.id)?.slug })));
const usedNames = computed(() => store.usedBy.map(u => u.name).join(', '));
const { money } = useUnits();
const summary = computed(() => (current.value ? t('webtools.summary', { provider: t(currentLabel.value), price: money(Number(current.value.pricePerCallMicros * 1000n) / 10_000) }) : ''));
const providerForm = ref<HTMLFormElement>();
const keyInput = ref<HTMLInputElement>();
const addSecretButton = ref<{ $el: HTMLElement }>();
const changeButton = ref<{ $el: HTMLElement }>();
const chosenRadio = () => providerForm.value?.querySelector<HTMLInputElement>('input[type="radio"]:checked') ?? providerForm.value?.querySelector<HTMLInputElement>('input[type="radio"]:not(:disabled)');

async function change() {
  editing.value = true;
  await nextTick();
  const radio = chosenRadio();
  if (radio) {
    focusWhenShown(radio);
  }
}

async function replaceKey() {
  replacing.value = true;
  await nextTick();
  if (keyInput.value) {
    focusWhenShown(keyInput.value);
  }
}

async function save() {
  if (await search.submit()) {
    await nextTick();
    focusIfLost(changeButton.value?.$el);
  }
}

const replacedRow = ref<HttpSecret | null>(null);

function replaceValue(row: HttpSecret) {
  replacedRow.value = row;
  secretForm.showReplace(row);
}

watch(secretForm.open, (open, wasOpen) => {
  const row = replacedRow.value;
  if (!wasOpen || open || !row) {
    return;
  }
  replacedRow.value = null;
  const label = t('webtools.actions_for', { host: row.host, header: row.header });
  setTimeout(() => {
    const triggers = [...document.querySelectorAll<HTMLButtonElement>('button[aria-haspopup="true"]')].filter(b => b.textContent?.includes(label));
    const trigger = triggers.find(b => b.getClientRects().length > 0) ?? triggers[0];
    if (trigger) {
      focusWhenShown(trigger);
    }
  });
});

async function cancel() {
  search.cancel();
  await nextTick();
  focusIfLost(changeButton.value?.$el);
}

watch(removing.target, (next, before) => {
  if (next === null && before !== null) {
    setTimeout(() => focusIfLost(chosenRadio()));
  }
});
watch(deleting.target, (next, before) => {
  if (next === null && before !== null) {
    setTimeout(() => focusIfLost(addSecretButton.value?.$el));
  }
});

const columns = computed<Column[]>(() => [
  { key: 'host', label: t('webtools.secret_host'), mono: true },
  { key: 'header', label: t('webtools.secret_header'), mono: true },
  { key: 'status', label: t('webtools.col_status') },
  ...(canEdit.value ? [{ key: 'menu', label: t('webtools.col_actions'), hideLabel: true, align: 'end' as const }] : []),
]);

useHead({ title: () => `${t('webtools.title')} · ${project.value?.name ?? ''} · OAgent` });
</script>

<template>
  <div class="flex max-w-3xl flex-col gap-4">
    <PageHeader :title="t('webtools.title')" :description="t('webtools.subtitle')" />
    <ProjectReadOnlyNote v-if="!canEdit" :text="t('project.read_only')" />

    <LoadError v-if="store.error" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(projectId)" />
    <template v-else>
      <Section :title="t('webtools.search_provider')" :description="t('webtools.search_provider_hint')">
        <div class="flex flex-col gap-4">
          <div v-if="!current" class="flex flex-wrap items-center gap-3">
            <div v-if="store.usedBy.length" role="status" class="alert alert-warning alert-soft w-full text-base-content text-ui">
              <Icon name="lucide:triangle-alert" size="16" class="shrink-0 text-warning" aria-hidden="true" />
              <span>{{ t('webtools.no_provider_used', { agents: usedNames }, store.usedBy.length) }}</span>
            </div>
            <p v-else class="text-ui text-base-content/70">
              {{ t('webtools.no_provider') }}
            </p>
            <AppButton v-if="canEdit && store.provider" variant="plain" size="xs" @click="removing.ask(true)">
              {{ t('webtools.remove') }}
            </AppButton>
          </div>
          <div v-else class="flex flex-wrap items-center gap-3">
            <StatusLabel tone="success" :label="t('webtools.key_set', { provider: t(currentLabel) })" :detail="current.updatedAt ? t('webtools.added', { date: date(current.updatedAt) }) : ''" />
            <span class="text-ui">{{ summary }}</span>
            <div v-if="canEdit" class="flex flex-wrap gap-2">
              <AppButton v-if="!formShown" ref="changeButton" variant="plain" size="xs" @click="change()">
                {{ t('webtools.change') }}
              </AppButton>
              <AppButton v-if="!replacing" variant="plain" size="xs" @click="replaceKey()">
                {{ t('providers.replace_key') }}
              </AppButton>
              <AppButton variant="plain" size="xs" @click="removing.ask(true)">
                {{ t('webtools.remove') }}
              </AppButton>
            </div>
          </div>

          <form v-if="canEdit && formShown" ref="providerForm" novalidate class="flex flex-col gap-3" @submit.prevent="save()">
            <fieldset class="fieldset">
              <legend class="fieldset-legend text-ui">
                {{ t('webtools.choose') }}
              </legend>
              <div class="grid gap-2 sm:grid-cols-3">
                <label
                  v-for="p in searchProviders" :key="p.kind"
                  class="flex items-start gap-3 rounded-box border border-base-300 p-3"
                  :class="search.selectable(p.kind) ? ['cursor-pointer', { 'bg-base-200': kind === p.kind }] : 'cursor-not-allowed opacity-60'"
                >
                  <input
                    v-model="kind" type="radio" :name="`${id}-provider`" :value="p.kind" class="radio radio-sm mt-0.5"
                    :disabled="!search.selectable(p.kind)" :aria-describedby="search.selectable(p.kind) ? `${id}-${p.kind}-hint` : `${id}-${p.kind}-soon`"
                  >
                  <span class="flex min-w-0 flex-col gap-1">
                    <span class="flex flex-wrap items-center gap-2 text-ui font-medium">
                      {{ t(p.labelKey) }}
                      <AppBadge v-if="!search.selectable(p.kind)" :id="`${id}-${p.kind}-soon`" size="xs" class="badge-ghost" :label="t('webtools.coming_soon')" />
                    </span>
                    <span :id="`${id}-${p.kind}-hint`" class="text-xs text-base-content/70">{{ t(p.hintKey) }}</span>
                  </span>
                </label>
              </div>
              <FieldError :id="`${id}-kind-err`" :message="errors.kind" />
            </fieldset>

            <div class="flex flex-col gap-1">
              <label class="flex flex-col gap-1">
                <span class="label text-ui font-semibold text-base-content">{{ t('webtools.price') }}</span>
                <input
                  v-model="price" type="text" inputmode="decimal" class="input input-sm max-w-40 tabular-nums" :class="{ 'input-error': errors.price_per_call_micros }"
                  :aria-invalid="!!errors.price_per_call_micros" :aria-describedby="describedBy(`${id}-price-hint`, !!errors.price_per_call_micros && `${id}-price-err`)"
                >
              </label>
              <p :id="`${id}-price-hint`" class="text-xs text-base-content/70">
                {{ t('webtools.price_hint') }}
              </p>
              <FieldError :id="`${id}-price-err`" :message="errors.price_per_call_micros" />
            </div>

            <div v-if="keyShown" class="flex flex-col gap-1">
              <label class="flex flex-col gap-1">
                <span class="label text-ui font-semibold text-base-content">{{ t('providers.api_key') }}</span>
                <input
                  ref="keyInput" v-model="apiKey" type="password" autocomplete="new-password" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.api_key }"
                  :aria-invalid="!!errors.api_key" :aria-describedby="describedBy(`${id}-key-hint`, !!errors.api_key && `${id}-key-err`)"
                >
              </label>
              <p :id="`${id}-key-hint`" class="text-xs text-base-content/70">
                {{ current && kind !== current.kind ? t('webtools.switch_needs_key') : t('secret.write_only') }} {{ t('webtools.key_server_only') }}
              </p>
              <FieldError :id="`${id}-key-err`" :message="errors.api_key" />
            </div>

            <ErrorText :error="error" />
            <div class="flex items-center gap-2">
              <AppButton type="submit" size="sm" :busy="busy">
                {{ t('webtools.save') }}
              </AppButton>
              <AppButton v-if="current" variant="plain" size="sm" @click="cancel()">
                {{ t('common.cancel') }}
              </AppButton>
            </div>
          </form>

          <div class="flex flex-col gap-1" data-test="used-by">
            <h3 class="text-ui font-semibold">
              {{ t('webtools.used_by') }}
            </h3>
            <ul v-if="usedBy.length" class="flex flex-wrap gap-x-3 gap-y-1 text-ui">
              <li v-for="u in usedBy" :key="u.id" class="min-w-0 break-all">
                <NuxtLink v-if="u.slug" :to="`${agentBase}/${u.slug}`" class="link inline-flex min-h-6 items-center">
                  {{ u.name }}
                </NuxtLink>
                <span v-else>{{ u.name }}</span>
              </li>
            </ul>
            <p v-else class="text-xs text-base-content/70">
              {{ t('webtools.used_by_none') }}
            </p>
          </div>
        </div>
      </Section>

      <Section :title="t('webtools.secrets')" :description="t('webtools.secrets_hint')">
        <template v-if="canEdit" #actions>
          <AppButton ref="addSecretButton" size="sm" @click="secretForm.showAdd()">
            {{ t('webtools.add_secret') }}
          </AppButton>
        </template>
        <DataTable stacked :caption="t('webtools.secrets')" row-key="id" :columns="columns" :rows="store.secrets">
          <template #empty>
            <EmptyState icon="lucide:key-round" :title="t('webtools.secrets_empty')" :body="t('webtools.secrets_empty_body')" :heading-level="3" />
          </template>
          <template #cell-host="{ row }">
            <span class="block min-w-0 break-all">{{ row.host }}</span>
          </template>
          <template #cell-header="{ row }">
            <span class="block min-w-0 break-all">{{ row.header }}</span>
          </template>
          <template #cell-status="{ row }">
            <StatusLabel tone="success" :label="t('webtools.secret_set')" :detail="row.updatedAt ? t('webtools.added', { date: date(row.updatedAt) }) : ''" />
          </template>
          <template #cell-menu="{ row }">
            <AppDropdown placement="end" size="xs" trigger-class="btn-square" content-class="w-48" :label="t('webtools.actions_for', { host: row.host, header: row.header })">
              <template #trigger>
                <Icon name="lucide:ellipsis" size="14" class="shrink-0" aria-hidden="true" />
              </template>
              <ul class="menu menu-sm w-full p-0 text-ui">
                <li>
                  <button type="button" @click="replaceValue(row as HttpSecret)">
                    <Icon name="lucide:key-round" size="16" class="shrink-0" aria-hidden="true" />
                    {{ t('webtools.replace_value') }}
                  </button>
                </li>
                <li>
                  <button type="button" @click="deleting.ask(row as HttpSecret)">
                    <Icon name="lucide:trash-2" size="16" class="shrink-0 text-error" aria-hidden="true" />
                    {{ t('common.delete') }}
                  </button>
                </li>
              </ul>
            </AppDropdown>
          </template>
        </DataTable>
      </Section>

      <template v-if="canEdit">
        <HttpSecretModal :form="secretForm" />
        <ConfirmModal
          :handle="removing"
          :title="t('webtools.remove_title')"
          :body="usedNames ? t('webtools.remove_body_used', { agents: usedNames }) : t('webtools.remove_body')"
          :confirm-label="t('webtools.remove')"
          :cancel-label="t('common.cancel')"
        />
        <ConfirmModal
          :handle="deleting"
          :title="t('webtools.delete_secret_title')"
          :body="t('webtools.delete_secret_body', { header: deleting.target.value?.header ?? '', host: deleting.target.value?.host ?? '' })"
          :confirm-label="t('common.delete')"
          :cancel-label="t('common.cancel')"
        />
      </template>
    </template>
  </div>
</template>
