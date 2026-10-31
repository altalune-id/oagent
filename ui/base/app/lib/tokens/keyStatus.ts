export type ExpiryState = { kind: 'none' } | { kind: 'ok' } | { kind: 'soon', days: number } | { kind: 'expired' };

const dayMs = 86_400_000;
const soonDays = 7;

// expiryState tells the list which badge an expiry date earns at now.
export function expiryState(expiresAt: Date | undefined, now: Date): ExpiryState {
  if (!expiresAt) {
    return { kind: 'none' };
  }
  const left = expiresAt.getTime() - now.getTime();
  if (left <= 0) {
    return { kind: 'expired' };
  }
  const days = Math.ceil(left / dayMs);
  return days <= soonDays ? { kind: 'soon', days } : { kind: 'ok' };
}

export type KeyStatus = 'active' | 'expired' | 'revoked';

// keyStatus is what the Status column shows; an expired key no longer works, so only an active one can be revoked.
export function keyStatus(revoked: boolean, expiry: ExpiryState): KeyStatus {
  if (revoked) {
    return 'revoked';
  }
  return expiry.kind === 'expired' ? 'expired' : 'active';
}

const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * dayMs],
  ['month', 30 * dayMs],
  ['day', dayMs],
  ['hour', 3_600_000],
  ['minute', 60_000],
];

// relativeTime renders how long ago date was, in locale; an empty string means never.
export function relativeTime(date: Date | undefined, now: Date, locale: string): string {
  if (!date) {
    return '';
  }
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto', style: 'long' });
  const ago = now.getTime() - date.getTime();
  for (const [unit, ms] of units) {
    if (ago >= ms) {
      return rtf.format(-Math.floor(ago / ms), unit);
    }
  }
  return rtf.format(0, 'second');
}
