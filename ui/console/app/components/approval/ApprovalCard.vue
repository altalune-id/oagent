<script setup lang="ts">
import type { ApprovalRequest } from '~/stores/approvals';
import AppButton from '#base/components/ui/AppButton.vue';
import { shownName } from '~/components/enduser/forget';
import { chatTime } from '~/components/try/chatTime';
import { useNow } from '~/composables/useNow';
import { decisionLabel, statusAt } from './decision';

const { approval, readOnly = false, busy = false } = defineProps<{ approval: ApprovalRequest, readOnly?: boolean, busy?: boolean }>();
const emit = defineEmits<{ approve: [], reject: [reason: string] }>();

const { t, locale } = useI18n();
const uid = useId();
const rejecting = ref(false);
const reason = ref('');
const acted = ref(false);
const reasonInput = ref<HTMLInputElement>();
const rejectButton = ref<{ $el: HTMLElement }>();
const decisionText = ref<HTMLElement>();
const shownArgs = computed(() => approval.args.filter(a => !a.gate));
const shortTool = computed(() => approval.tool.split('__').at(-1) ?? approval.tool);
const raw = computed(() => JSON.stringify(approval.raw, null, 2));

const now = useNow();
const status = computed(() => statusAt(approval, now.value));
const decision = computed(() => {
  if (status.value === 'executing') {
    return t('approvalcard.executing');
  }
  if (status.value === 'pending') {
    return readOnly && (approval.requestedBy || approval.requesterDeleted) ? t('approvalcard.waiting_for', { name: shownName(approval.requestedBy, approval.requesterDeleted, t) }) : '';
  }
  return decisionLabel(approval, t, d => chatTime(d, locale.value), now.value);
});

watch(() => approval.status, (status) => {
  if (acted.value && status !== 'pending') {
    void nextTick(() => decisionText.value?.focus());
  }
});

function startReject() {
  rejecting.value = true;
  void nextTick(() => reasonInput.value?.focus());
}

function cancelReject() {
  rejecting.value = false;
  void nextTick(() => rejectButton.value?.$el.focus());
}

function approve() {
  acted.value = true;
  emit('approve');
}

function reject() {
  acted.value = true;
  emit('reject', reason.value);
  rejecting.value = false;
}
</script>

<template>
  <section class="flex flex-col gap-3 rounded-box border bg-base-100 p-3 text-ui" :class="status === 'pending' ? 'border-warning' : 'border-base-300'" :aria-labelledby="`${uid}-title`">
    <div class="flex flex-wrap items-start justify-between gap-2">
      <h3 :id="`${uid}-title`" class="flex items-center gap-1.5 font-semibold">
        <Icon name="lucide:hand" size="16" class="shrink-0" aria-hidden="true" />
        {{ approval.title }}
      </h3>
      <span class="badge badge-sm badge-ghost font-mono">{{ approval.tool }}</span>
    </div>
    <dl :id="`${uid}-args`" class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1">
      <template v-for="arg in shownArgs" :key="arg.label">
        <dt class="text-base-content/70" :class="{ 'font-mono text-xs': arg.mono }">
          {{ arg.label }}
        </dt>
        <dd class="min-w-0 break-words" :class="{ 'font-mono text-xs': arg.mono }">
          {{ arg.value }}
        </dd>
      </template>
    </dl>
    <details class="collapse-arrow collapse rounded-box border border-base-300">
      <summary class="collapse-title min-h-0 py-2 text-xs">
        {{ t('approvalcard.raw') }}
      </summary>
      <pre class="collapse-content font-mono text-xs whitespace-pre-wrap wrap-anywhere">{{ raw }}</pre>
    </details>

    <template v-if="status === 'pending' && !readOnly">
      <div v-if="rejecting" class="flex flex-col gap-2">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ t('approvalcard.reason') }}</span>
          <input ref="reasonInput" v-model="reason" type="text" class="input input-sm w-full text-ui" :placeholder="t('approvalcard.reason_placeholder')">
        </label>
        <div class="flex flex-wrap gap-2">
          <AppButton variant="error" size="sm" soft :disabled="busy" @click="reject()">
            {{ t('approvalcard.reject_send') }}
          </AppButton>
          <AppButton variant="plain" size="sm" @click="cancelReject()">
            {{ t('common.cancel') }}
          </AppButton>
        </div>
      </div>
      <div v-else class="flex flex-wrap gap-2">
        <AppButton size="sm" :disabled="busy" :aria-describedby="`${uid}-args`" @click="approve()">
          <Icon name="lucide:check" size="16" aria-hidden="true" />
          {{ t('approvalcard.approve_run') }} <span class="font-mono text-xs">{{ shortTool }}</span>
        </AppButton>
        <AppButton ref="rejectButton" variant="plain" size="sm" :disabled="busy" :aria-describedby="`${uid}-args`" @click="startReject()">
          {{ t('approvalcard.reject') }}
        </AppButton>
      </div>
    </template>
    <p v-if="decision" ref="decisionText" tabindex="-1" class="flex flex-wrap items-center gap-x-2 text-ui text-base-content/70" role="status">
      <Icon :name="status === 'approved' ? 'lucide:circle-check' : status === 'rejected' ? 'lucide:circle-x' : 'lucide:clock'" size="16" class="shrink-0" aria-hidden="true" />
      <span>{{ decision }}</span>
      <span v-if="approval.reason">· “{{ approval.reason }}”</span>
    </p>
  </section>
</template>
