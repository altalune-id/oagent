<script setup lang="ts">
import type { WhoamiData } from './whoami';
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
  <main class="card">
    <header>
      <span class="avatar">{{ initial }}</span>
      <strong v-if="title">{{ title }}</strong>
      <span v-if="source && source !== title" class="badge">{{ source }}</span>
      <span v-if="who.operator" class="badge operator">operator</span>
    </header>

    <dl v-if="rows.length">
      <template v-for="row in rows" :key="row.label">
        <dt>{{ row.label }}</dt><dd>{{ row.value }}</dd>
      </template>
    </dl>

    <p v-if="badges.shown.length" class="badges">
      <span v-for="p in badges.shown" :key="p" class="badge mono">{{ p }}</span>
      <span v-if="badges.more" class="badge muted">+{{ badges.more }}</span>
    </p>
  </main>
</template>

<style scoped>
.avatar {
  display: inline-grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: #3b82f6;
  color: #fff;
  font-weight: 600;
}
.badges {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 12px 0 0;
}
.badge {
  padding: 1px 8px;
  border: 1px solid rgba(128, 128, 128, 0.4);
  border-radius: 999px;
  font-size: 12px;
}
.operator {
  border-color: #f59e0b;
  color: #b45309;
}
</style>
