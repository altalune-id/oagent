import { useActiveOrg } from '~/composables/useActiveOrg';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { usePinnedStore } from '~/stores/pinned';
import { useProjectsStore } from '~/stores/projects';

// useLanding resolves where a signed-in person lands: an org's pinned or first project, its overview, onboarding or the org list.
export function useLanding() {
  const orgs = useOrgsStore();
  const projects = useProjectsStore();
  const pinned = usePinnedStore();
  const auth = useAuthStore();
  const activeOrg = useActiveOrg();

  async function resolve(orgSlug?: string): Promise<string> {
    await auth.load();
    const org = (orgSlug ? orgs.bySlug(orgSlug) : undefined)
      ?? orgs.bySlug(pinned.orgSlug)
      ?? orgs.bySlug(auth.me?.activeOrgSlug ?? '')
      ?? orgs.items[0];
    if (!org) {
      return auth.me?.orgCreation === true ? '/onboarding' : '/orgs';
    }
    await activeOrg.enter(org.slug);
    if (!orgs.bySlug(org.slug)) {
      return `/o/${org.slug}`;
    }
    await projects.ensureLoaded(org.id);
    const list = projects.forOrg(org.id);
    const project = list.find(p => p.slug === pinned.projectFor(org.slug)) ?? list[0];
    return project ? `/o/${org.slug}/projects/${project.slug}` : `/o/${org.slug}`;
  }

  return { resolve };
}
