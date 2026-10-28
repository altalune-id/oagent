import { Code, ConnectError } from '@connectrpc/connect';
import { isPerson } from '~/lib/auth/person';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';

// useActiveOrg switches a person's session to the org they open, before its pages load.
export function useActiveOrg() {
  const auth = useAuthStore();
  const orgs = useOrgsStore();

  async function cached(slug: string) {
    await orgs.ensureLoaded();
    if (!orgs.bySlug(slug)) {
      await orgs.load();
    }
    return orgs.bySlug(slug);
  }

  async function enter(slug: string): Promise<void> {
    await auth.load();
    const me = auth.me;
    if (!me || !isPerson(me) || me.activeOrgSlug === slug) {
      return;
    }
    const org = await cached(slug);
    if (!org) {
      return;
    }
    try {
      await auth.setActiveOrg(org.id);
    } catch (e) {
      if (!(e instanceof ConnectError && e.code === Code.NotFound)) {
        throw e;
      }
      await orgs.load();
    }
  }

  return { enter };
}
