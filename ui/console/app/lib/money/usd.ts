const microsPerUsd = 1_000_000n;

// usdText shows a micro-USD amount as a plain dollar amount with at least two decimals.
export function usdText(micros: bigint): string {
  const frac = (micros % microsPerUsd).toString().padStart(6, '0').replace(/0+$/, '').padEnd(2, '0');
  return `${micros / microsPerUsd}.${frac}`;
}

// decimalSeparator is the character a locale writes between whole and fractional digits.
export function decimalSeparator(locale: string): string {
  return new Intl.NumberFormat(locale).formatToParts(1.5).find(p => p.type === 'decimal')?.value ?? '.';
}

// parseUsd reads a typed dollar amount into micro-USD, or null when it is not one with at most six decimals; decimal is the locale's separator.
export function parseUsd(text: string, decimal = '.'): bigint | null {
  const clean = text.trim().replace(/^\$/, '');
  const m = /^(\d+)(?:\.(\d{1,6}))?$/.exec(decimal === '.' ? clean : clean.replace(decimal, '.'));
  if (!m) {
    return null;
  }
  return BigInt(m[1]!) * microsPerUsd + BigInt((m[2] ?? '').padEnd(6, '0'));
}

// perCallMicros reads a typed price per 1,000 calls into micro-USD per call, or null when it has more than three decimals.
export function perCallMicros(text: string, decimal = '.'): bigint | null {
  const micros = parseUsd(text, decimal);
  if (micros === null || micros % 1000n !== 0n) {
    return null;
  }
  return micros / 1000n;
}

// per1kText shows a micro-USD price per call as the dollar price per 1,000 calls.
export function per1kText(micros: bigint): string {
  return usdText(micros * 1000n);
}
