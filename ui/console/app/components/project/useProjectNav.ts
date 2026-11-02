import type { NavGroup } from '#base/components/ui/types';

// useProjectNav lists a project's pages, the one source for the sidebar, the breadcrumbs and the palette.
export function useProjectNav() {
  const route = useRoute();
  const { t } = useI18n();
  return computed<NavGroup[]>(() => {
    const base = `/o/${route.params.org}/projects/${route.params.project}`;
    return [{
      items: [
        { key: 'overview', label: t('nav.overview'), to: base, icon: 'lucide:activity', active: route.path === base },
        { key: 'agents', label: t('nav.agents'), to: `${base}/agents`, icon: 'lucide:bot', active: route.path.startsWith(`${base}/agents`) },
        { key: 'webhooks', label: t('webhooks.title'), to: `${base}/webhooks`, icon: 'lucide:webhook', active: route.path.startsWith(`${base}/webhooks`) },
        { key: 'settings', label: t('nav.settings'), to: `${base}/settings`, icon: 'lucide:settings-2', active: route.path.startsWith(`${base}/settings`) },
      ],
    }];
  });
}
