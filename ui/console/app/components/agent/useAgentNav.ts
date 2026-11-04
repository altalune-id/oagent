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
      { key: 'try', label: t('try.title'), to: `${base.value}/try`, icon: 'lucide:message-square-text', active: route.path.startsWith(`${base.value}/try`) },
      { key: 'channels', label: t('channels.title'), to: `${base.value}/channels`, icon: 'lucide:radio-tower', active: route.path.startsWith(`${base.value}/channels`) },
      { key: 'threads', label: t('threads.title'), to: `${base.value}/threads`, icon: 'lucide:messages-square', active: route.path.startsWith(`${base.value}/threads`) },
      { key: 'runs', label: t('agent.runs'), to: `${base.value}/runs`, icon: 'lucide:list-tree', active: route.path.startsWith(`${base.value}/runs`) },
    ],
  }]);
  return { list, base, groups };
}
