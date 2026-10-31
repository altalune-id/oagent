<script setup lang="ts">
import type { EntityItem } from '#base/components/ui/types';
import EntitySelect from '#base/components/ui/EntitySelect.vue';
import { useOrgsStore } from '~/stores/orgs';
import { usePinnedStore } from '~/stores/pinned';
import { useProjectsStore } from '~/stores/projects';
import { useUIStore } from '~/stores/ui';

const orgs = useOrgsStore();
const projects = useProjectsStore();
const pinned = usePinnedStore();
const ui = useUIStore();
const route = useRoute();
const { t } = useI18n();

const org = computed(() => route.params.org as string | undefined);
const project = computed(() => route.params.project as string | undefined);
const orgId = computed(() => (org.value ? orgs.bySlug(org.value)?.id ?? '' : ''));
const list = computed(() => (orgId.value ? projects.forOrg(orgId.value) : []));
const current = computed(() => list.value.find(p => p.slug === project.value));
const items = computed<EntityItem[]>(() =>
  list.value.map(p => ({ key: p.slug, label: p.name, active: p.slug === project.value, pinned: !!org.value && pinned.projectFor(org.value) === p.slug })),
);

function blur() {
  if (import.meta.client) {
    (document.activeElement as HTMLElement | null)?.blur();
  }
}

function onSelect(slug: string) {
  blur();
  if (!org.value || slug === project.value) {
    return;
  }
  return navigateTo(`/o/${org.value}/projects/${slug}`);
}

function onPin(slug: string) {
  if (!org.value) {
    return;
  }
  if (pinned.projectFor(org.value) === slug) {
    pinned.unpinProject(org.value);
  } else {
    pinned.setProject(org.value, slug);
  }
}

function onCreate() {
  blur();
  ui.openProjectCreate();
}
</script>

<template>
  <EntitySelect
    v-if="org && orgs.bySlug(org)"
    :label="current?.name ?? t('project.select')"
    icon="lucide:folder"
    :placeholder="!current"
    :items="items"
    :empty="t('project.empty')"
    :pin-label="t('common.pin')"
    :unpin-label="t('common.unpin')"
    @select="onSelect"
    @pin="onPin"
  >
    <template #actions>
      <li>
        <button type="button" @click="onCreate">
          <Icon name="lucide:plus" size="16" />
          {{ t('project.new') }}
        </button>
      </li>
    </template>
  </EntitySelect>
</template>
