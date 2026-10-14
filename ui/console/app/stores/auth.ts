import type { WhoamiResponse } from '~~/gen/auth/v1/auth_pb';

// me stays null when whoami fails (nobody signed in); before credentials are required that console still works.
export const useAuthStore = defineStore('auth', () => {
  const me = ref<WhoamiResponse | null>(null);
  const loaded = ref(false);
  const { $authClient: client } = useNuxtApp();
  let inflight: Promise<void> | null = null;

  async function fetchMe() {
    try {
      me.value = await client.whoami({});
    } catch {
      me.value = null;
    } finally {
      loaded.value = true;
    }
  }

  function load() {
    inflight ??= fetchMe().then(() => {
      if (!me.value) {
        inflight = null;
      }
    });
    return inflight;
  }

  return { me, loaded, load };
});
