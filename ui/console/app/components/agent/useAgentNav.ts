import type { NavGroup } from '#base/components/ui/types';

// useAgentNav lists an agent's pages, the one source for the agent sidebar, the breadcrumbs and the palette.
export function useAgentNav() {
  const route = useRoute();
  const { t } = useI18n();
  const list = computed(() => `/o/${route.params.org}/projects/${route.params.project}/agents`);
  const base = computed(() => `${list.value}/${route.params.agent}`);
  const groups = computed<NavGroup[]>(() => [{
    items: [
      { key: 'configure', label: t('agent.configure'), to: base.value, icon: 'lucide:sliders-horizontal', active: route.path === base.value },
      { key: 'runs', label: t('agent.runs'), to: `${base.value}/runs`, icon: 'lucide:list-tree', active: route.path.startsWith(`${base.value}/runs`) },
    ],
  }]);
  return { list, base, groups };
}
