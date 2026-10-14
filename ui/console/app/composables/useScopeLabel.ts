import scopes from '@shared/scope/scopes.json';

const labelKeys = new Map(scopes.map(s => [s.name, s.labelKey] as const));

// useScopeLabel returns a scope label function; a scope outside the catalog shows its raw name, not a missing key.
export function useScopeLabel() {
  const { t } = useI18n();
  return (name: string) => {
    const key = labelKeys.get(name);
    return key ? t(key) : name;
  };
}
