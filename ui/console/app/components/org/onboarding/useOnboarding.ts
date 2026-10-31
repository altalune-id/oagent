import type { OrgCreateCall } from '~/components/org/create/useOrgCreate';
import { errorCode } from '#base/lib/apperror/message';
import { useLanding } from '~/components/layout/useLanding';
import { useOrgCreate } from '~/components/org/create/useOrgCreate';
import { useOrgsStore } from '~/stores/orgs';

// useOnboarding creates the caller's own first org and lands on its default project; one who already owns an org is taken there.
export function useOnboarding() {
  const { resolve } = useLanding();
  const orgs = useOrgsStore();
  const { $authClient: auth } = useNuxtApp();

  const create: OrgCreateCall = async (req) => {
    try {
      return await auth.createOwnOrg(req);
    } catch (e) {
      if (errorCode(e) === 'ORG006') {
        await orgs.load();
        await navigateTo(await resolve(), { replace: true });
      }
      throw e;
    }
  };

  return useOrgCreate({ create, landing: org => resolve(org.slug) });
}
