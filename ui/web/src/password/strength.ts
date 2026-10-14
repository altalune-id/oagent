import type { Reason } from './policy';

export type Score = 0 | 1 | 2 | 3 | 4;

const classes = [/\p{Ll}/u, /\p{Lu}/u, /\p{N}/u, /[^\p{L}\p{N}]/u];

export function score(plain: string, v: Reason[]): Score {
  if (v.length > 0) {
    return 0;
  }
  const n = [...plain].length;
  const kinds = classes.filter(re => re.test(plain)).length;
  if (n >= 20 || (n >= 16 && kinds >= 3)) {
    return 4;
  }
  if (n >= 16 || (n >= 14 && kinds >= 3)) {
    return 3;
  }
  if (n >= 14 || kinds >= 3) {
    return 2;
  }
  return 1;
}

const labels = ['Too weak', 'Weak', 'Fair', 'Good', 'Strong'] as const;

export function label(s: Score): string {
  return labels[s];
}
