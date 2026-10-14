import { useOrgsStore } from '~/stores/orgs';
import { usePinnedStore } from '~/stores/pinned';
import { useProjectsStore } from '~/stores/projects';

// Lands on the pinned project, else the org's first project, else the overview; falls back to the pinned org, then the first org.
export function useLanding() {
  const orgs = useOrgsStore();
  const projects = useProjectsStore();
  const pinned = usePinnedStore();

  async function resolve(orgSlug?: string): Promise<string> {
    const org = (orgSlug ? orgs.bySlug(orgSlug) : orgs.bySlug(pinned.orgSlug)) ?? orgs.items[0];
    if (!org) {
      return '/orgs';
    }
    await projects.ensureLoaded(org.id);
    const list = projects.forOrg(org.id);
    const project = list.find(p => p.slug === pinned.projectFor(org.slug)) ?? list[0];
    return project ? `/o/${org.slug}/projects/${project.slug}` : `/o/${org.slug}`;
  }

  return { resolve };
}
