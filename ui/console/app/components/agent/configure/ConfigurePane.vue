<script setup lang="ts">
import type { AgentConfig } from './agentConfig';
import AppButton from '#base/components/ui/AppButton.vue';
import CopyButton from '#base/components/ui/CopyButton.vue';
import EventMark from '~/components/run/EventMark.vue';
import { useUnits } from '~/composables/useUnits';
import { sampleTest } from '~/preview/samples';

const { agentId, config, runUrl } = defineProps<{ agentId: string, config: AgentConfig, runUrl: string }>();

const { t } = useI18n();
const { money } = useUnits();
const tab = ref<'test' | 'api' | 'mcp'>('test');
const name = useId();
const sample = sampleTest();

const body = computed(() => ({ id: agentId, config }));
const snippets = computed(() => ({
  api: `curl -X POST "$OAGENT_URL/agent.v1.AgentService/UpdateAgentDraft" \\\n  -H "Authorization: Bearer $OAGENT_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d '${JSON.stringify(body.value)}'`,
  mcp: JSON.stringify({ tool: 'agent_update_draft', arguments: body.value }, null, 2),
}));
const lines = computed(() => ({
  api: snippets.value.api.split('\n').map((text, i) => ({ prefix: i === 0 ? '$' : ' ', text })),
  mcp: snippets.value.mcp.split('\n').map((text, i) => ({ prefix: String(i + 1), text })),
}));
</script>

<template>
  <aside class="flex min-w-0 flex-col border-t border-base-300 bg-base-200 xl:sticky xl:top-0 xl:h-dvh xl:border-s xl:border-t-0" :aria-label="t('configure.pane')">
    <div class="tabs tabs-border tabs-sm min-h-0 flex-1 grid-rows-[auto_1fr] px-2 pt-1">
      <input v-model="tab" type="radio" :name="name" value="test" class="tab" :aria-label="t('configure.test_tab')">
      <div class="tab-content min-h-0 min-w-0 overflow-hidden">
        <div class="flex h-full min-h-96 flex-col gap-2 p-2">
          <div class="flex items-center justify-between text-xs text-base-content/70">
            <span>{{ t('configure.tests_draft') }}</span>
            <span class="badge badge-info badge-soft badge-sm">{{ t('preview.badge') }}</span>
          </div>
          <div class="min-h-0 flex-1 overflow-y-auto">
            <div class="chat chat-end">
              <div class="chat-bubble min-h-0 text-ui">
                {{ sample.message }}
              </div>
            </div>
            <div class="chat chat-start">
              <div class="chat-bubble min-h-0 bg-base-100 text-ui">
                {{ sample.reply }}
              </div>
              <div class="chat-footer flex flex-wrap items-center gap-2 pt-1 text-xs text-base-content/70">
                <EventMark v-for="(lane, i) in sample.lanes" :key="i" :lane="lane" />
                <span class="tabular-nums">{{ t('configure.test_summary', { steps: sample.steps, seconds: sample.seconds, cost: money(sample.costCents) }) }}</span>
                <NuxtLink class="link" :to="runUrl">
                  {{ t('configure.open_run') }}
                </NuxtLink>
              </div>
            </div>
          </div>
          <p class="text-xs text-base-content/70">
            {{ t('configure.test_hint') }}
          </p>
          <form class="join w-full" @submit.prevent>
            <label class="sr-only" :for="`${name}-msg`">{{ t('configure.test_message') }}</label>
            <input :id="`${name}-msg`" type="text" class="input input-sm text-ui join-item w-full" :placeholder="t('configure.test_message')" disabled>
            <AppButton type="submit" size="sm" class="join-item" disabled>
              {{ t('configure.send') }}
            </AppButton>
          </form>
        </div>
      </div>
      <template v-for="kind in (['api', 'mcp'] as const)" :key="kind">
        <input v-model="tab" type="radio" :name="name" :value="kind" class="tab" :aria-label="t(`configure.${kind}_tab`)">
        <div class="tab-content min-w-0 p-2">
          <div class="flex items-center justify-between pb-2">
            <p class="text-xs text-base-content/70">
              {{ t(`configure.${kind}_title`) }}
            </p>
            <CopyButton :value="snippets[kind]" :label="t('common.copy')" />
          </div>
          <div class="mockup-code w-full text-xs">
            <pre v-for="(line, i) in lines[kind]" :key="i" :data-prefix="line.prefix"><code>{{ line.text }}</code></pre>
          </div>
        </div>
      </template>
    </div>
  </aside>
</template>
