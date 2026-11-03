<script setup lang="ts">
import { useLanding } from '~/components/layout/useLanding';
import { useOrgsStore } from '~/stores/orgs';

const orgs = useOrgsStore();
const { resolve } = useLanding();

await orgs.ensureLoaded();

await navigateTo(orgs.error ? '/orgs' : await resolve(), { replace: true });
</script>

<template>
  <div />
</template>
