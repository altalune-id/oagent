<script setup lang="ts">
import type { EntityItem } from '~/components/ui/types';
import { useLanding } from '~/components/layout/useLanding';
import EntitySelect from '~/components/ui/EntitySelect.vue';
import { useOrgsStore } from '~/stores/orgs';
import { usePinnedStore } from '~/stores/pinned';
import { useUIStore } from '~/stores/ui';

const orgs = useOrgsStore();
const pinned = usePinnedStore();
const ui = useUIStore();
const { resolve } = useLanding();
const route = useRoute();
const { t } = useI18n();

const current = computed(() => orgs.bySlug(route.params.org as string));
const items = computed<EntityItem[]>(() =>
  orgs.items.map(org => ({ key: org.slug, label: org.name, active: org.slug === current.value?.slug, pinned: pinned.orgSlug === org.slug })),
);

function blur() {
  if (import.meta.client) {
    (document.activeElement as HTMLElement | null)?.blur();
  }
}

async function onSelect(slug: string) {
  blur();
  if (slug === current.value?.slug) {
    return;
  }
  await navigateTo(await resolve(slug));
}

function onPin(slug: string) {
  if (pinned.orgSlug === slug) {
    pinned.unpinOrg();
  } else {
    pinned.setOrg(slug);
  }
}

function onAll() {
  blur();
  return navigateTo('/orgs');
}

function onCreate() {
  blur();
  ui.openOrgCreate();
}
</script>

<template>
  <EntitySelect
    :label="current?.name ?? t('org.select')"
    icon="lucide:building-2"
    :placeholder="!current"
    :items="items"
    :empty="t('org.empty')"
    :pin-label="t('common.pin')"
    :unpin-label="t('common.unpin')"
    @select="onSelect"
    @pin="onPin"
  >
    <template #actions>
      <li>
        <button type="button" @click="onAll">
          <Icon name="lucide:layout-list" size="16" />
          {{ t('org.all') }}
        </button>
      </li>
      <li>
        <button type="button" @click="onCreate">
          <Icon name="lucide:plus" size="16" />
          {{ t('org.new') }}
        </button>
      </li>
    </template>
  </EntitySelect>
</template>
