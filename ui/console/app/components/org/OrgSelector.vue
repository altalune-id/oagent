<script setup lang="ts">
import type { EntityItem } from '#base/components/ui/types';
import EntitySelect from '#base/components/ui/EntitySelect.vue';
import { useLanding } from '~/components/layout/useLanding';
import { useOrgCreation } from '~/components/org/create/useOrgCreation';
import { logoURL } from '~/lib/auth/urls';
import { useOrgsStore } from '~/stores/orgs';
import { useRecentStore } from '~/stores/recent';
import { useUIStore } from '~/stores/ui';

const orgs = useOrgsStore();
const recent = useRecentStore();
const ui = useUIStore();
const { resolve } = useLanding();
const { canCreate } = useOrgCreation();
const route = useRoute();
const { t } = useI18n();
const logo = logoURL(useRuntimeConfig().public.apiUrl);

const current = computed(() => orgs.bySlug(route.params.org as string));
const items = computed<EntityItem[]>(() =>
  recent.byRecency(orgs.items, o => o.slug, recent.orgs).map(org => ({ key: org.slug, label: org.name, active: org.slug === current.value?.slug })),
);

async function onSelect(slug: string) {
  if (slug === current.value?.slug) {
    return;
  }
  await navigateTo(await resolve(slug));
}
</script>

<template>
  <EntitySelect :label="t('org.switch')" :items="items" :empty="t('org.empty')" @select="onSelect">
    <template #trigger>
      <span class="inline-flex min-w-0 items-center gap-2">
        <img :src="logo" alt="" width="16" height="16" class="size-4 shrink-0 object-contain">
        <span class="font-semibold">OAgent</span>
        <span class="text-base-content/70">/</span>
        <span class="truncate">{{ current?.name ?? t('org.select') }}</span>
      </span>
    </template>
    <template v-if="canCreate" #actions>
      <li>
        <button type="button" @click="ui.openOrgCreate()">
          <Icon name="lucide:plus" size="16" class="shrink-0" />
          {{ t('org.new') }}
        </button>
      </li>
    </template>
  </EntitySelect>
</template>
