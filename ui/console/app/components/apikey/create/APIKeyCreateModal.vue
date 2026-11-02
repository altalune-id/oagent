<script setup lang="ts">
import scopes from '@shared/scope/scopes.json';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import SecretField from '#base/components/ui/SecretField.vue';
import { useScopeLabel } from '#base/composables/useScopeLabel';
import { useProjectsStore } from '~/stores/projects';
import { useAPIKeyCreate } from './useAPIKeyCreate';

const { orgId } = defineProps<{ orgId: string }>();

const { t } = useI18n();
const ids = { name: useId(), nameErr: useId(), project: useId(), expires: useId() };
const scopeLabel = useScopeLabel();
const projects = useProjectsStore();
const create = useAPIKeyCreate(orgId);
const { open, kind, name, scopes: selectedScopes, allProjects, projectId, projectIds, expiresInHours, busy, error, fieldErrors, plaintext } = create;

const availableScopes = computed(() => scopes.filter(s => !s.holder && (kind.value === 'org' || s.level === 'project')));
const orgProjects = computed(() => projects.forOrg(orgId));

const expiryOptions = computed(() => [
  { value: 168, label: t('apikeys.days_7') },
  { value: 720, label: t('apikeys.days_30') },
  { value: 2160, label: t('apikeys.days_90') },
  { value: 8760, label: t('apikeys.days_365') },
]);
</script>

<template>
  <AppButton size="sm" @click="create.show()">
    <Icon name="lucide:plus" size="16" class="shrink-0" aria-hidden="true" />
    {{ t('apikeys.new') }}
  </AppButton>

  <AppModal :open="open" :title="t('apikeys.new')" :dismissible="!plaintext" @close="create.hide()">
    <div v-if="plaintext" class="space-y-3">
      <p class="flex items-start gap-2 text-ui">
        <Icon name="lucide:triangle-alert" size="16" class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
        {{ t('apikeys.secret_warning') }}
      </p>
      <SecretField :value="plaintext" :label="t('apikeys.copy')" />
    </div>

    <form v-else novalidate class="space-y-3" @submit.prevent="create.submit()">
      <div class="fieldset">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ t('common.name') }}</span>
          <input
            :id="ids.name" v-model="name" type="text" class="input input-sm w-full text-ui" :class="{ 'input-error': fieldErrors.name }"
            placeholder="deploy" :disabled="busy" :aria-invalid="!!fieldErrors.name" :aria-describedby="ids.nameErr"
          >
        </label>
        <FieldError :id="ids.nameErr" :message="fieldErrors.name" />
      </div>

      <fieldset class="fieldset">
        <legend class="fieldset-legend text-ui">
          {{ t('apikeys.kind') }}
        </legend>
        <div class="flex gap-4">
          <label class="flex items-center gap-2 text-ui"><input v-model="kind" type="radio" value="org" class="radio radio-sm" :disabled="busy">{{ t('apikeys.kind_org') }}</label>
          <label class="flex items-center gap-2 text-ui"><input v-model="kind" type="radio" value="project" class="radio radio-sm" :disabled="busy">{{ t('apikeys.kind_project') }}</label>
        </div>
      </fieldset>

      <div v-if="kind === 'project'" class="fieldset">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ t('apikeys.project') }}</span>
          <select :id="ids.project" v-model="projectId" class="select select-sm w-full text-ui" :disabled="busy">
            <option value="" disabled>
              {{ t('apikeys.select_project') }}
            </option>
            <option v-for="p in orgProjects" :key="p.id" :value="p.id">
              {{ p.name }}
            </option>
          </select>
        </label>
      </div>

      <fieldset v-else class="fieldset">
        <legend class="fieldset-legend text-ui">
          {{ t('apikeys.projects') }}
        </legend>
        <label class="flex items-center gap-2 text-ui"><input v-model="allProjects" type="checkbox" class="checkbox checkbox-xs rounded-sm" :disabled="busy">{{ t('apikeys.all_projects') }}</label>
        <div v-if="!allProjects" class="max-h-32 space-y-1 overflow-auto rounded-box border border-base-300 p-2">
          <label v-for="p in orgProjects" :key="p.id" class="flex items-center gap-2 text-ui">
            <input v-model="projectIds" type="checkbox" :value="p.id" class="checkbox checkbox-xs rounded-sm" :disabled="busy">{{ p.name }}
          </label>
        </div>
      </fieldset>

      <fieldset class="fieldset">
        <legend class="fieldset-legend text-ui">
          {{ t('apikeys.scopes') }}
        </legend>
        <div class="grid grid-cols-2 gap-1">
          <label v-for="s in availableScopes" :key="s.name" class="flex items-center gap-2 text-ui">
            <input v-model="selectedScopes" type="checkbox" :value="s.name" class="checkbox checkbox-xs rounded-sm" :disabled="busy">{{ scopeLabel(s.name) }}
          </label>
        </div>
      </fieldset>

      <div class="fieldset">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ t('apikeys.expires') }}</span>
          <select :id="ids.expires" v-model.number="expiresInHours" class="select select-sm w-full text-ui" :disabled="busy">
            <option v-for="e in expiryOptions" :key="e.value" :value="e.value">
              {{ e.label }}
            </option>
          </select>
        </label>
      </div>

      <ErrorText :error="error" />
    </form>

    <template #footer>
      <AppButton variant="ghost" size="sm" :disabled="busy" @click="create.hide()">
        {{ plaintext ? t('apikeys.done') : t('common.cancel') }}
      </AppButton>
      <AppButton v-if="!plaintext" size="sm" :disabled="busy || !name.trim()" @click="create.submit()">
        {{ busy ? t('common.creating') : t('common.create') }}
      </AppButton>
    </template>
  </AppModal>
</template>
