import type { NavGroup } from '#base/components/ui/types';

// useOrgNav lists the organization's own pages, shown alone in the org sidebar and above the project row in a project.
export function useOrgNav() {
  const route = useRoute();
  const { t } = useI18n();
  return computed<NavGroup>(() => {
    const base = `/o/${route.params.org}`;
    const settings = `${base}/settings`;
    return {
      items: [
        { key: 'org-overview', label: t('nav.org_overview'), to: base, icon: 'lucide:building-complex', active: route.path === base },
        { key: 'projects', label: t('nav.projects'), to: `${base}/projects`, icon: 'lucide:folders', active: route.path === `${base}/projects` || route.path === `${base}/projects/` },
        { key: 'members', label: t('members.title'), to: `${settings}/members`, icon: 'lucide:users', active: route.path.startsWith(`${settings}/members`) },
        { key: 'api-keys', label: t('apikeys.title'), to: `${settings}/api-keys`, icon: 'lucide:key-round', active: route.path.startsWith(`${settings}/api-keys`) },
        { key: 'personal-keys', label: t('apikeys.personal_title'), to: `${settings}/personal-keys`, icon: 'lucide:user-round-key', active: route.path.startsWith(`${settings}/personal-keys`) },
        { key: 'org-settings', label: t('nav.org_settings'), to: settings, icon: 'lucide:settings', active: route.path === settings },
      ],
    };
  });
}
