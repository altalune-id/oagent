<script setup lang="ts">
import type { ChatItem } from './useTryChat';
import AppButton from '#base/components/ui/AppButton.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import ApprovalCard from '~/components/approval/ApprovalCard.vue';
import { shownName } from '~/components/enduser/forget';
import { runDetailText, runFix, runStatusTone } from '~/components/thread/conversation';
import { useUnits } from '~/composables/useUnits';
import { chatTime, dayBreaks } from './chatTime';

const { items, agentName, youName, viewer = 'person', readOnly = false, busy = false, runBase = '' } = defineProps<{
  items: ChatItem[],
  agentName: string,
  youName: string,
  viewer?: 'person' | 'agent',
  readOnly?: boolean,
  busy?: boolean,
  runBase?: string,
}>();
const emit = defineEmits<{ approve: [], reject: [reason: string], retry: [], reload: [] }>();

const { t, te, locale } = useI18n();
const agentBase = computed(() => runBase.replace(/\/runs$/, ''));
const time = (d: Date) => chatTime(d, locale.value);
const days = computed(() => dayBreaks(items.map(i => ({ id: i.id, at: 'at' in i ? i.at : undefined })), locale.value, t('chat.today')));
const { cost, costTitle, seconds, count } = useUnits();
const own = 'bg-base-300';
const other = 'border border-base-300 bg-base-100';
const side = computed(() => (viewer === 'person'
  ? { user: 'chat-end', agent: 'chat-start', self: 'self-start', userBubble: own, agentBubble: other }
  : { user: 'chat-start', agent: 'chat-end', self: 'self-end', userBubble: other, agentBubble: own }));
const toolIcon = { running: '', done: 'lucide:check', preview: 'lucide:eye', failed: 'lucide:x', awaiting: 'lucide:hand', rejected: 'lucide:ban' } as const;

function pretty(value: unknown) {
  return JSON.stringify(value, null, 2);
}
</script>

<template>
  <div role="log" aria-live="polite" :aria-busy="items.some(i => i.kind === 'agent' && i.streaming) || undefined" class="flex flex-col gap-3">
    <template v-for="item in items" :key="item.id">
      <p v-if="days.has(item.id)" class="divider my-0 text-xs text-base-content/70">
        {{ days.get(item.id) }}
      </p>
      <div v-if="item.kind === 'user'" class="chat" :class="side.user">
        <div class="chat-header text-xs">
          {{ item.deleted ? shownName('', true, t) : item.speaker ?? youName }}
          <time v-if="item.at" :datetime="item.at.toISOString()" class="text-base-content/70">{{ time(item.at) }}</time>
          <span v-if="item.skipped" class="badge badge-ghost badge-sm text-base-content">{{ t(`thread.skipped_${item.skipped}`) }}</span>
        </div>
        <div class="chat-bubble text-base-content text-ui" :class="side.userBubble">
          <img v-if="item.photo" :src="item.photo.url" :alt="t('try.photo_alt')" class="mb-2 size-32 rounded-box object-cover">
          <span v-if="item.photoPlaceholder" class="mb-2 flex size-32 flex-col items-center justify-center gap-1 rounded-box border border-base-300 bg-base-100 text-xs text-base-content/70">
            <Icon name="lucide:receipt" size="20" aria-hidden="true" />
            {{ t('thread.photo_placeholder') }}
          </span>
          <span v-if="item.deleted" class="flex items-center gap-1.5 text-base-content/70 italic">
            <Icon name="lucide:ban" size="14" aria-hidden="true" />
            {{ t('chat.message_deleted') }}
          </span>
          <template v-else>
            {{ item.text }}
          </template>
        </div>
      </div>

      <div v-else-if="item.kind === 'agent'" class="chat" :class="side.agent">
        <div class="chat-header text-xs">
          {{ agentName }}
          <time v-if="item.at" :datetime="item.at.toISOString()" class="text-base-content/70">{{ time(item.at) }}</time>
        </div>
        <div v-if="item.failed" class="chat-bubble border border-error bg-base-100 text-base-content text-ui">
          <p class="flex items-start gap-1.5">
            <Icon name="lucide:circle-alert" size="16" class="mt-0.5 shrink-0 text-error" aria-hidden="true" />
            {{ t('try.model_failed') }}
          </p>
          <AppButton v-if="!readOnly" variant="plain" size="xs" class="mt-2" @click="emit('retry')">
            {{ t('common.retry') }}
          </AppButton>
        </div>
        <div v-else-if="item.streaming && !item.text" class="chat-bubble bg-base-100 border border-base-300 text-base-content text-ui" role="status">
          <span class="loading loading-dots loading-xs" aria-hidden="true" />
          {{ t('try.thinking') }}
        </div>
        <div v-else class="chat-bubble text-base-content text-ui whitespace-pre-wrap" :class="side.agentBubble" :aria-busy="item.streaming || undefined">
          {{ item.text }}
        </div>
      </div>

      <details v-else-if="item.kind === 'tool'" :class="side.self">
        <summary class="badge badge-sm badge-soft text-base-content cursor-pointer gap-1.5">
          <span v-if="item.state === 'running'" class="loading loading-spinner loading-xs" aria-hidden="true" />
          <Icon v-else :name="toolIcon[item.state]" size="12" :class="{ 'text-error': item.state === 'failed', 'text-success': item.state === 'done', 'text-warning': item.state === 'awaiting' }" aria-hidden="true" />
          <span class="font-mono">{{ item.name }}</span>{{ ' ' }}
          <span class="text-base-content/70">{{ t(`try.tool_${item.state}`) }}</span>
          <span class="sr-only">{{ t('try.tool_chip_hint') }}</span>
        </summary>
        <div class="mt-2 flex max-w-xl flex-col gap-2 rounded-box border border-base-300 bg-base-100 p-3 text-xs">
          <p class="font-semibold">
            {{ t('try.arguments') }}
          </p>
          <pre class="font-mono whitespace-pre-wrap wrap-anywhere">{{ pretty(item.args) }}</pre>
          <template v-if="!['running', 'awaiting', 'rejected'].includes(item.state)">
            <p class="font-semibold">
              {{ t('try.result') }}
            </p>
            <pre v-if="item.state !== 'failed'" class="font-mono whitespace-pre-wrap wrap-anywhere">{{ pretty(item.result) }}</pre>
            <p v-else>
              {{ t('try.tool_failed_detail') }}
            </p>
          </template>
        </div>
      </details>

      <div v-else-if="item.kind === 'run'" class="flex flex-col gap-1">
        <div class="flex items-center gap-3 text-xs">
          <span class="h-px flex-1 bg-base-300" aria-hidden="true" />
          <NuxtLink :to="`${runBase}/${item.run.id}`" class="link link-hover flex max-w-[85%] flex-wrap items-center justify-center gap-x-1.5 gap-y-0.5 text-center text-base-content/70">
            <span class="whitespace-nowrap">{{ t('thread.run') }} <span class="font-mono">{{ item.run.id }}</span></span>
            <span v-if="item.run.draft" class="badge badge-xs badge-ghost text-base-content">{{ t('run.draft') }}</span>
            <StatusLabel :tone="runStatusTone[item.run.status]" :label="t(`runstatus.${item.run.status}`)" class="text-xs whitespace-nowrap" />
            <span class="whitespace-nowrap">{{ seconds(item.run.seconds) }} · {{ t(item.run.estimated ? 'thread.tokens_estimated' : 'thread.tokens', { n: count(item.run.tokens) }) }} · <span v-if="item.run.unpriced">{{ t('thread.unpriced') }}</span><span v-else :title="costTitle(item.run.costCents)">{{ cost(item.run.costCents) }}</span></span>
          </NuxtLink>
          <span class="h-px flex-1 bg-base-300" aria-hidden="true" />
        </div>
        <p v-if="item.run.detail && item.run.status === 'failed'" class="text-center text-xs text-base-content/70">
          {{ runDetailText(item.run.detail, t, te) }}
          <template v-if="runBase && runFix(item.run.detail, agentBase)">
            ·
            <NuxtLink :to="runFix(item.run.detail, agentBase)!.to" class="link">
              {{ t(runFix(item.run.detail, agentBase)!.labelKey) }}
            </NuxtLink>
          </template>
        </p>
      </div>

      <div v-else-if="item.kind === 'approval'" class="w-full max-w-xl" :class="side.self">
        <ApprovalCard :approval="item.approval" :read-only="readOnly" :busy="busy" @approve="emit('approve')" @reject="emit('reject', $event)" />
      </div>

      <div v-else-if="item.kind === 'notice' && item.key === 'try.dropped'" role="note" class="flex flex-wrap items-center gap-2 text-ui text-base-content/70">
        <Icon name="lucide:wifi-off" size="16" class="shrink-0" aria-hidden="true" />
        {{ t('try.dropped') }}
        <AppButton v-if="!readOnly" variant="plain" size="xs" @click="emit('reload')">
          {{ t('try.reload') }}
        </AppButton>
      </div>
      <div v-else-if="item.kind === 'notice' && item.key.startsWith('try.limit')" role="alert" class="alert alert-info alert-soft text-base-content text-ui">
        <Icon name="lucide:octagon-pause" size="16" class="shrink-0" aria-hidden="true" />
        <span>{{ t(item.key, item.params ?? {}) }}</span>
      </div>
      <p v-else-if="item.kind === 'notice'" role="note" class="text-center text-xs text-base-content/70">
        {{ t(item.key, item.params ?? {}) }}
      </p>
    </template>
  </div>
</template>
