import { grants } from '~/components/apikey/personal/grants';
import { useAuthStore } from '~/stores/auth';

export interface Caller {
  operator?: boolean;
  permissions?: string[];
}

// can mirrors the server's permission check for the signed-in person; with nobody signed in the server decides.
export function can(me: Caller | null | undefined, permission: string): boolean {
  return !me || !!me.operator || grants(me.permissions ?? [], permission);
}

// useCan follows the auth store and reports whether the signed-in person holds permission.
export function useCan(permission: string) {
  const auth = useAuthStore();
  return computed(() => can(auth.me, permission));
}
