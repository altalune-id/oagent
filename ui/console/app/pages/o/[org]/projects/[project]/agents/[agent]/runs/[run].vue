<script setup lang="ts">
import type { Lane, RunStatus } from '~/components/run/runView';
import type { RunEvent } from '~/preview/samples';
import KeyValue from '#base/components/ui/KeyValue.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import { useDates } from '#base/composables/useDates';
import EventMark from '~/components/run/EventMark.vue';
import RunMinimap from '~/components/run/RunMinimap.vue';
import { eventsFor, lane, lanes, toolCalls, totals } from '~/components/run/runView';
import { useUnits } from '~/composables/useUnits';
import { sampleRun } from '~/preview/samples';

const { t } = useI18n();
const route = useRoute();
const { moment } = useDates();
const { money, seconds, count } = useUnits();

const run = sampleRun();
const status = ref<RunStatus>('waiting');
const amount = ref(run.approval.amountCents / 100);
const shown = ref<Lane[]>([...lanes]);
const selectedId = ref(run.events.at(-1)!.id);

const list = computed(() => `/o/${route.params.org}/projects/${route.params.project}/agents`);
const agentBase = computed(() => `${list.value}/${route.params.agent}`);
const events = computed(() => eventsFor(run, status.value));
const sum = computed(() => totals(events.value));
const visible = computed(() => events.value.filter((e) => {
  const l = lane(e);
  return l === null || shown.value.includes(l);
}));
const selected = computed<RunEvent>(() => events.value.find(e => e.id === selectedId.value) ?? events.value[0]!);
const tools = computed(() => toolCalls(events.value));

function toggle(l: Lane) {
  shown.value = shown.value.includes(l) ? shown.value.filter(x => x !== l) : [...shown.value, l];
}

function select(id: string) {
  selectedId.value = id;
  document.getElementById(`event-${id}`)?.focus();
}

function decide(next: RunStatus) {
  status.value = next;
  selectedId.value = events.value.at(-1)!.id;
}

const statusLabel = computed(() => t(`run.status_${status.value}`));
const statusBadge = computed(() => ({ waiting: 'badge-warning', approved: 'badge-success', rejected: 'badge-error' })[status.value]);

useHead({ title: () => `${t('run.title', { id: run.id })} · OAgent` });
</script>

<template>
  <div class="space-y-6">
    <PageHeader
      :title="t('run.title', { id: run.id })"
      mono
      :preview="t('preview.badge')"
      :crumbs="[{ label: t('nav.agents'), to: list }, { label: run.agentName, to: agentBase }, { label: t('agent.runs'), to: `${agentBase}/runs` }, { label: run.id }]"
      :crumbs-label="t('common.breadcrumb')"
      :description="t('run.started', { agent: run.agentName, version: run.version, model: run.model, by: run.startedBy, at: moment(run.startedAt) })"
    >
      <template #actions>
        <a v-if="status === 'waiting'" href="#approval" class="btn btn-primary btn-sm">{{ t('run.review_approval') }}</a>
      </template>
    </PageHeader>

    <div role="note" class="alert alert-info alert-soft">
      <Icon name="lucide:flask-conical" size="18" />
      <span>{{ t('run.preview_note') }}</span>
    </div>

    <div class="stats stats-vertical w-full border border-base-300 sm:stats-horizontal">
      <div class="stat">
        <div class="stat-title text-base-content/70">
          {{ t('run.status') }}
        </div>
        <div class="stat-value text-lg">
          <span class="badge badge-soft" :class="statusBadge">{{ statusLabel }}</span>
        </div>
      </div>
      <div class="stat">
        <div class="stat-title text-base-content/70">
          {{ t('run.duration') }}
        </div>
        <div class="stat-value text-lg tabular-nums">
          {{ seconds(sum.seconds) }}
        </div>
      </div>
      <div class="stat">
        <div class="stat-title text-base-content/70">
          {{ t('run.cost') }}
        </div>
        <div class="stat-value text-lg tabular-nums">
          {{ money(sum.costCents) }}
        </div>
      </div>
      <div class="stat">
        <div class="stat-title text-base-content/70">
          {{ t('run.steps') }}
        </div>
        <div class="stat-value text-lg tabular-nums">
          {{ t('run.steps_of', { n: sum.steps, max: run.maxSteps }) }}
        </div>
      </div>
    </div>

    <Section :title="t('run.timeline')">
      <RunMinimap :events="events" :selected="selected.id" @select="select" />
    </Section>

    <div class="grid grid-cols-[minmax(0,1fr)] gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
      <Section :title="t('run.transcript')">
        <template #actions>
          <div class="flex flex-wrap gap-1" role="group" :aria-label="t('run.show_types')">
            <button
              v-for="l in lanes" :key="l" type="button" class="btn btn-xs" :class="shown.includes(l) ? 'btn-active' : 'btn-ghost'"
              :aria-pressed="shown.includes(l)" @click="toggle(l)"
            >
              <EventMark :lane="l" />
              {{ t(`run.lane_${l}`) }}
            </button>
          </div>
        </template>
        <ol class="space-y-1">
          <li v-for="e in visible" :key="e.id">
            <div v-if="e.type === 'approval'" id="approval" class="card card-border my-2" :class="status === 'waiting' ? 'border-warning' : 'border-base-300'">
              <div class="card-body gap-3 p-4">
                <button :id="`event-${e.id}`" type="button" class="flex items-center gap-2 text-left text-sm font-semibold" @click="selectedId = e.id">
                  <EventMark lane="approval" />
                  {{ t('run.approval_for', { tool: e.name }) }}
                  <span class="font-normal text-base-content/70">{{ e.at }} s</span>
                </button>
                <p class="font-medium">
                  {{ run.approval.title }}
                </p>
                <p class="text-sm text-base-content/70">
                  {{ t('run.approval_why', { why: run.approval.why }) }}
                </p>
                <div v-if="status === 'waiting'" class="flex flex-wrap items-end gap-2">
                  <label class="input w-36">
                    <span class="text-base-content/70">{{ t('run.amount') }} $</span>
                    <input v-model.number="amount" type="number" min="0" step="0.01" class="grow tabular-nums">
                  </label>
                  <button type="button" class="btn btn-primary btn-sm" @click="decide('approved')">
                    {{ t('run.approve') }}
                  </button>
                  <button type="button" class="btn btn-outline btn-error btn-sm" @click="decide('rejected')">
                    {{ t('run.reject') }}
                  </button>
                </div>
                <p v-else role="status" class="text-sm">
                  {{ status === 'approved' ? t('run.approved_note', { amount: money(amount * 100) }) : t('run.rejected_note') }}
                </p>
              </div>
            </div>
            <button
              v-else
              :id="`event-${e.id}`"
              type="button"
              class="flex w-full items-start gap-2 rounded-box px-2 py-1.5 text-left hover:bg-base-200"
              :class="{ 'bg-base-200': e.id === selected.id, 'border-l-2 border-error': e.type === 'error' }"
              :aria-current="e.id === selected.id || undefined"
              @click="selectedId = e.id"
            >
              <span class="mt-1.5"><EventMark :lane="lane(e)" /></span>
              <span class="min-w-0 flex-1">
                <span class="flex flex-wrap items-baseline gap-x-2">
                  <span class="text-sm font-medium" :class="{ 'font-mono': e.type === 'tool' || e.type === 'error' }">{{ e.name }}</span>
                  <span class="text-xs text-base-content/70">{{ t('run.event_time', { at: e.at, took: e.took }) }}<template v-if="e.costCents">, {{ money(e.costCents) }}</template></span>
                </span>
                <span class="block text-sm text-base-content/70">{{ e.summary }}</span>
              </span>
            </button>
          </li>
        </ol>
      </Section>

      <div class="space-y-6 lg:sticky lg:top-20 lg:self-start">
        <Section :title="t('run.inspector')" :description="selected.name" flush>
          <KeyValue
            :items="[
              { label: t('run.started_at'), value: `${selected.at} s` },
              { label: t('run.took'), value: `${selected.took} s` },
              { label: t('run.cost'), value: selected.costCents ? money(selected.costCents) : '—' },
              { label: t('run.tokens'), value: selected.tokens ? count(selected.tokens) : '—' },
            ]"
          />
          <div v-if="selected.input || selected.output" class="flex flex-col gap-4 border-t border-base-300 px-4 py-3">
            <div v-if="selected.input" class="space-y-1">
              <h3 class="text-sm font-semibold">
                {{ t('run.input') }}
              </h3>
              <pre class="overflow-auto rounded-box bg-base-200 p-3 font-mono text-xs whitespace-pre-wrap break-all">{{ selected.input }}</pre>
            </div>
            <div v-if="selected.output" class="space-y-1">
              <h3 class="text-sm font-semibold">
                {{ t('run.output') }}
              </h3>
              <pre class="overflow-auto rounded-box bg-base-200 p-3 font-mono text-xs whitespace-pre-wrap break-all">{{ selected.output }}</pre>
            </div>
          </div>
        </Section>
        <Section :title="t('run.tools_in_run')">
          <ul class="space-y-1 text-sm">
            <li v-for="tool in tools" :key="tool.name" class="flex justify-between gap-2">
              <span class="font-mono text-xs break-all">{{ tool.name }}</span>
              <span class="text-base-content/70 tabular-nums">{{ t('run.tool_calls', { calls: tool.calls, failed: tool.failed, seconds: tool.seconds }) }}</span>
            </li>
          </ul>
        </Section>
      </div>
    </div>
  </div>
</template>
