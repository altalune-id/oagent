// useUnits formats counts, durations, media lengths and US-dollar cents in the current locale.
export function useUnits() {
  const { locale } = useI18n();
  const money = (cents: number) => new Intl.NumberFormat(locale.value, { style: 'currency', currency: 'USD', minimumFractionDigits: 2, maximumFractionDigits: cents < 100 ? 3 : 2 }).format(cents / 100);
  const tiny = (cents: number) => cents > 0 && cents < 1;
  return {
    count: (n: number) => new Intl.NumberFormat(locale.value).format(n),
    seconds: (s: number) => new Intl.NumberFormat(locale.value, { style: 'unit', unit: 'second', unitDisplay: 'narrow', maximumFractionDigits: 1 }).format(s),
    clock: (s: number) => `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`,
    money,
    cost: (cents: number) => (tiny(cents) ? `< ${money(1)}` : money(cents)),
    costTitle: (cents: number) => (tiny(cents) ? new Intl.NumberFormat(locale.value, { style: 'currency', currency: 'USD', minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(cents / 100) : undefined),
  };
}
