const units = ['B', 'KiB', 'MiB', 'GiB'];

// formatBytes shows a byte count in B, KiB, MiB or GiB (1024 steps) in the locale.
export function formatBytes(bytes: number, locale: string): string {
  let value = Number.isFinite(bytes) && bytes > 0 ? bytes : 0;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(value)} ${units[unit]}`;
}
