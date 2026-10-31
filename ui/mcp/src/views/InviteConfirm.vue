<script setup lang="ts">
import type { Outcome, ToolCaller } from './invite';
import ErrorText from '#base/components/ui/ErrorText.vue';
import ViewShell from '../components/ViewShell.vue';
import { t } from '../copy';
import { confirmDraft, discardDraft, EXPIRED_MESSAGE, isExpired, parseDraft } from './invite';

const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<ToolCaller>('mcpApp');

const draft = computed(() => parseDraft(props.data));
const outcome = ref<Outcome | null>(null);
const busy = ref(false);

onMounted(() => {
  if (draft.value && isExpired(draft.value, new Date())) {
    outcome.value = { state: 'expired', message: EXPIRED_MESSAGE };
  }
});

async function run(action: (app: ToolCaller) => Promise<Outcome>) {
  if (!app || busy.value) {
    return;
  }
  busy.value = true;
  try {
    outcome.value = await action(app);
  } finally {
    busy.value = false;
  }
}

const confirm = () => draft.value && run(a => confirmDraft(a, draft.value!, new Date()));
const discard = () => draft.value && run(a => discardDraft(a, draft.value!));

const done = computed(() => outcome.value?.state === 'sent' || outcome.value?.state === 'discarded' || outcome.value?.state === 'expired');
</script>

<template>
  <ViewShell :title="t('mcp.invite_title')">
    <ErrorText v-if="!draft" :error="{ message: t('mcp.invite_missing') }" />
    <template v-else>
      <p>{{ t('mcp.invite_intro') }}</p>
      <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
        <dt class="text-base-content/60">
          {{ t('field.email') }}
        </dt>
        <dd class="font-mono break-all">
          {{ draft.email }}
        </dd>
        <dt class="text-base-content/60">
          {{ t('mcp.title_org') }}
        </dt>
        <dd class="break-all">
          {{ draft.orgName }}
        </dd>
        <dt class="text-base-content/60">
          {{ t('field.role') }}
        </dt>
        <dd>{{ draft.role }}</dd>
      </dl>

      <div v-if="!done" class="flex gap-2">
        <button type="button" class="btn btn-primary btn-sm" :disabled="busy" @click="confirm">
          <span v-if="busy" class="loading loading-spinner loading-xs" aria-hidden="true" />
          {{ t('mcp.invite_send') }}
        </button>
        <button type="button" class="btn btn-ghost btn-sm" :disabled="busy" @click="discard">
          {{ t('mcp.invite_discard') }}
        </button>
      </div>

      <p v-if="outcome?.state === 'sent'" role="status">
        {{ t('mcp.invite_sent_to', { email: outcome.email }) }}
      </p>
      <p v-else-if="outcome?.state === 'discarded'" role="status" class="text-sm text-base-content/60">
        {{ t('mcp.invite_discarded_body') }}
      </p>
      <ErrorText v-else-if="outcome?.state === 'expired' || outcome?.state === 'error'" :error="{ message: outcome.message }" />
    </template>
  </ViewShell>
</template>
