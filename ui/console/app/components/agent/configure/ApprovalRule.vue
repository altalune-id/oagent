<script setup lang="ts">
import type { AgentToolConfig } from './agentConfig';
import type { RuleMode, RuleRow } from './approvalRule';
import type { McpToolArg } from '~/stores/mcpServers';
import AppButton from '#base/components/ui/AppButton.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import { checkRow, maxConditions, modeOf, rowsFrom, scalarArgs, summaryOf, toConditions } from './approvalRule';

const { grant, args, serverName, trusted, disabled = false } = defineProps<{ grant: AgentToolConfig, args: McpToolArg[], serverName: string, trusted: boolean, disabled?: boolean }>();
const emit = defineEmits<{ change: [next: AgentToolConfig], refresh: [] }>();

const { t } = useI18n();
const uid = useId();
const modes: RuleMode[] = ['always', 'never', 'unless'];
const mode = ref<RuleMode>(modeOf(grant));
const rows = ref<RuleRow[]>(rowsFrom(grant.freeWhen));
const checks = computed(() => rows.value.map(r => checkRow(r, args)));
const blocked = computed(() => checks.value.some(c => c.error));
const summary = computed(() => (mode.value === 'unless' ? summaryOf(rows.value, t) : ''));
const pickable = computed(() => scalarArgs(args));

function commit() {
  if (mode.value === 'unless' && (!rows.value.length || blocked.value)) {
    return;
  }
  emit('change', { ...grant, requireApproval: mode.value !== 'never', freeWhen: mode.value === 'unless' ? toConditions(rows.value, args) : [] });
}

function setMode(next: RuleMode) {
  mode.value = next;
  if (next === 'unless' && !rows.value.length) {
    rows.value = [{ arg: pickable.value[0]?.name ?? '', text: '', absent: false }];
  }
  commit();
}

function update(i: number, patch: Partial<RuleRow>) {
  rows.value[i] = { ...rows.value[i]!, ...patch };
  commit();
}

function add() {
  if (rows.value.length < maxConditions) {
    rows.value.push({ arg: '', text: '', absent: false });
  }
}

function remove(i: number) {
  rows.value.splice(i, 1);
  if (!rows.value.length) {
    mode.value = 'always';
  }
  commit();
}

watch(() => args, commit, { deep: true });

const params = (row: RuleRow) => ({ server: serverName, arg: row.arg, n: row.text });
</script>

<template>
  <fieldset class="fieldset gap-2 py-0" :aria-describedby="summary ? `${uid}-summary` : undefined">
    <legend class="fieldset-legend text-ui">
      {{ t('approvalrule.label') }}
    </legend>
    <div class="join join-vertical sm:join-horizontal">
      <input
        v-for="m in modes" :key="m" class="btn join-item btn-xs" type="radio" :name="`${uid}-mode`" :value="m" :checked="mode === m"
        :aria-label="t(`approvalrule.${m}`)" :disabled="disabled" @change="setMode(m)"
      >
    </div>

    <div v-if="mode === 'unless'" class="flex flex-col gap-2" data-testid="rule-editor">
      <p class="text-xs text-base-content/70">
        {{ t('approvalrule.all_rows') }}
      </p>
      <div v-for="(row, i) in rows" :key="i" class="flex flex-col gap-1 rounded-box border border-base-300 p-2" data-testid="rule-row">
        <p v-if="i > 0" class="text-xs font-medium">
          {{ t('approvalrule.and') }}
        </p>
        <div class="flex flex-wrap items-center gap-2 text-ui">
          <span>{{ t('approvalrule.row_lead') }}</span>
          <label class="contents">
            <span class="sr-only">{{ t('approvalrule.arg') }}</span>
            <input
              :list="`${uid}-args`" type="text" class="input input-xs w-36 font-mono" :value="row.arg" :disabled="disabled" autocomplete="off"
              :aria-invalid="!!checks[i]!.error" :aria-describedby="`${uid}-msg-${i}`" @input="update(i, { arg: ($event.target as HTMLInputElement).value.trim() })"
            >
          </label>
          <span>{{ t('approvalrule.row_mid') }}</span>
          <label class="contents">
            <span class="sr-only">{{ t('approvalrule.values') }}</span>
            <input
              type="text" class="input input-xs min-w-0 flex-1 basis-40" :value="row.text" :placeholder="t('approvalrule.values_placeholder')"
              :disabled="disabled" autocomplete="off" :aria-describedby="`${uid}-msg-${i}`" @input="update(i, { text: ($event.target as HTMLInputElement).value })"
            >
          </label>
          <AppButton variant="ghost" size="xs" square :disabled="disabled" :aria-label="t('approvalrule.remove_condition')" :title="t('approvalrule.remove_condition')" @click="remove(i)">
            <Icon name="lucide:x" size="14" aria-hidden="true" />
          </AppButton>
        </div>
        <label class="label cursor-pointer gap-2 text-xs text-base-content">
          <input type="checkbox" class="checkbox checkbox-xs rounded-sm" :checked="row.absent" :disabled="disabled" @change="update(i, { absent: ($event.target as HTMLInputElement).checked })">
          {{ t('approvalrule.skip_missing', { arg: row.arg || '…' }) }}
        </label>
        <div :id="`${uid}-msg-${i}`">
          <FieldError :id="`${uid}-err-${i}`" :message="checks[i]!.error ? t(checks[i]!.error, params(row)) : ''" />
          <div v-if="checks[i]!.error === 'approvalrule.unknown_arg_missing_error'" class="mt-1 flex flex-wrap gap-2">
            <AppButton variant="plain" size="xs" :disabled="disabled" @click="emit('refresh')">
              {{ t('approvalrule.refresh_tools') }}
            </AppButton>
            <AppButton variant="plain" size="xs" :disabled="disabled" @click="update(i, { absent: false })">
              {{ t('approvalrule.turn_off_missing') }}
            </AppButton>
          </div>
          <p v-if="checks[i]!.warning" class="mt-1 flex items-start gap-1 text-xs text-base-content/70">
            <Icon name="lucide:triangle-alert" size="14" class="shrink-0" aria-hidden="true" />
            <span>{{ t(checks[i]!.warning, params(row)) }} {{ t('approvalrule.recheck_hint') }}</span>
          </p>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <AppButton variant="plain" size="xs" :disabled="disabled || rows.length >= maxConditions" @click="add()">
          <Icon name="lucide:plus" size="14" aria-hidden="true" />
          {{ t('approvalrule.add_condition') }}
        </AppButton>
        <span v-if="rows.length >= maxConditions" class="text-xs text-base-content/70">{{ t('approvalrule.limit_conditions', { n: maxConditions }) }}</span>
      </div>
      <p v-if="!args.length" class="text-xs text-base-content/70">
        {{ t('approvalrule.no_schema') }}
      </p>
      <p v-if="summary" :id="`${uid}-summary`" class="text-xs" data-testid="rule-summary">
        {{ summary }}
      </p>
    </div>
    <datalist :id="`${uid}-args`">
      <option v-for="a in pickable" :key="a.name" :value="a.name" />
    </datalist>
    <div v-if="!trusted && mode !== 'always'" role="alert" class="alert alert-warning alert-soft text-base-content text-ui">
      <Icon name="lucide:triangle-alert" size="16" class="shrink-0" aria-hidden="true" />
      <span>{{ t('grants.untrusted_no_ask') }}</span>
    </div>
  </fieldset>
</template>
