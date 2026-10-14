// Theme colors only, so every pair keeps its contrast in light and dark themes.
export const avatarColors = [
  'bg-primary text-primary-content',
  'bg-secondary text-secondary-content',
  'bg-accent text-accent-content',
  'bg-info text-info-content',
  'bg-success text-success-content',
  'bg-warning text-warning-content',
  'bg-error text-error-content',
  'bg-neutral text-neutral-content',
] as const;

export type AvatarColor = (typeof avatarColors)[number];

export interface AvatarPerson {
  id: string;
  name: string;
  email: string;
  avatarUrl?: string;
}

export type AvatarView
  = | { kind: 'image', src: string, alt: string }
    | { kind: 'initials', text: string, label: string, color: AvatarColor };

export function initials(name: string, email: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  if (words.length > 0) {
    const first = Array.from(words[0]!)[0]!;
    const last = words.length > 1 ? Array.from(words[words.length - 1]!)[0]! : '';
    return (first + last).toUpperCase();
  }
  const letter = Array.from(email.trim())[0];
  return letter ? letter.toUpperCase() : '?';
}

// FNV-1a over the public id: the same person keeps the same color everywhere.
export function avatarColor(id: string): AvatarColor {
  let hash = 0x811C9DC5;
  for (let i = 0; i < id.length; i++) {
    hash ^= id.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return avatarColors[id ? hash % avatarColors.length : 0]!;
}

export function avatarView(p: AvatarPerson): AvatarView {
  const label = p.name.trim() || p.email;
  if (p.avatarUrl) {
    return { kind: 'image', src: p.avatarUrl, alt: label };
  }
  return { kind: 'initials', text: initials(p.name, p.email), label, color: avatarColor(p.id) };
}
