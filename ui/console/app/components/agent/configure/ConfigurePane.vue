<script setup lang="ts">
import type { AgentConfig } from './agentConfig';
import CopyButton from '#base/components/ui/CopyButton.vue';
import { toWire } from './agentConfig';

const { agentId, config } = defineProps<{ agentId: string, config: AgentConfig }>();

const { t } = useI18n();
const tab = ref<'api' | 'mcp'>('api');
const name = useId();

const body = computed(() => ({ id: agentId, config: toWire(config) }));
const snippets = computed(() => ({
  api: `curl -X POST "$OAGENT_URL/api/agent.v1.AgentService/UpdateAgentDraft" \\\n  -H "Authorization: Bearer $OAGENT_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d '${JSON.stringify(body.value).replaceAll('\'', '\'\\\'\'')}'`,
  mcp: JSON.stringify({ tool: 'agent_update_draft', arguments: body.value }, null, 2),
}));
const lines = computed(() => ({
  api: snippets.value.api.split('\n').map((text, i) => ({ prefix: i === 0 ? '$' : ' ', text })),
  mcp: snippets.value.mcp.split('\n').map((text, i) => ({ prefix: String(i + 1), text })),
}));
</script>

<template>
  <aside class="min-w-0 border-t border-base-300 bg-base-200 xl:border-s xl:border-t-0" :aria-label="t('configure.pane')">
    <div class="tabs tabs-border tabs-sm grid-rows-[auto_1fr] px-2 pt-1 xl:sticky xl:top-0 xl:max-h-dvh xl:overflow-y-auto">
      <template v-for="kind in (['api', 'mcp'] as const)" :key="kind">
        <input v-model="tab" type="radio" :name="name" :value="kind" class="tab" :aria-label="t(`configure.${kind}_tab`)">
        <div class="tab-content min-w-0 p-2">
          <div class="flex items-center justify-between pb-2">
            <p class="text-xs text-base-content/70">
              {{ t(`configure.${kind}_title`) }}
            </p>
            <CopyButton :value="snippets[kind]" :label="t('common.copy')" />
          </div>
          <div class="mockup-code w-full text-xs" tabindex="0" role="region" :aria-label="t(`configure.${kind}_title`)">
            <pre v-for="(line, i) in lines[kind]" :key="i" :data-prefix="line.prefix"><code>{{ line.text }}</code></pre>
          </div>
        </div>
      </template>
    </div>
  </aside>
</template>
