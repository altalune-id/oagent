// isPerson reports a signed-in person with a browser session (a password or the OIDC provider), never an API key or basic auth.
export function isPerson(me: { source?: string, userId?: string } | null | undefined): boolean {
  return !!me && (me.source === 'local' || me.source === 'oidc') && !!me.userId;
}
