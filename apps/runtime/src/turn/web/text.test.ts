import { describe, expect, it } from 'vitest';

import { stripTags, tooComplex } from './text.ts';

const hostile = [
  '<a '.repeat(87_382),
  `<script>${'<a '.repeat(87_000)}`,
  '<'.repeat(262_144),
  `${'< '.repeat(131_071)}>`,
  `${'<div>'.repeat(10)}${'<a href="x'.repeat(26_000)}`,
];

describe('stripTags', () => {
  it('drops tags, script and style bodies, and collapses whitespace', () => {
    expect(stripTags('<p>Hello <b>big</b></p>\n<script>evil()</script><style>p{}</style><SCRIPT type="x">a<b</SCRIPT> world')).toBe('Hello big world');
  });

  it('keeps text after a lone < and drops an unterminated tag', () => {
    expect(stripTags('a < b and <i>c</i> <unclosed')).toBe('a < b and c');
    expect(stripTags('x<script>never closed')).toBe('x');
  });

  it.each(hostile.map((h, i) => [i, h] as const))('runs in linear time on hostile input %i', (_i, html) => {
    const started = performance.now();
    stripTags(html);
    expect(performance.now() - started).toBeLessThan(1000);
  });
});

describe('linear time at 1 MB', () => {
  it('scans 1 MB of unclosed tags or lone brackets within 2 s', () => {
    for (const html of ['<a '.repeat(349_526), `${'< '.repeat(524_287)}>`]) {
      const started = performance.now();
      tooComplex(html, 256, 10_000);
      stripTags(html);
      expect(performance.now() - started).toBeLessThan(2000);
    }
  });
});

describe('tooComplex', () => {
  it('refuses more than the tag limit, void tags included', () => {
    expect(tooComplex('<br>'.repeat(10_001), 256, 10_000)).toBe(true);
    expect(tooComplex('<br>'.repeat(10_000), 256, 10_000)).toBe(false);
    expect(tooComplex('<p>a</p>'.repeat(5001), 256, 10_000)).toBe(true);
  });

  it('counts open tags, ignores void and self-closing ones, and closes on end tags', () => {
    expect(tooComplex('<div>'.repeat(257), 256, 10_000)).toBe(true);
    expect(tooComplex('<div>'.repeat(256), 256, 10_000)).toBe(false);
    expect(tooComplex(`${'<div></div>'.repeat(1000)}`, 256, 10_000)).toBe(false);
    expect(tooComplex(`${'<br>'.repeat(300)}${'<span class="i"/>'.repeat(300)}${'<IMG src=x>'.repeat(300)}`, 256, 10_000)).toBe(false);
    expect(tooComplex('<p>a < b</p>'.repeat(300), 256, 10_000)).toBe(false);
  });

  it.each(hostile.map((h, i) => [i, h] as const))('runs in linear time on hostile input %i', (_i, html) => {
    const started = performance.now();
    tooComplex(html, 256, 10_000);
    expect(performance.now() - started).toBeLessThan(1000);
  });
});
