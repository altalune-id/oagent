import type { Crumb, NavGroup, PaletteItem, TopBarPlace } from '#base/components/ui/types';
import { useAgentNav } from '~/components/agent/useAgentNav';
import { personLabel } from '~/components/enduser/label';
import { useOrgNav } from '~/components/layout/useOrgNav';
import { environmentLabelKey } from '~/components/project/environment';
import { useProjectNav } from '~/components/project/useProjectNav';
import { useAgentsStore } from '~/stores/agents';
import { useEndUsersStore } from '~/stores/endUsers';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

// useNavCrumbs derives the top bar trail and the palette entries from the sidebar nav, so a page needs no crumbs of its own.
export function useNavCrumbs() {
  const route = useRoute();
  const { t } = useI18n();
  const orgs = useOrgsStore();
  const projects = useProjectsStore();
  const agents = useAgentsStore();
  const endUsers = useEndUsersStore();
  const orgNav = useOrgNav();
  const projectNav = useProjectNav();
  const agentNav = useAgentNav();

  const org = computed(() => orgs.bySlug(route.params.org as string));
  const project = computed(() => {
    const slug = route.params.project as string | undefined;
    return slug ? projects.items.find(p => p.orgId === org.value?.id && p.slug === slug) ?? { slug, name: slug, id: '' } : undefined;
  });
  const agent = computed(() => {
    const slug = route.params.agent as string | undefined;
    return slug ? agents.items.find(a => a.projectId === project.value?.id && a.slug === slug) ?? { slug, name: slug } : undefined;
  });
  const active = (groups: NavGroup[]) => groups.flatMap(g => g.items).find(i => i.active);

  const crumbs = computed<Crumb[]>(() => {
    if (!org.value) {
      return [];
    }
    const trail: Crumb[] = [{ label: org.value.name, to: `/o/${org.value.slug}` }];
    if (!project.value) {
      const page = active([orgNav.value]);
      return page ? [...trail, { label: page.label }] : trail;
    }
    trail.push({ label: project.value.name, to: `/o/${org.value.slug}/projects/${project.value.slug}` });
    if (!agent.value) {
      const page = active(projectNav.value);
      const endUser = route.params.endUser as string | undefined;
      if (page && endUser) {
        const person = endUsers.detail?.user.id === endUser ? endUsers.detail.user : endUsers.items.find(u => u.id === endUser);
        return [...trail, { label: page.label, to: page.to }, { label: person ? personLabel(person, t) : endUser }];
      }
      return page ? [...trail, { label: page.label }] : trail;
    }
    trail.push({ label: t('nav.agents'), to: agentNav.list.value });
    const page = active(agentNav.groups.value);
    if (page?.key === 'configure') {
      return [...trail, { label: agent.value.name }];
    }
    trail.push({ label: agent.value.name, to: agentNav.base.value });
    if (route.params.thread) {
      return [...trail, { label: t('threads.title'), to: `${agentNav.base.value}/threads` }, { label: t('thread.crumb') }];
    }
    if (route.params.run) {
      return [...trail, { label: t('run.col_run') }];
    }
    return page ? [...trail, { label: page.label }] : trail;
  });

  const palette = computed<PaletteItem[]>(() => {
    const items: PaletteItem[] = orgNav.value.items.map(i => ({ key: `org-${i.key}`, group: org.value?.name ?? '', label: i.label, to: i.to }));
    if (project.value) {
      items.push(...projectNav.value.flatMap(g => g.items).map(i => ({ key: `project-${i.key}`, group: project.value!.name, label: i.label, to: i.to })));
    }
    if (agent.value) {
      items.push(...agentNav.groups.value.flatMap(g => g.items).map(i => ({ key: `agent-${i.key}`, group: agent.value!.name, label: i.label, to: i.to })));
    }
    return items;
  });

  const place = computed<TopBarPlace | undefined>(() => {
    if (!org.value) {
      return undefined;
    }
    if (!project.value) {
      return { label: org.value.name };
    }
    const key = environmentLabelKey('environment' in project.value ? project.value.environment : undefined);
    return { label: agent.value?.name ?? project.value.name, badge: key === 'environment.none' ? undefined : t(key) };
  });

  return { crumbs, place, palette };
}
