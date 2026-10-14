<script setup lang="ts">
import type { Outcome, ToolCaller } from './invite';
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
  <main class="card">
    <header><span class="dot" /><strong>Invite a member</strong></header>

    <p v-if="!draft" class="error">
      This result has no draft invite.
    </p>
    <template v-else>
      <p>An assistant suggests inviting this person. Nothing is sent until you confirm.</p>
      <dl>
        <dt>Email</dt><dd>{{ draft.email }}</dd>
        <dt>Organization</dt><dd>{{ draft.orgName }}</dd>
        <dt>Role</dt><dd>{{ draft.role }}</dd>
      </dl>

      <div v-if="!done" class="actions">
        <button type="button" class="primary" :disabled="busy" @click="confirm">
          {{ busy ? "Working…" : "Confirm and send" }}
        </button>
        <button type="button" class="ghost" :disabled="busy" @click="discard">
          Discard
        </button>
      </div>

      <p v-if="outcome?.state === 'sent'">
        Invite sent to {{ outcome.email }}.
      </p>
      <p v-else-if="outcome?.state === 'discarded'" class="muted">
        Draft discarded. No invite was sent.
      </p>
      <p v-else-if="outcome?.state === 'expired'" class="error">
        {{ outcome.message }}
      </p>
      <p v-else-if="outcome?.state === 'error'" class="error">
        {{ outcome.message }}
      </p>
    </template>
  </main>
</template>

<style scoped>
.actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}
.primary {
  padding: 6px 14px;
  border: 0;
  border-radius: 6px;
  background: #3b82f6;
  color: #fff;
  font: inherit;
  cursor: pointer;
}
.primary:disabled {
  opacity: 0.6;
  cursor: default;
}
.actions .ghost {
  margin-top: 0;
}
</style>
