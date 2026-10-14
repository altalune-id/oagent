<script setup lang="ts">
import scopes from '@shared/scope/scopes.json';
import AppButton from '~/components/ui/AppButton.vue';
import AppModal from '~/components/ui/AppModal.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import FieldError from '~/components/ui/FieldError.vue';
import SecretField from '~/components/ui/SecretField.vue';
import { useScopeLabel } from '~/composables/useScopeLabel';
import { useProjectsStore } from '~/stores/projects';
import { useAPIKeyCreate } from './useAPIKeyCreate';

const { orgId } = defineProps<{ orgId: string }>();

const { t } = useI18n();
const scopeLabel = useScopeLabel();
const projects = useProjectsStore();
const create = useAPIKeyCreate(orgId);
const { open, kind, name, scopes: selectedScopes, allProjects, projectId, projectIds, expiresInHours, busy, error, fieldErrors, plaintext } = create;

const availableScopes = computed(() => scopes.filter(s => !s.personOnly && (kind.value === 'org' || s.level === 'project')));
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
    <Icon name="lucide:plus" size="16" />
    {{ t('apikeys.new') }}
  </AppButton>

  <AppModal :open="open" :title="t('apikeys.new')" @close="create.hide()">
    <div v-if="plaintext" class="space-y-3">
      <p class="text-sm text-warning">
        {{ t('apikeys.secret_warning') }}
      </p>
      <SecretField :value="plaintext" :label="t('apikeys.copy')" />
    </div>

    <form v-else novalidate class="space-y-3" @submit.prevent="create.submit()">
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.name') }}</span>
        <input
          v-model="name" type="text" class="input input-sm input-bordered w-full" :class="{ 'input-error': fieldErrors.name }"
          placeholder="deploy" :disabled="busy" :aria-invalid="!!fieldErrors.name" aria-describedby="apikey-name-error"
        >
        <FieldError id="apikey-name-error" :message="fieldErrors.name" />
      </label>

      <div class="flex gap-4">
        <label class="flex items-center gap-2 text-sm"><input v-model="kind" type="radio" value="org" class="radio radio-sm" :disabled="busy">{{ t('apikeys.kind_org') }}</label>
        <label class="flex items-center gap-2 text-sm"><input v-model="kind" type="radio" value="project" class="radio radio-sm" :disabled="busy">{{ t('apikeys.kind_project') }}</label>
      </div>

      <label v-if="kind === 'project'" class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('apikeys.project') }}</span>
        <select v-model="projectId" class="select select-sm select-bordered w-full" :disabled="busy">
          <option value="" disabled>
            {{ t('apikeys.select_project') }}
          </option>
          <option v-for="p in orgProjects" :key="p.id" :value="p.id">
            {{ p.name }}
          </option>
        </select>
      </label>

      <div v-else class="space-y-2">
        <label class="flex items-center gap-2 text-sm"><input v-model="allProjects" type="checkbox" class="checkbox checkbox-sm" :disabled="busy">{{ t('apikeys.all_projects') }}</label>
        <div v-if="!allProjects" class="max-h-32 space-y-1 overflow-auto rounded-box border border-base-300 p-2">
          <label v-for="p in orgProjects" :key="p.id" class="flex items-center gap-2 text-sm">
            <input v-model="projectIds" type="checkbox" :value="p.id" class="checkbox checkbox-xs" :disabled="busy">{{ p.name }}
          </label>
        </div>
      </div>

      <div>
        <span class="mb-1 block text-xs text-base-content/60">{{ t('apikeys.scopes') }}</span>
        <div class="grid grid-cols-2 gap-1">
          <label v-for="s in availableScopes" :key="s.name" class="flex items-center gap-2 text-sm">
            <input v-model="selectedScopes" type="checkbox" :value="s.name" class="checkbox checkbox-xs" :disabled="busy">{{ scopeLabel(s.name) }}
          </label>
        </div>
      </div>

      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('apikeys.expires') }}</span>
        <select v-model.number="expiresInHours" class="select select-sm select-bordered w-full" :disabled="busy">
          <option v-for="e in expiryOptions" :key="e.value" :value="e.value">
            {{ e.label }}
          </option>
        </select>
      </label>

      <ErrorText :error="error" />
    </form>

    <template #footer>
      <button class="btn btn-ghost btn-sm" type="button" :disabled="busy" @click="create.hide()">
        {{ plaintext ? t('apikeys.done') : t('common.cancel') }}
      </button>
      <button v-if="!plaintext" class="btn btn-primary btn-sm" type="button" :disabled="busy || !name.trim()" @click="create.submit()">
        {{ busy ? t('common.creating') : t('common.create') }}
      </button>
    </template>
  </AppModal>
</template>
