<script setup lang="ts">
import type { RunEvent } from '~/stores/threads';
import EventMark from './EventMark.vue';
import { eventName, lane, laneColor, lanes, scale, ticks } from './runView';

const { events, selected = '' } = defineProps<{ events: RunEvent[], selected?: string }>();
const emit = defineEmits<{ select: [id: string] }>();
const { t } = useI18n();

const max = computed(() => scale(events));
const rows = computed(() => lanes.map(l => ({ lane: l, events: events.filter(e => lane(e) === l) })));
const marks = computed(() => ticks(max.value));

function pct(s: number) {
  return `${(s / max.value) * 100}%`;
}
</script>

<template>
  <div class="flex flex-col gap-1">
    <div v-for="row in rows" :key="row.lane" class="grid grid-cols-[4.5rem_1fr] items-center gap-2">
      <span class="inline-flex items-center gap-1.5 text-xs">
        <EventMark :lane="row.lane" />
        {{ t(`run.lane_${row.lane}`) }}
      </span>
      <div class="relative h-5 rounded-[2px] bg-base-200">
        <button
          v-for="e in row.events"
          :key="e.id"
          type="button"
          class="absolute top-0.5 bottom-0.5 min-w-1 rounded-[2px] hover:opacity-80"
          :class="[
            laneColor[row.lane],
            { 'border border-base-content': row.lane === 'approval', 'ring-2 ring-base-content ring-offset-1 ring-offset-base-100': e.id === selected },
          ]"
          :style="{ left: pct(e.at), width: pct(e.took) }"
          :title="t('run.block_label', { name: eventName(e, t), at: e.at, took: e.took })"
          :aria-label="t('run.block_label', { name: eventName(e, t), at: e.at, took: e.took })"
          :aria-pressed="e.id === selected"
          @click="emit('select', e.id)"
        />
      </div>
    </div>
    <div class="grid grid-cols-[4.5rem_1fr] gap-2 text-xs tabular-nums text-base-content/70" aria-hidden="true">
      <span />
      <div class="relative h-4">
        <span
          v-for="(s, i) in marks"
          :key="s"
          class="absolute whitespace-nowrap"
          :class="i === 0 ? '' : i === marks.length - 1 ? '-translate-x-full' : '-translate-x-1/2'"
          :style="{ left: pct(s) }"
        >{{ s }} s</span>
      </div>
    </div>
  </div>
</template>
