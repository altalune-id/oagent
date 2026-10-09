import scopes from '@shared/scope/scopes.json'

const labelKeys = new Map(scopes.map(s => [s.name, s.labelKey] as const))

export function useScopeLabel() {
  const { t } = useI18n()
  return (name: string) => t(labelKeys.get(name) ?? name)
}
