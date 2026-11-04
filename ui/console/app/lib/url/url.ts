// isHttpsUrl reports whether a value is a full https URL, the only scheme secrets are sent to.
export function isHttpsUrl(value: string): boolean {
  return URL.canParse(value) && new URL(value).protocol === 'https:';
}
