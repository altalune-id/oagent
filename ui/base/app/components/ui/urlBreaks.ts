const breakAfter = /(\/p\/|\/\.well-known\/)/;

// urlBreaks splits a URL after each /p/ and /.well-known/, where a long URL may wrap.
export function urlBreaks(url: string): string[] {
  const parts: string[] = [];
  let current = '';
  for (const piece of url.split(breakAfter)) {
    current += piece;
    if (breakAfter.test(piece)) {
      parts.push(current);
      current = '';
    }
  }
  if (current || parts.length === 0) {
    parts.push(current);
  }
  return parts;
}
