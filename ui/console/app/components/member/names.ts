export { initials } from '#base/components/ui/avatar';

export function displayName(m: { name: string, email: string }): string {
  return m.name.trim() || m.email;
}
