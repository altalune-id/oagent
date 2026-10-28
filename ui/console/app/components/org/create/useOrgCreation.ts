import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';

// useOrgCreation says whether "New organization" is offered: where the deployment allows it, only to a person in no org until there is an org switch.
export function useOrgCreation() {
  const auth = useAuthStore();
  const orgs = useOrgsStore();
  const allowed = computed(() => auth.me?.orgCreation === true);
  const ownsOne = computed(() => orgs.items.some(o => o.role === 'owner'));
  const canCreate = computed(() => allowed.value && orgs.items.length === 0);
  return { allowed, ownsOne, canCreate };
}
