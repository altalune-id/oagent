import { t } from '../copy';

// roles labels each org role, for badges and the invite view.
export const roles = {
  owner: { label: t('members.role_owner'), tone: 'info' },
  admin: { label: t('members.role_admin'), tone: 'neutral' },
  member: { label: t('members.role_member'), tone: 'ghost' },
} as const;

export function roleLabel(role: string | undefined): string {
  if (!role) {
    return '';
  }
  return role in roles ? roles[role as keyof typeof roles].label : role;
}
