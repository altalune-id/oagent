<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import { useId } from 'vue';
import ErrorText from '#base/components/ui/ErrorText.vue';

export interface PersonalKeyCreateLabels {
  name: string;
  expires: string;
  expiresHelp: string;
  projects: string;
  allProjects: string;
  selectProjects: string;
  scopes: string;
  scopesHelp: string;
  create: string;
  creating: string;
}

withDefaults(defineProps<{
  expiryOptions: { days: number, label: string }[],
  projects: { id: string, name: string }[],
  scopeOptions: { name: string, label: string }[],
  busy?: boolean,
  canSubmit?: boolean,
  error?: ShownError | null,
  labels?: PersonalKeyCreateLabels,
}>(), {
  busy: false,
  canSubmit: false,
  error: null,
  labels: () => ({
    name: 'Name',
    expires: 'Expires',
    expiresHelp: 'The key stops working on this date. You can\'t extend it; create a new key instead.',
    projects: 'Projects',
    allProjects: 'All projects',
    selectProjects: 'Only select projects',
    scopes: 'Scopes',
    scopesHelp: 'Pick only what the script or client needs. You can grant only what your role allows.',
    create: 'Create',
    creating: 'Creating…',
  }),
});

const emit = defineEmits<{ submit: [] }>();

const name = defineModel<string>('name', { default: '' });
const expiresInDays = defineModel<number>('expiresInDays', { default: 30 });
const allProjects = defineModel<boolean>('allProjects', { default: true });
const projectIds = defineModel<string[]>('projectIds', { default: () => [] });
const scopes = defineModel<string[]>('scopes', { default: () => [] });

const id = useId();
const ids = {
  name: `${id}-name`,
  expires: `${id}-expires`,
  expiresHelp: `${id}-expires-help`,
  scopesHelp: `${id}-scopes-help`,
};
</script>

<template>
  <form class="space-y-2 rounded-box border border-base-300 bg-base-100 p-4" @submit.prevent="emit('submit')">
    <div class="fieldset">
      <label :for="ids.name" class="fieldset-legend">{{ labels.name }}</label>
      <input :id="ids.name" v-model="name" type="text" class="input w-full" placeholder="laptop" maxlength="100" autocomplete="off" :disabled="busy">
    </div>

    <div class="fieldset">
      <label :for="ids.expires" class="fieldset-legend">{{ labels.expires }}</label>
      <select :id="ids.expires" v-model.number="expiresInDays" class="select w-full" :aria-describedby="ids.expiresHelp" :disabled="busy">
        <option v-for="o in expiryOptions" :key="o.days" :value="o.days">
          {{ o.label }}
        </option>
      </select>
      <p :id="ids.expiresHelp" class="label whitespace-normal">
        {{ labels.expiresHelp }}
      </p>
    </div>

    <fieldset class="fieldset">
      <legend class="fieldset-legend">
        {{ labels.projects }}
      </legend>
      <label class="flex items-center gap-2 text-sm">
        <input v-model="allProjects" type="radio" :value="true" class="radio radio-sm" :disabled="busy">
        {{ labels.allProjects }}
      </label>
      <label class="flex items-center gap-2 text-sm">
        <input v-model="allProjects" type="radio" :value="false" class="radio radio-sm" :disabled="busy">
        {{ labels.selectProjects }}
      </label>
      <div v-if="!allProjects" class="max-h-40 space-y-1 overflow-auto rounded-box border border-base-300 p-2">
        <label v-for="p in projects" :key="p.id" class="flex items-center gap-2 text-sm">
          <input v-model="projectIds" type="checkbox" :value="p.id" class="checkbox checkbox-sm rounded-sm" :disabled="busy">
          {{ p.name }}
        </label>
      </div>
    </fieldset>

    <fieldset class="fieldset" :aria-describedby="ids.scopesHelp">
      <legend class="fieldset-legend">
        {{ labels.scopes }}
      </legend>
      <div class="grid gap-1 sm:grid-cols-2">
        <label v-for="s in scopeOptions" :key="s.name" class="flex items-center gap-2 text-sm">
          <input v-model="scopes" type="checkbox" :value="s.name" class="checkbox checkbox-sm rounded-sm" :disabled="busy">
          {{ s.label }}
        </label>
      </div>
      <p :id="ids.scopesHelp" class="label whitespace-normal">
        {{ labels.scopesHelp }}
      </p>
    </fieldset>

    <ErrorText :error="error" />
    <div class="flex justify-end pt-2">
      <button type="submit" class="btn btn-primary btn-sm" :disabled="!canSubmit">
        <Icon name="lucide:plus" size="16" />
        {{ busy ? labels.creating : labels.create }}
      </button>
    </div>
  </form>
</template>
