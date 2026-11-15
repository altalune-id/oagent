import { describe, expect, it } from 'vitest';
import { renderMarkdown } from './markdown';

describe('renderMarkdown', () => {
  it('escapes raw HTML instead of rendering it', () => {
    const out = renderMarkdown('<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>');
    expect(out).not.toContain('<script');
    expect(out).not.toContain('<img');
    expect(out).toContain('&lt;script&gt;');
  });

  it('drops a javascript: link and opens safe links in a new tab without an opener', () => {
    expect(renderMarkdown('[x](javascript:alert(1))')).not.toContain('href');
    expect(renderMarkdown('[docs](https://example.com)')).toContain('<a href="https://example.com" class="link link-primary" target="_blank" rel="noopener noreferrer">docs</a>');
  });

  it.each([
    ['a mixed-case scheme', '[x](JaVaScRiPt:alert(1))'],
    ['an entity-encoded scheme', '[x](&#106;avascript:alert(1))'],
    ['a vbscript: link', '[x](vbscript:msgbox(1))'],
    ['a data:text/html link', '[x](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)'],
    ['an autolink', '<javascript:alert(1)>'],
    ['a reference-style link', '[x][r]\n\n[r]: javascript:alert(1)'],
  ])('renders %s with no href', (_, src) => {
    const out = renderMarkdown(src);
    expect(out).not.toContain('href');
    expect(out).not.toContain('<a');
  });

  it.each([
    ['an img with onerror', '<img src=x onerror=alert(1)>', '&lt;img src=x onerror=alert(1)&gt;'],
    ['a script', '<script>alert(1)</script>', '&lt;script&gt;alert(1)&lt;/script&gt;'],
  ])('escapes %s', (_, src, escaped) => {
    const out = renderMarkdown(src);
    expect(out).not.toMatch(/<(img|script)/);
    expect(out).toContain(escaped);
  });

  it('leaves bare URLs as text and images off', () => {
    expect(renderMarkdown('see https://example.com')).not.toContain('<a');
    expect(renderMarkdown('![x](https://example.com/a.png)')).not.toContain('<img');
  });
});
