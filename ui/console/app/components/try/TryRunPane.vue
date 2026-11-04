<script setup lang="ts">
import type { ChatItem, Scenario } from './useTryChat';
import { scenarios } from './useTryChat';

const { stepsText, tokens, cost, model, toolCalls, busy = false } = defineProps<{
  stepsText: string,
  tokens: string,
  cost: string,
  model: string,
  toolCalls: Extract<ChatItem, { kind: 'tool' }>[],
  busy?: boolean,
}>();
const scenario = defineModel<Scenario>('scenario', { required: true });
const { t } = useI18n();
</script>

<template>
  <div class="flex flex-col gap-4">
    <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 text-ui">
      <dt class="text-base-content/70">
        {{ t('run.steps') }}
      </dt>
      <dd class="tabular-nums">
        {{ stepsText }}
      </dd>
      <dt class="text-base-content/70">
        {{ t('try.tokens') }}
      </dt>
      <dd class="tabular-nums">
        {{ tokens }}
      </dd>
      <dt class="text-base-content/70">
        {{ t('run.cost') }}
      </dt>
      <dd class="tabular-nums">
        {{ cost }}
      </dd>
      <dt class="text-base-content/70">
        {{ t('configure.model') }}
      </dt>
      <dd class="font-mono text-xs break-all">
        {{ model }}
      </dd>
    </dl>
    <label class="flex flex-col gap-1">
      <span class="label text-ui text-base-content">{{ t('try.scenario') }}</span>
      <select v-model="scenario" class="select select-sm w-full text-ui" :disabled="busy">
        <option v-for="s in scenarios" :key="s" :value="s">
          {{ t(`try.scenario_${s}`) }}
        </option>
      </select>
    </label>
    <p class="flex items-center gap-1.5 text-xs text-base-content/70">
      <Icon name="lucide:flask-conical" size="14" class="shrink-0" aria-hidden="true" />
      {{ t('try.preview_note') }}
    </p>
    <div class="flex flex-col gap-1">
      <h3 class="text-ui font-semibold">
        {{ t('try.tool_calls') }}
      </h3>
      <p v-if="!toolCalls.length" class="text-xs text-base-content/70">
        {{ t('try.no_tool_calls') }}
      </p>
      <div v-else class="max-w-full overflow-x-auto rounded-box border border-base-300 bg-base-100">
        <table class="table table-xs">
          <caption class="sr-only">
            {{ t('try.tool_calls') }}
          </caption>
          <thead class="text-base-content/70">
            <tr>
              <th scope="col" class="text-xs font-medium">
                {{ t('try.col_tool') }}
              </th>
              <th scope="col" class="text-xs font-medium">
                {{ t('run.status') }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="call in toolCalls" :key="call.id">
              <td class="font-mono">
                {{ call.name }}
              </td>
              <td>{{ t(`try.tool_${call.state}`) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
