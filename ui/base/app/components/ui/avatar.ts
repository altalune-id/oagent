export interface AvatarPerson {
  id: string;
  name: string;
  email: string;
  avatarUrl?: string;
}

export type AvatarView
  = | { kind: 'image', src: string, alt: string }
    | { kind: 'initials', text: string, label: string };

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

export function avatarView(p: AvatarPerson): AvatarView {
  const label = p.name.trim() || p.email;
  if (p.avatarUrl) {
    return { kind: 'image', src: p.avatarUrl, alt: label };
  }
  return { kind: 'initials', text: initials(p.name, p.email), label };
}
