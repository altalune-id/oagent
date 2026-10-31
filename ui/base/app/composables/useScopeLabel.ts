import scopes from '@shared/scope/scopes.json';

const labelKeys = new Map(scopes.map(s => [s.name, s.labelKey] as const));

// useScopeLabel returns a scope label function; a scope outside the catalog shows its raw name, not a missing key.
export function useScopeLabel() {
  const { $i18n } = useNuxtApp();
  return (name: string) => {
    const key = labelKeys.get(name);
    return key ? $i18n.t(key) : name;
  };
}
