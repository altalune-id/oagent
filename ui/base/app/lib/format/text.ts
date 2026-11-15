// codePoints counts the Unicode code points in s, the unit the server's length limits use (Go runes, CEL size).
export function codePoints(s: string): number {
  return [...s].length;
}
