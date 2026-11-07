// isWebUrl reports whether a value is a full http or https URL; the server decides whether http is allowed.
export function isWebUrl(value: string): boolean {
  return URL.canParse(value) && ['https:', 'http:'].includes(new URL(value).protocol) && !!new URL(value).host;
}
