import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { iconSubset, usedIcons } from './iconSubset';

function sources() {
  const dir = mkdtempSync(join(tmpdir(), 'icons-'));
  mkdirSync(join(dir, 'views'));
  writeFileSync(join(dir, 'views', 'A.vue'), `<Icon name="lucide:copy" /> <Icon :name="ok ? 'lucide:check' : 'lucide:building-2'" />`);
  writeFileSync(join(dir, 'b.ts'), `const icon = 'lucide:copy';`);
  writeFileSync(join(dir, 'b.test.ts'), `'lucide:trash-2'`);
  return dir;
}

describe('icon subset', () => {
  it('collects every lucide icon the sources name, once, skipping tests', () => {
    expect(usedIcons([sources()])).toEqual(['building-2', 'check', 'copy']);
  });

  it('bundles only those icons, resolving aliases to their parent', () => {
    const plugin = iconSubset([sources()]) as any;
    const id = plugin.resolveId('virtual:icons');
    const fail = (m: string) => {
      throw new Error(m);
    };
    const watched: string[] = [];
    const code: string = plugin.load.call({ error: fail, addWatchFile: (f: string) => watched.push(f) }, id);
    expect(watched).toHaveLength(2);
    const set = JSON.parse(code.replace(/^export default |;$/g, ''));
    expect(Object.keys(set.icons).length).toBeLessThanOrEqual(3);
    expect(set.icons.copy).toBeDefined();
    expect(set.aliases['building-2']).toBeDefined();
    expect(plugin.load.call({}, 'other')).toBeUndefined();
  });
});
