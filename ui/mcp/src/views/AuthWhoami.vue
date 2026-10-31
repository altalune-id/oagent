<script setup lang="ts">
import type { WhoamiData } from './whoami';
import ViewShell from '../components/ViewShell.vue';
import { t } from '../copy';
import { permissionBadges, sourceLabel, whoamiRows, whoamiTitle } from './whoami';

const props = defineProps<{ data: Record<string, unknown> }>();

const who = computed(() => props.data as WhoamiData);
const title = computed(() => whoamiTitle(who.value));
const initial = computed(() => (title.value || '?').charAt(0).toUpperCase());
const source = computed(() => (who.value.source === 'local' ? '' : sourceLabel(who.value.source)));
const rows = computed(() => whoamiRows(who.value));
const badges = computed(() => permissionBadges(who.value.permissions));
</script>

<template>
  <ViewShell :title="title || 'OAgent'">
    <template #badges>
      <span v-if="source && source !== title" class="badge badge-sm badge-ghost">{{ source }}</span>
      <span v-if="who.operator" class="badge badge-sm badge-warning">{{ t('mcp.whoami_operator') }}</span>
    </template>

    <div class="flex items-center gap-3">
      <div class="avatar avatar-placeholder">
        <div class="w-8 rounded-full bg-primary text-primary-content">
          <span class="text-sm font-semibold">{{ initial }}</span>
        </div>
      </div>
      <dl v-if="rows.length" class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
        <template v-for="row in rows" :key="row.label">
          <dt class="text-base-content/60">
            {{ row.label }}
          </dt>
          <dd class="font-mono break-all">
            {{ row.value }}
          </dd>
        </template>
      </dl>
    </div>

    <p v-if="badges.shown.length" class="flex flex-wrap gap-1">
      <span v-for="p in badges.shown" :key="p" class="badge badge-sm badge-ghost font-mono">{{ p }}</span>
      <span v-if="badges.more" class="badge badge-sm badge-ghost">+{{ badges.more }}</span>
    </p>
  </ViewShell>
</template>
