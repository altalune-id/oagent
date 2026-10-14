export { initials } from '~/components/ui/avatar';

export function displayName(m: { name: string, email: string }): string {
  return m.name.trim() || m.email;
}
