import type { NavGroup } from '#base/components/ui/types';
import { useApprovalsStore } from '~/stores/approvals';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

// useProjectNav lists a project's pages, the one source for the sidebar, the breadcrumbs and the palette.
export function useProjectNav() {
  const route = useRoute();
  const { t } = useI18n();
  const approvals = useApprovalsStore();
  const orgs = useOrgsStore();
  const projects = useProjectsStore();
  return computed<NavGroup[]>(() => {
    const base = `/o/${route.params.org}/projects/${route.params.project}`;
    const settings = `${base}/settings`;
    const org = orgs.bySlug(route.params.org as string);
    const current = projects.items.find(p => p.orgId === org?.id && p.slug === route.params.project);
    const pendingCount = current && approvals.countProject === current.id ? approvals.pendingCount : 0;
    return [{
      items: [
        { key: 'overview', label: t('nav.overview'), to: base, icon: 'lucide:activity', active: route.path === base },
        { key: 'agents', label: t('nav.agents'), to: `${base}/agents`, icon: 'lucide:bot', active: route.path.startsWith(`${base}/agents`) },
        { key: 'approvals', label: t('approvals.title'), to: `${base}/approvals`, icon: 'lucide:hand', active: route.path.startsWith(`${base}/approvals`), badge: pendingCount || undefined },
        { key: 'end-users', label: t('endusers.title'), to: `${base}/end-users`, icon: 'lucide:users-round', active: route.path.startsWith(`${base}/end-users`) },
        { key: 'knowledge', label: t('knowledge.title'), to: `${base}/knowledge`, icon: 'lucide:library', active: route.path.startsWith(`${base}/knowledge`) },
        { key: 'webhooks', label: t('webhooks.title'), to: `${base}/webhooks`, icon: 'lucide:webhook', active: route.path.startsWith(`${base}/webhooks`) },
      ],
    }, {
      label: t('nav.project_settings'),
      items: [
        { key: 'settings', label: t('nav.general'), to: settings, icon: 'lucide:settings-2', active: route.path === settings },
        { key: 'model-providers', label: t('providers.title'), to: `${settings}/model-providers`, icon: 'lucide:cpu', active: route.path.startsWith(`${settings}/model-providers`) },
        { key: 'mcp-servers', label: t('mcpservers.title'), to: `${settings}/mcp-servers`, icon: 'lucide:plug', active: route.path.startsWith(`${settings}/mcp-servers`) },
        { key: 'web-search', label: t('websearch.title'), to: `${settings}/web-search`, icon: 'lucide:search', active: route.path.startsWith(`${settings}/web-search`) },
      ],
    }];
  });
}
