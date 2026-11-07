import { IdentityKind } from '#gen/enduser/v1/enduser_pb';

type Translate = (key: string, params?: Record<string, unknown>) => string;

export interface PersonLabelParts {
  displayName: string;
  identityKind: IdentityKind;
  identityHint: string;
}

const kindKeys: Record<IdentityKind, string> = {
  [IdentityKind.UNSPECIFIED]: 'person.kind_unknown',
  [IdentityKind.API_USER]: 'person.kind_api_user',
  [IdentityKind.API_KEY]: 'person.kind_api_key',
  [IdentityKind.WA_PHONE]: 'person.kind_wa_phone',
  [IdentityKind.WA_LID]: 'person.kind_wa_lid',
  [IdentityKind.WIDGET_ANON]: 'person.kind_widget_anon',
  [IdentityKind.WIDGET_SUB]: 'person.kind_widget_sub',
};

// identityKindLabel names how a person reached the agent, in the viewer's locale.
export function identityKindLabel(kind: IdentityKind, t: Translate): string {
  return t(kindKeys[kind] ?? kindKeys[IdentityKind.UNSPECIFIED]);
}

// personLabel shows the display name, else the identity kind with its masked hint ("WhatsApp …90"); the server never sends a full number.
export function personLabel(p: PersonLabelParts, t: Translate): string {
  const name = p.displayName.trim();
  if (name) {
    return name;
  }
  const kind = identityKindLabel(p.identityKind, t);
  return p.identityHint ? t('person.masked', { kind, hint: p.identityHint }) : kind;
}
