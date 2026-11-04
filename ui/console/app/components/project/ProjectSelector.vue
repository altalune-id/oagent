<script setup lang="ts">
import type { EntityItem } from '#base/components/ui/types';
import type { ProjectEnvironment } from '#gen/project/v1/project_pb';
import EntitySelect from '#base/components/ui/EntitySelect.vue';
import { environmentLabelKey } from '~/components/project/environment';
import { useCan } from '~/composables/useCan';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useRecentStore } from '~/stores/recent';
import { useUIStore } from '~/stores/ui';

const orgs = useOrgsStore();
const projects = useProjectsStore();
const recent = useRecentStore();
const ui = useUIStore();
const route = useRoute();
const { t } = useI18n();
const canCreate = useCan('project:write');

const org = computed(() => route.params.org as string | undefined);
const project = computed(() => route.params.project as string | undefined);
const orgId = computed(() => (org.value ? orgs.bySlug(org.value)?.id ?? '' : ''));
const list = computed(() => (orgId.value ? projects.forOrg(orgId.value) : []));
const current = computed(() => list.value.find(p => p.slug === project.value));

function tag(env?: ProjectEnvironment) {
  const key = environmentLabelKey(env);
  return key === 'environment.none' ? '' : t(key);
}

const items = computed<EntityItem[]>(() =>
  recent.byRecency(list.value, p => p.slug, recent.projects[org.value ?? ''] ?? [])
    .map(p => ({ key: p.slug, label: p.name, active: p.slug === project.value })),
);

function onSelect(slug: string) {
  if (!org.value || slug === project.value) {
    return;
  }
  return navigateTo(`/o/${org.value}/projects/${slug}`);
}
</script>

<template>
  <EntitySelect
    v-if="org && orgs.bySlug(org)"
    :label="t('project.switch')"
    :title="t('project.group_title')"
    :items="items"
    :empty="t('project.empty')"
    trigger-class="h-auto min-h-8 py-1"
    @select="onSelect"
  >
    <template #trigger>
      <span class="flex min-w-0 flex-col items-start gap-0.5">
        <span class="max-w-full truncate font-medium">{{ current?.name ?? t('project.select') }}</span>
        <span v-if="tag(current?.environment)" class="badge badge-xs">{{ tag(current?.environment) }}</span>
      </span>
    </template>
    <template v-if="canCreate" #actions>
      <li>
        <button type="button" @click="ui.openProjectCreate()">
          <Icon name="lucide:plus" size="16" class="shrink-0" />
          {{ t('project.new') }}
        </button>
      </li>
    </template>
  </EntitySelect>
</template>
