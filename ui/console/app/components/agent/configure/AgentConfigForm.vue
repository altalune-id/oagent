<script setup lang="ts">
import type { AgentConfig } from './agentConfig';
import type { ToolInfo } from '~/preview/samples';
import FormField from '#base/components/ui/FormField.vue';

const { models, tools, disabled = false } = defineProps<{
  models: string[],
  tools: ToolInfo[],
  disabled?: boolean,
}>();

const config = defineModel<AgentConfig>({ required: true });
const { t } = useI18n();
const ids = { instructions: useId(), temperature: useId(), tools: useId() };

function toolOf(name: string) {
  return config.value.tools.find(tool => tool.name === name);
}

function setEnabled(name: string, on: boolean) {
  const rest = config.value.tools.filter(tool => tool.name !== name);
  config.value = { ...config.value, tools: on ? [...rest, { name, requireApproval: false }] : rest };
}

function setApproval(name: string, on: boolean) {
  config.value = { ...config.value, tools: config.value.tools.map(tool => (tool.name === name ? { ...tool, requireApproval: on } : tool)) };
}

const budget = computed({
  get: () => config.value.maxCostCents / 100,
  set: (dollars: number) => {
    config.value = { ...config.value, maxCostCents: Math.round((Number(dollars) || 0) * 100) };
  },
});
</script>

<template>
  <form class="flex min-w-0 flex-col gap-5 p-4 lg:p-6" @submit.prevent>
    <div class="flex flex-col gap-1">
      <div class="flex items-center justify-between">
        <label :for="ids.instructions" class="label text-ui font-semibold text-base-content">{{ t('configure.instructions') }}</label>
        <span class="text-xs tabular-nums text-base-content/70">{{ config.instructions.length }} / 32000</span>
      </div>
      <textarea
        :id="ids.instructions" v-model="config.instructions" maxlength="32000" class="textarea h-40 w-full text-ui leading-relaxed"
        :disabled="disabled"
      />
    </div>

    <div class="grid gap-4 sm:grid-cols-2">
      <FormField v-slot="{ id }" :label="t('configure.model')">
        <select :id="id" v-model="config.model" class="select select-sm text-ui w-full font-mono" :disabled="disabled">
          <option value="" disabled>
            {{ t('configure.choose_model') }}
          </option>
          <option v-for="m in models" :key="m" :value="m">
            {{ m }}
          </option>
        </select>
      </FormField>
      <div class="flex flex-col gap-1">
        <div class="flex items-center justify-between">
          <label :for="ids.temperature" class="label text-ui font-semibold text-base-content">{{ t('configure.temperature') }}</label>
          <output :for="ids.temperature" class="text-xs tabular-nums">{{ config.temperature.toFixed(1) }}</output>
        </div>
        <input
          :id="ids.temperature" v-model.number="config.temperature" type="range" min="0" max="2" step="0.1" class="range range-xs w-full"
          :aria-valuetext="config.temperature.toFixed(1)" :aria-describedby="`${ids.temperature}-hint`" :disabled="disabled"
        >
        <p :id="`${ids.temperature}-hint`" class="label whitespace-normal text-xs text-base-content/70">
          {{ t('configure.temperature_hint') }}
        </p>
      </div>
    </div>

    <div class="flex flex-col gap-1">
      <h2 :id="ids.tools" class="text-ui font-semibold">
        {{ t('configure.tools') }}
      </h2>
      <ul class="divide-y divide-base-300 rounded-box border border-base-300 bg-base-100" :aria-labelledby="ids.tools">
        <li v-for="tool in tools" :key="tool.name" class="flex flex-wrap items-start gap-2 px-3 py-2 text-ui">
          <label class="flex min-w-0 flex-1 items-start gap-2 max-sm:basis-[calc(100%-2rem)]">
            <input
              type="checkbox" class="checkbox checkbox-sm mt-0.5 rounded-sm" :checked="!!toolOf(tool.name)" :disabled="disabled"
              :aria-label="t('configure.tool_enable', { tool: tool.name })" @change="setEnabled(tool.name, ($event.target as HTMLInputElement).checked)"
            >
            <span class="min-w-0 flex-1">
              <span class="font-mono text-xs font-medium break-all">{{ tool.name }}</span>
              <span class="block text-base-content/70">{{ tool.description }}</span>
            </span>
          </label>
          <label class="badge badge-sm shrink-0 cursor-pointer gap-1 max-sm:ms-8" :class="{ 'badge-outline': !toolOf(tool.name)?.requireApproval }">
            <input
              type="checkbox" class="checkbox checkbox-xs rounded-sm" :checked="!!toolOf(tool.name)?.requireApproval" :disabled="disabled || !toolOf(tool.name)"
              :aria-label="t('configure.tool_approval', { tool: tool.name })" @change="setApproval(tool.name, ($event.target as HTMLInputElement).checked)"
            >
            <Icon name="lucide:hand" size="12" class="shrink-0" aria-hidden="true" />
            {{ t('configure.needs_approval') }}
          </label>
        </li>
      </ul>
    </div>

    <div class="grid gap-4 sm:grid-cols-2">
      <FormField v-slot="{ id, describedBy }" :label="t('configure.max_steps')" :hint="t('configure.max_steps_hint')">
        <input :id="id" v-model.number="config.maxSteps" type="number" min="0" max="100" class="input input-sm text-ui w-full" :aria-describedby="describedBy" :disabled="disabled">
      </FormField>
      <FormField v-slot="{ id, describedBy }" :label="t('configure.budget')" :hint="t('configure.budget_hint')">
        <label class="input input-sm text-ui w-full">
          <span aria-hidden="true">$</span>
          <input :id="id" v-model.number="budget" type="number" min="0" max="10000" step="0.01" class="grow" :aria-describedby="describedBy" :disabled="disabled">
        </label>
      </FormField>
    </div>
  </form>
</template>
