<script setup lang="ts">
import AppButton from '#base/components/ui/AppButton.vue';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useConfirm } from '#base/composables/useConfirm';
import { useDates } from '#base/composables/useDates';
import PreviewNote from '~/components/layout/PreviewNote.vue';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useProject } from '~/components/project/useProject';
import { searchProviders, useWebSearchForm } from '~/components/websearch/useWebSearchForm';
import { useCan } from '~/composables/useCan';
import { describedBy } from '~/lib/names/names';
import { useAuthStore } from '~/stores/auth';
import { useWebSearchStore } from '~/stores/webSearch';

const { t } = useI18n();
const { date } = useDates();
const { project } = useProject();
const canEdit = useCan('project:write');
const store = useWebSearchStore();
const id = useId();

await store.ensureLoaded(project.value!.id);

const form = useWebSearchForm();
const { choice, key, replacing, error, current, keyShown } = form;
const currentLabel = computed(() => searchProviders.find(p => p.id === current.value)?.labelKey ?? '');
const removing = useConfirm<boolean>(async () => {
  store.remove();
}, 'error.remove_search_key');

useHead({ title: () => `${t('websearch.title')} · ${project.value?.name ?? ''} · OAgent` });

await useAuthStore().load();
</script>

<template>
  <div class="flex max-w-2xl flex-col gap-4">
    <PageHeader :title="t('websearch.title')" :description="t('websearch.subtitle')" :preview="t('preview.badge')">
      <template #status>
        <PreviewNote :text="t('preview.local_note')" />
      </template>
    </PageHeader>
    <ProjectReadOnlyNote v-if="!canEdit" :text="t('project.read_only')" />

    <LoadError v-if="store.error" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(project!.id)" />
    <template v-else>
      <Section :title="t('websearch.provider')">
        <p v-if="!current" class="text-ui text-base-content/70">
          {{ t('websearch.none') }}
        </p>
        <div v-else class="flex flex-wrap items-center gap-3">
          <StatusLabel tone="success" :label="t('websearch.key_set', { provider: t(currentLabel) })" :detail="store.setting.keyAddedAt ? t('websearch.added', { date: date(store.setting.keyAddedAt) }) : ''" />
          <template v-if="canEdit">
            <AppButton v-if="!replacing" variant="plain" size="xs" @click="replacing = true">
              {{ t('providers.replace_key') }}
            </AppButton>
            <AppButton variant="plain" size="xs" @click="removing.ask(true)">
              {{ t('websearch.remove') }}
            </AppButton>
          </template>
        </div>

        <form v-if="canEdit" novalidate class="flex flex-col gap-3" @submit.prevent="form.save()">
          <fieldset class="fieldset">
            <legend class="fieldset-legend text-ui">
              {{ t('websearch.choose') }}
            </legend>
            <div class="flex flex-col gap-2">
              <label v-for="p in searchProviders" :key="p.id" class="flex cursor-pointer items-start gap-3 rounded-box border border-base-300 p-3" :class="{ 'bg-base-200': choice === p.id }">
                <input v-model="choice" type="radio" :name="`${id}-provider`" :value="p.id" class="radio radio-sm mt-0.5">
                <span class="flex flex-col">
                  <span class="text-ui font-medium">{{ t(p.labelKey) }}<span v-if="p.id === current" class="text-base-content/70"> · {{ t('websearch.current') }}</span></span>
                  <span class="text-xs text-base-content/70">{{ t(p.hintKey) }}</span>
                </span>
              </label>
            </div>
          </fieldset>
          <template v-if="keyShown">
            <div class="flex flex-col gap-1">
              <label class="flex flex-col gap-1">
                <span class="label text-ui font-semibold text-base-content">{{ t('providers.api_key') }}</span>
                <input v-model="key" type="password" autocomplete="new-password" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': error }" :aria-invalid="!!error" :aria-describedby="describedBy(`${id}-hint`, !!error && `${id}-err`)">
              </label>
              <p :id="`${id}-hint`" class="label whitespace-normal text-xs text-base-content/70">
                {{ current && choice !== current ? t('websearch.switch_needs_key') : t('secret.write_only') }}
              </p>
              <FieldError :id="`${id}-err`" :message="error" />
            </div>
            <div class="flex items-center gap-2">
              <AppButton type="submit" size="sm">
                {{ t('websearch.save') }}
              </AppButton>
              <AppButton v-if="current" variant="plain" size="sm" @click="form.cancel()">
                {{ t('common.cancel') }}
              </AppButton>
            </div>
          </template>
        </form>
      </Section>

      <ConfirmModal
        :handle="removing"
        :title="t('websearch.remove_title')"
        :body="store.setting.usedBy.length ? t('websearch.remove_body_used', { agents: store.setting.usedBy.join(', ') }) : t('websearch.remove_body')"
        :confirm-label="t('websearch.remove')"
        :cancel-label="t('common.cancel')"
      />
    </template>
  </div>
</template>
