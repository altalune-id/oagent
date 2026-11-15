const VOID = new Set(['area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input', 'link', 'meta', 'source', 'track', 'wbr']);
const RAW_TEXT = new Set(['script', 'style']);
const MAX_NAME_SCAN = 66;

type Tag = {
  start: number,
  end: number,
  name: string,
  closing: boolean,
  selfClosing: boolean,
};

function* tags(html: string): Generator<Tag> {
  let at = 0;
  let gt = -1;
  for (;;) {
    const start = html.indexOf('<', at);
    if (start === -1) {
      return;
    }
    if (gt <= start) {
      gt = html.indexOf('>', start + 1);
    }
    if (gt === -1) {
      return;
    }
    const head = /^(\/?)([a-z][\w-]*)/i.exec(html.slice(start + 1, Math.min(gt, start + MAX_NAME_SCAN)));
    if (head === null) {
      at = start + 1;
      continue;
    }
    yield { start, end: gt, name: head[2]!.toLowerCase(), closing: head[1] === '/', selfClosing: html[gt - 1] === '/' };
    at = gt + 1;
  }
}

export function tooComplex(html: string, maxDepth: number, maxTags: number): boolean {
  let depth = 0;
  let count = 0;
  for (const t of tags(html)) {
    if (++count > maxTags) {
      return true;
    }
    if (VOID.has(t.name) || t.selfClosing) {
      continue;
    }
    depth += t.closing ? -1 : 1;
    if (depth > maxDepth) {
      return true;
    }
  }
  return false;
}

export function stripTags(html: string): string {
  const parts: string[] = [];
  let at = 0;
  for (const t of tags(html)) {
    if (t.start < at) {
      continue;
    }
    parts.push(html.slice(at, t.start));
    at = t.end + 1;
    if (!t.closing && !t.selfClosing && RAW_TEXT.has(t.name)) {
      const close = new RegExp(`</${t.name}`, 'gi');
      close.lastIndex = at;
      const found = close.exec(html);
      const closeEnd = found === null ? -1 : html.indexOf('>', found.index);
      at = closeEnd === -1 ? html.length : closeEnd + 1;
    }
  }
  const tail = html.slice(at);
  const open = tail.search(/<[a-z/]/i);
  parts.push(open === -1 ? tail : tail.slice(0, open));
  return parts.join(' ').replace(/\s+/g, ' ').trim();
}
