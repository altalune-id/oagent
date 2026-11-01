<script setup lang="ts">
import type { AgentConfig } from './agentConfig';
import CopyButton from '#base/components/ui/CopyButton.vue';
import Section from '#base/components/ui/Section.vue';

const { agentId, config } = defineProps<{ agentId: string, config: AgentConfig }>();

const { t } = useI18n();
const tab = ref<'api' | 'mcp'>('api');
const name = useId();

const body = computed(() => ({ id: agentId, config }));
const snippets = computed(() => ({
  api: `curl -X POST "$OAGENT_URL/agent.v1.AgentService/UpdateAgentDraft" \\\n  -H "Authorization: Bearer $OAGENT_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d '${JSON.stringify(body.value)}'`,
  mcp: JSON.stringify({ tool: 'agent_update_draft', arguments: body.value }, null, 2),
}));
</script>

<template>
  <Section :title="t('configure.api_title')" :description="t('configure.api_hint')">
    <div role="tablist" class="tabs tabs-border" :aria-label="t('configure.api_title')">
      <input v-model="tab" type="radio" :name="name" value="api" role="tab" class="tab" :aria-label="t('configure.api_tab')">
      <input v-model="tab" type="radio" :name="name" value="mcp" role="tab" class="tab" :aria-label="t('configure.mcp_tab')">
    </div>
    <div class="relative">
      <pre class="max-h-72 overflow-auto rounded-box bg-neutral p-4 pr-12 font-mono text-xs whitespace-pre-wrap break-words text-neutral-content"><code>{{ snippets[tab] }}</code></pre>
      <CopyButton :value="snippets[tab]" :label="t('common.copy')" class="absolute top-2 right-2 text-neutral-content" />
    </div>
  </Section>
</template>
