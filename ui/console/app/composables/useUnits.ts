// useUnits formats counts, durations and US-dollar cents in the current locale.
export function useUnits() {
  const { locale } = useI18n();
  return {
    count: (n: number) => new Intl.NumberFormat(locale.value).format(n),
    seconds: (s: number) => new Intl.NumberFormat(locale.value, { style: 'unit', unit: 'second', unitDisplay: 'narrow', maximumFractionDigits: 1 }).format(s),
    money: (cents: number) => new Intl.NumberFormat(locale.value, { style: 'currency', currency: 'USD', minimumFractionDigits: 2, maximumFractionDigits: cents < 100 ? 3 : 2 }).format(cents / 100),
  };
}
