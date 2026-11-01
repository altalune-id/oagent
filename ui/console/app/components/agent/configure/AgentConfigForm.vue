<script setup lang="ts">
import type { AgentConfig, ToolInfo } from '~/preview/samples';
import FormField from '#base/components/ui/FormField.vue';
import Section from '#base/components/ui/Section.vue';

const { models, tools, disabled = false } = defineProps<{
  models: string[],
  tools: ToolInfo[],
  disabled?: boolean,
}>();

const config = defineModel<AgentConfig>({ required: true });
const { t } = useI18n();
const ids = { temperature: useId() };

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
  <div class="space-y-6">
    <Section :title="t('configure.instructions')" :description="t('configure.instructions_hint')">
      <FormField v-slot="{ id, describedBy }" :label="t('configure.instructions')">
        <textarea
          :id="id" v-model="config.instructions" rows="8" maxlength="32000" class="textarea w-full font-mono text-sm"
          :aria-describedby="describedBy" :disabled="disabled"
        />
      </FormField>
    </Section>

    <Section :title="t('configure.model')">
      <div class="grid gap-4 sm:grid-cols-2">
        <FormField v-slot="{ id }" :label="t('configure.model')">
          <select :id="id" v-model="config.model" class="select w-full font-mono" :disabled="disabled">
            <option v-for="m in models" :key="m" :value="m">
              {{ m }}
            </option>
          </select>
        </FormField>
        <div class="fieldset">
          <label :for="ids.temperature" class="fieldset-legend">
            {{ t('configure.temperature') }}
            <span class="font-mono font-normal">{{ config.temperature.toFixed(1) }}</span>
          </label>
          <input
            :id="ids.temperature" v-model.number="config.temperature" type="range" min="0" max="2" step="0.1" class="range range-sm range-primary w-full"
            :aria-valuetext="config.temperature.toFixed(1)" :disabled="disabled"
          >
          <p class="label whitespace-normal text-base-content/70">
            {{ t('configure.temperature_hint') }}
          </p>
        </div>
      </div>
    </Section>

    <Section :title="t('configure.tools')" :description="t('configure.tools_hint')">
      <ul class="list rounded-box border border-base-300">
        <li v-for="tool in tools" :key="tool.name" class="list-row items-center">
          <input
            type="checkbox" class="toggle toggle-sm toggle-primary" :checked="!!toolOf(tool.name)" :disabled="disabled"
            :aria-label="t('configure.tool_enable', { tool: tool.name })" @change="setEnabled(tool.name, ($event.target as HTMLInputElement).checked)"
          >
          <div class="min-w-0">
            <p class="font-mono text-sm break-all">
              {{ tool.name }}
            </p>
            <p class="text-xs text-base-content/70">
              {{ tool.description }}
            </p>
          </div>
          <label class="label gap-2 text-sm text-base-content">
            <input
              type="checkbox" class="checkbox checkbox-sm rounded-sm" :checked="!!toolOf(tool.name)?.requireApproval" :disabled="disabled || !toolOf(tool.name)"
              @change="setApproval(tool.name, ($event.target as HTMLInputElement).checked)"
            >
            {{ t('configure.needs_approval') }}
          </label>
        </li>
      </ul>
    </Section>

    <Section :title="t('configure.limits')">
      <div class="grid gap-4 sm:grid-cols-2">
        <FormField v-slot="{ id, describedBy }" :label="t('configure.max_steps')" :hint="t('configure.max_steps_hint')">
          <input :id="id" v-model.number="config.maxSteps" type="number" min="0" max="100" class="input w-full" :aria-describedby="describedBy" :disabled="disabled">
        </FormField>
        <FormField v-slot="{ id, describedBy }" :label="t('configure.budget')" :hint="t('configure.budget_hint')">
          <label class="input w-full">
            <span aria-hidden="true">$</span>
            <input :id="id" v-model.number="budget" type="number" min="0" max="10000" step="0.01" class="grow" :aria-describedby="describedBy" :disabled="disabled">
          </label>
        </FormField>
      </div>
    </Section>
  </div>
</template>
