import en from '../../base/i18n/locales/en-US.json';

const copy: Record<string, string> = en;

// t returns the shared English copy for key, filling its {name} params; an unknown key comes back as itself.
export function t(key: string, params: Record<string, unknown> = {}): string {
  return (copy[key] ?? key).replace(/\{(\w+)\}/g, (whole, name: string) => (params[name] === undefined ? whole : String(params[name])));
}
