import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';

// useOrgCreation says whether "New organization" is offered: only where the deployment allows it, and not to someone who already owns an org (ORG006).
export function useOrgCreation() {
  const auth = useAuthStore();
  const orgs = useOrgsStore();
  const allowed = computed(() => auth.me?.orgCreation === true);
  const ownsOne = computed(() => orgs.items.some(o => o.role === 'owner'));
  const canCreate = computed(() => allowed.value && !ownsOne.value);
  return { allowed, ownsOne, canCreate };
}
