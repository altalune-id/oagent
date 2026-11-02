import { siblingURL } from '~/lib/auth/urls';
import { useAuthStore } from '~/stores/auth';

// useUserMenu gives the account menu its signed-in person, sign-out URL and translated labels, wherever the shell shows it.
export function useUserMenu() {
  const { t } = useI18n();
  const auth = useAuthStore();
  const { apiUrl, appVersion } = useRuntimeConfig().public;
  const labels = computed(() => ({
    signOut: t('auth.sign_out'),
    signedInAs: t('auth.signed_in_as'),
    operator: t('auth.operator'),
    apikey: t('auth.source_apikey'),
    basic: t('auth.source_basic'),
    profile: t('profile.title'),
  }));
  return { me: computed(() => auth.me), logoutUrl: siblingURL(apiUrl, '/logout'), labels, version: appVersion };
}
