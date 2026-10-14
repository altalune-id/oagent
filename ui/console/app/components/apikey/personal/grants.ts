export interface Scope {
  name: string;
  level: string;
  labelKey: string;
  personOnly?: boolean;
}

// Same matching as scope.Grants in Go: '*', 'agent:*' or the exact permission.
export function grants(granted: string[], permission: string): boolean {
  if (!permission) {
    return false;
  }
  const resource = permission.split(':')[0];
  return granted.some(g => g === '*' || g === permission || g === `${resource}:*`);
}

// A personal key never holds a person-only permission nor one above the owner's role.
export function mintableForMe(catalog: Scope[], myPermissions: string[]): Scope[] {
  return catalog.filter(s => !s.personOnly && grants(myPermissions, s.name));
}
