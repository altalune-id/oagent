<script setup lang="ts">
import type { RunEvent } from '~/preview/samples';
import EventMark from './EventMark.vue';
import { lane, lanes, scale, ticks } from './runView';

const { events, selected = '' } = defineProps<{ events: RunEvent[], selected?: string }>();
const emit = defineEmits<{ select: [id: string] }>();
const { t } = useI18n();

const max = computed(() => scale(events));
const rows = computed(() => lanes.map(l => ({ lane: l, events: events.filter(e => lane(e) === l) })));

function place(e: RunEvent) {
  return { left: `${(e.at / max.value) * 100}%`, width: `max(0.5rem, ${(e.took / max.value) * 100}%)` };
}
</script>

<template>
  <div class="space-y-1.5">
    <div v-for="row in rows" :key="row.lane" class="grid grid-cols-[6rem_1fr] items-center gap-2">
      <span class="flex items-center gap-1.5 text-xs text-base-content/70">
        <EventMark :lane="row.lane" />
        {{ t(`run.lane_${row.lane}`) }}
      </span>
      <div class="relative h-4 rounded-sm bg-base-200">
        <button
          v-for="e in row.events"
          :key="e.id"
          type="button"
          class="absolute inset-y-0 rounded-sm outline-offset-2"
          :class="[
            { model: 'bg-info', tool: 'bg-base-content/70', approval: 'bg-warning', error: 'bg-error' }[row.lane],
            { 'ring-2 ring-base-content ring-offset-1 ring-offset-base-100': e.id === selected },
          ]"
          :style="place(e)"
          :aria-label="t('run.block_label', { name: e.name, at: e.at, took: e.took })"
          :aria-pressed="e.id === selected"
          @click="emit('select', e.id)"
        />
      </div>
    </div>
    <div class="grid grid-cols-[6rem_1fr] gap-2" aria-hidden="true">
      <span />
      <div class="flex justify-between text-xs text-base-content/70">
        <span v-for="s in ticks(max)" :key="s">{{ s }} s</span>
      </div>
    </div>
  </div>
</template>
