const microsPerUsd = 1_000_000n;

// usdText shows a micro-USD amount as a plain dollar amount with at least two decimals.
export function usdText(micros: bigint): string {
  const frac = (micros % microsPerUsd).toString().padStart(6, '0').replace(/0+$/, '').padEnd(2, '0');
  return `${micros / microsPerUsd}.${frac}`;
}

// parseUsd reads a typed dollar amount into micro-USD, or null when it is not one with at most six decimals.
export function parseUsd(text: string): bigint | null {
  const m = /^(\d+)(?:\.(\d{1,6}))?$/.exec(text.trim().replace(/^\$/, ''));
  if (!m) {
    return null;
  }
  return BigInt(m[1]!) * microsPerUsd + BigInt((m[2] ?? '').padEnd(6, '0'));
}
