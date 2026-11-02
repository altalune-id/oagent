<script setup lang="ts">
import type { Lane, RunStatus } from '~/components/run/runView';
import type { RunEvent } from '~/preview/samples';
import AppButton from '#base/components/ui/AppButton.vue';
import CopyButton from '#base/components/ui/CopyButton.vue';
import { useDates } from '#base/composables/useDates';
import EventMark from '~/components/run/EventMark.vue';
import RunMinimap from '~/components/run/RunMinimap.vue';
import { eventsFor, lane, lanes, toolCalls, totals } from '~/components/run/runView';
import { useUnits } from '~/composables/useUnits';
import { sampleRun } from '~/preview/samples';

definePageMeta({ bleed: true });

const { t } = useI18n();
const route = useRoute();
const { moment } = useDates();
const { money, seconds, count } = useUnits();

const run = sampleRun();
const status = ref<RunStatus>('waiting');
const amount = ref(run.approval.amountCents / 100);
const editing = ref(false);
const shown = ref<Lane[]>([...lanes]);
const fromHash = route.hash?.startsWith('#event-') ? route.hash.slice('#event-'.length) : '';
const selectedId = ref(run.events.some(e => e.id === fromHash) ? fromHash : run.events.at(-1)!.id);
const amountId = useId();

const events = computed(() => eventsFor(run, status.value));
const sum = computed(() => totals(events.value));
const visible = computed(() => events.value.filter((e) => {
  const l = lane(e);
  return l === null || shown.value.includes(l);
}));
const selected = computed<RunEvent>(() => events.value.find(e => e.id === selectedId.value) ?? events.value[0]!);
const selectedLane = computed(() => lane(selected.value));
const tools = computed(() => toolCalls(events.value));
const link = computed(() => `${window.location.origin}${route.path}#event-${selected.value.id}`);

function select(id: string) {
  selectedId.value = id;
  nextTick(() => document.getElementById(`event-${id}`)?.focus());
}

function decide(next: RunStatus) {
  status.value = next;
  editing.value = false;
  selectedId.value = events.value.at(-1)!.id;
}

function eventTime(e: RunEvent) {
  const time = e.took ? t('run.event_time', { at: e.at, took: e.took }) : t('run.event_at', { at: e.at });
  return e.costCents ? `${time}, ${money(e.costCents)}` : time;
}

useHead({ title: () => `${t('run.title', { id: run.id })} · OAgent` });
</script>

<template>
  <div class="flex min-w-0 flex-col">
    <header class="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 px-4 py-3 lg:px-6">
      <div class="min-w-0">
        <h1 class="text-xl font-semibold">
          {{ t('run.heading') }} <span class="font-mono text-base font-medium break-all">{{ run.id }}</span>
          <span class="badge badge-info badge-soft badge-sm ms-2 align-middle">{{ t('preview.badge') }}</span>
        </h1>
        <p class="text-ui text-base-content/70">
          {{ t('run.started', { agent: run.agentName, version: run.version, model: run.model, by: run.startedBy, at: moment(run.startedAt) }) }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-x-5 gap-y-2">
        <dl class="flex flex-wrap items-baseline gap-x-5 gap-y-1 text-ui">
          <div class="flex items-baseline gap-1.5">
            <dt class="text-base-content/70">
              {{ t('run.status') }}
            </dt>
            <dd class="font-medium" role="status">
              {{ t(`run.status_${status}`) }}
            </dd>
          </div>
          <div class="flex items-baseline gap-1.5">
            <dt class="text-base-content/70">
              {{ t('run.duration') }}
            </dt>
            <dd class="font-medium tabular-nums">
              {{ seconds(sum.seconds) }}
            </dd>
          </div>
          <div class="flex items-baseline gap-1.5">
            <dt class="text-base-content/70">
              {{ t('run.cost') }}
            </dt>
            <dd class="font-medium tabular-nums">
              {{ money(sum.costCents) }}
            </dd>
          </div>
          <div class="flex items-baseline gap-1.5">
            <dt class="text-base-content/70">
              {{ t('run.steps') }}
            </dt>
            <dd class="font-medium tabular-nums">
              {{ t('run.steps_of', { n: sum.steps, max: run.maxSteps }) }}
            </dd>
          </div>
        </dl>
        <AppButton v-if="status === 'waiting'" size="sm" @click="select('e9')">
          <Icon name="lucide:hand" size="16" aria-hidden="true" />
          {{ t('run.review_approval') }}
        </AppButton>
      </div>
      <p role="note" class="flex w-full items-center gap-1.5 text-xs text-base-content/70">
        <Icon name="lucide:flask-conical" size="14" class="shrink-0" aria-hidden="true" />
        {{ t('run.preview_note') }}
      </p>
    </header>

    <section class="border-y border-base-300 px-4 py-3 lg:px-6" :aria-label="t('run.timeline')">
      <RunMinimap :events="events" :selected="selected.id" @select="select" />
    </section>

    <div class="grid min-w-0 grid-cols-[minmax(0,1fr)] lg:grid-cols-[minmax(0,1fr)_minmax(20rem,24rem)]">
      <section class="min-w-0" :aria-labelledby="`${amountId}-transcript`">
        <div class="flex flex-wrap items-center justify-between gap-2 px-4 py-2">
          <h2 :id="`${amountId}-transcript`" class="text-ui font-semibold">
            {{ t('run.transcript') }}
          </h2>
          <fieldset class="flex flex-wrap items-center gap-x-3 gap-y-1">
            <legend class="sr-only">
              {{ t('run.show_types') }}
            </legend>
            <label v-for="l in lanes" :key="l" class="label gap-1.5 text-xs text-base-content">
              <input v-model="shown" class="checkbox checkbox-xs rounded-sm" type="checkbox" :value="l">
              <EventMark :lane="l" />
              {{ t(`run.lane_${l}`) }}
            </label>
          </fieldset>
        </div>
        <ol class="divide-y divide-base-300 border-t border-base-300 text-ui">
          <template v-for="e in visible" :key="e.id">
            <li v-if="e.type === 'approval'" id="approval" class="border-y-2 border-warning bg-base-200 px-4 py-3">
              <button :id="`event-${e.id}`" type="button" class="flex w-full items-start gap-2 text-start" :aria-pressed="e.id === selected.id" @click="selectedId = e.id">
                <span class="mt-1"><EventMark lane="approval" /></span>
                <span class="min-w-0 flex-1">
                  <span class="flex flex-wrap items-center justify-between gap-x-2">
                    <span class="font-semibold">{{ t('run.approval_prefix') }} <span class="font-mono text-xs">{{ e.name }}</span></span>
                    <span class="text-xs tabular-nums text-base-content/70">{{ t('run.event_at', { at: e.at }) }}</span>
                  </span>
                  <span class="block text-sm">{{ run.approval.title }}</span>
                </span>
              </button>
              <div v-if="status === 'waiting'" class="flex flex-col gap-2 ps-[1.125rem] pt-2">
                <p class="text-ui text-base-content/70">
                  {{ t('run.approval_why', { why: run.approval.why }) }}
                </p>
                <div v-if="editing" class="flex items-center gap-2">
                  <label class="text-ui" :for="amountId">{{ t('run.amount') }}</label>
                  <label class="input input-sm text-ui w-28">
                    $<input :id="amountId" v-model.number="amount" type="number" step="0.01" min="0" :max="run.approval.amountCents / 100" class="tabular-nums">
                  </label>
                </div>
                <div class="flex flex-wrap gap-2">
                  <AppButton size="sm" @click="decide('approved')">
                    <Icon name="lucide:check" size="16" aria-hidden="true" />
                    {{ t('run.approve') }}
                  </AppButton>
                  <AppButton v-if="!editing" variant="plain" size="sm" @click="editing = true">
                    <Icon name="lucide:pencil" size="16" aria-hidden="true" />
                    {{ t('run.edit_amount') }}
                  </AppButton>
                  <AppButton variant="plain" size="sm" @click="decide('rejected')">
                    <Icon name="lucide:x" size="16" aria-hidden="true" />
                    {{ t('run.reject') }}
                  </AppButton>
                </div>
              </div>
              <p v-else role="status" class="ps-[1.125rem] pt-1 text-ui">
                {{ status === 'approved' ? t('run.approved_note', { amount: money(amount * 100) }) : t('run.rejected_note') }}
              </p>
            </li>
            <li v-else>
              <button
                :id="`event-${e.id}`"
                type="button"
                class="flex w-full items-start gap-2 px-4 py-1.5 text-start hover:bg-base-200 aria-pressed:bg-base-200"
                :aria-pressed="e.id === selected.id"
                @click="selectedId = e.id"
              >
                <span class="mt-1 w-2.5 shrink-0"><EventMark :lane="lane(e)" /></span>
                <span class="min-w-0 flex-1">
                  <span class="flex flex-wrap items-center justify-between gap-x-2">
                    <span class="font-medium" :class="{ 'font-mono text-xs': e.type === 'tool' || e.type === 'error' }">{{ e.name }}</span>
                    <span class="text-xs tabular-nums text-base-content/70">{{ eventTime(e) }}</span>
                  </span>
                  <span class="block text-base-content/70">{{ e.summary }}</span>
                </span>
              </button>
            </li>
          </template>
        </ol>
      </section>

      <aside class="min-w-0 border-t border-base-300 bg-base-200 lg:border-s lg:border-t-0" :aria-label="t('run.inspector')">
        <div class="flex flex-col gap-3 p-4 text-ui lg:sticky lg:top-0 lg:max-h-dvh lg:overflow-y-auto">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="inline-flex items-center gap-1.5 text-xs">
                <EventMark :lane="selectedLane" />
                {{ selectedLane ? t(`run.lane_${selectedLane}`) : t('run.input') }}
              </div>
              <h2 class="font-semibold break-all">
                {{ selected.name }}
              </h2>
            </div>
            <CopyButton :value="link" :label="t('run.copy_link')" />
          </div>
          <p>{{ selected.summary }}</p>
          <dl class="grid grid-cols-[4rem_1fr] gap-x-3 gap-y-1 tabular-nums">
            <dt class="text-base-content/70">
              {{ t('run.started_at') }}
            </dt>
            <dd>{{ seconds(selected.at) }}</dd>
            <dt class="text-base-content/70">
              {{ t('run.took') }}
            </dt>
            <dd>{{ seconds(selected.took) }}</dd>
            <dt class="text-base-content/70">
              {{ t('run.cost') }}
            </dt>
            <dd>{{ selected.costCents ? money(selected.costCents) : '—' }}</dd>
            <dt class="text-base-content/70">
              {{ t('run.tokens') }}
            </dt>
            <dd>{{ selected.tokens ? count(selected.tokens) : '—' }}</dd>
          </dl>
          <div v-if="selected.input">
            <h3 class="text-xs text-base-content/70">
              {{ t('run.input') }}
            </h3>
            <pre class="whitespace-pre-wrap wrap-anywhere rounded-box bg-base-100 p-2 font-mono text-xs">{{ selected.input }}</pre>
          </div>
          <div v-if="selected.output">
            <h3 class="text-xs text-base-content/70">
              {{ t('run.output') }}
            </h3>
            <pre class="whitespace-pre-wrap wrap-anywhere rounded-box bg-base-100 p-2 font-mono text-xs">{{ selected.output }}</pre>
          </div>
          <div class="border-t border-base-300 pt-3">
            <h3 class="pb-1 text-xs text-base-content/70">
              {{ t('run.tools_in_run') }}
            </h3>
            <div class="max-w-full overflow-x-auto">
              <table class="table table-xs text-xs">
                <tbody>
                  <tr v-for="tool in tools" :key="tool.name">
                    <td class="px-0 font-mono">
                      {{ tool.name }}
                    </td>
                    <td class="px-0 text-end tabular-nums">
                      {{ tool.failed ? t('run.tool_calls_failed', { calls: tool.calls, failed: tool.failed }) : t('run.tool_calls', { calls: tool.calls, seconds: tool.seconds }, tool.calls) }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </aside>
    </div>
  </div>
</template>
