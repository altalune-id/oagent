import { useActiveOrg } from '~/composables/useActiveOrg';

export default defineNuxtRouteMiddleware(async (to) => {
  const slug = to.params.org;
  if (typeof slug === 'string' && slug) {
    await useActiveOrg().enter(slug);
  }
});
