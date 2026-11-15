<script setup lang="ts">
import CopyButton from '#base/components/ui/CopyButton.vue';
import KeyValue from '#base/components/ui/KeyValue.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import { useAgentPage } from '~/components/agent/useAgentPage';
import CodeText from '~/components/layout/CodeText.vue';

const { t } = useI18n();
const route = useRoute();
const { agent } = await useAgentPage();
const { apiUrl } = useRuntimeConfig().public;
const endpoint = computed(() => new URL(`${(apiUrl || '/api').replace(/\/$/, '')}/v1/agents/${agent.value.id}/runs`, window.location.origin).href);
const curl = computed(() => [
  `curl -N ${endpoint.value} \\`,
  '  -H "Authorization: Bearer $OAGENT_KEY" \\',
  '  -H "Content-Type: application/json" \\',
  '  -d \'{"threadId":"rumah-001","user":"cust_0001","messages":[{"role":"user","content":"beli kopi 40rb pakai cash"}]}\'',
]);

useHead({ title: () => `${agent.value?.name ?? ''} · ${t('channels.api')} · OAgent` });
</script>

<template>
  <div class="flex max-w-3xl flex-col gap-4">
    <PageHeader :title="t('channels.api')" :description="t('apiaccess.subtitle')" />
    <Section :title="t('apiaccess.title')">
      <div class="-mx-4 border-y border-base-300">
        <KeyValue :items="[{ label: t('apiaccess.endpoint'), value: `POST ${endpoint}`, mono: true }, { label: t('apiaccess.agent_id'), value: agent.id, mono: true, copy: true }]" :copy-label="t('common.copy_label', { label: '{label}' })">
          <template #value-0>
            <span class="flex min-w-0 flex-col">
              <span class="flex items-center gap-2"><span class="font-mono text-xs break-all">POST {{ endpoint }}</span><CopyButton :value="endpoint" :label="t('common.copy_label', { label: t('apiaccess.endpoint') })" /></span>
            </span>
          </template>
        </KeyValue>
      </div>
      <p class="text-ui">
        <CodeText :text="t('apiaccess.keys')" /> <NuxtLink :to="`/o/${route.params.org}/settings/api-keys`" class="link">
          {{ t('apiaccess.create_key') }}
        </NuxtLink>
      </p>
      <div class="flex flex-col gap-1">
        <div class="flex items-center justify-between">
          <p class="text-xs text-base-content/70">
            {{ t('apiaccess.example') }}
          </p>
          <CopyButton :value="curl.join('\n')" :label="t('common.copy')" />
        </div>
        <div class="mockup-code w-full text-xs" tabindex="0" role="region" :aria-label="t('apiaccess.example')">
          <pre v-for="(line, i) in curl" :key="i" :data-prefix="i === 0 ? '$' : ' '"><code>{{ line }}</code></pre>
        </div>
      </div>
      <p class="text-xs text-base-content/70">
        {{ t('apiaccess.stream_note') }}
      </p>
    </Section>
  </div>
</template>
