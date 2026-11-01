<script setup lang="ts">
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const role = computed(() => orgs.bySlug(route.params.org as string)?.role ?? '');
</script>

<template>
  <span v-if="role" class="badge badge-ghost badge-sm" data-testid="org-role">{{ t(`members.role_${role}`) }}</span>
</template>
