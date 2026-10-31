// @vitest-environment happy-dom
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { addIcon } from '@iconify/vue/offline';
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import AppButton from './AppButton.vue';
import CopyButton from './CopyButton.vue';
import { installStandalone } from './standalone';

const dir = dirname(fileURLToPath(import.meta.url));
const files = readdirSync(dir).filter(f => /\.(?:vue|ts)$/.test(f) && !/\.(?:test|stories)\.ts$/.test(f));
const components = files.filter(f => f.endsWith('.vue'));
const templateGlobals = new Set(['$event', '$slots', '$attrs', '$emit', '$props', '$el', '$refs', '$data']);
const builtins = new Set(['Icon', 'NuxtLink', 'Teleport', 'Transition', 'TransitionGroup', 'KeepAlive', 'Suspense', 'Component', 'Slot']);

// known lists every name a file may use without Nuxt: its imports from vue and its siblings, and what it declares itself.
function known(source: string): Set<string> {
  const names = new Set<string>();
  for (const [, spec] of source.matchAll(/import (?:type )?([^'"]+) from '(?:vue|\.[^']*)'/g)) {
    for (const part of spec!.replace(/[{}]/g, ',').split(',')) {
      const name = part.trim().split(/\s+as\s+/).pop();
      if (name) {
        names.add(name);
      }
    }
  }
  for (const [, name] of source.matchAll(/(?:function|const|let|var)\s+(\w+)/g)) {
    names.add(name!);
  }
  for (const [, list] of source.matchAll(/const\s+\{([^}]+)\}\s*=/g)) {
    for (const part of list!.split(',')) {
      names.add(part.split(/[:=]/)[0]!.trim());
    }
  }
  return names;
}

describe('ui components outside Nuxt', () => {
  it.each(files)('%s calls only composables and lifecycle hooks it imports from vue or a sibling', (file) => {
    const source = readFileSync(join(dir, file), 'utf8');
    const names = known(source);
    const calls = [...source.matchAll(/\b((?:use|on)[A-Z]\w*)\s*\(/g)].map(m => m[1]!);
    expect(calls.filter(name => !names.has(name)), file).toEqual([]);
  });

  it.each(files)('%s uses no Nuxt or plugin $global', (file) => {
    const source = readFileSync(join(dir, file), 'utf8');
    const dollars = [...source.matchAll(/(?<![\w$])\$[a-z_]\w*/gi)].map(m => m[0]);
    expect(dollars.filter(name => !templateGlobals.has(name)), file).toEqual([]);
  });

  it.each(components)('%s uses only imported components and the Icon and NuxtLink globals', (file) => {
    const source = readFileSync(join(dir, file), 'utf8');
    const imported = new Set([...source.matchAll(/import (\w+) from '[^']+\.vue'/g)].map(m => m[1]));
    const template = source.slice(source.indexOf('<template>'));
    for (const [, tag] of template.matchAll(/<([A-Z]\w*)[\s/>]/g)) {
      expect(imported.has(tag) || builtins.has(tag!), `${file}: <${tag}>`).toBe(true);
    }
    expect(template.match(/<[a-z]+-[a-z-]+[\s/>]/g), `${file}: kebab-case component`).toBeNull();
  });

  it.each(files)('%s imports only vue, its siblings, lib types and the offline icon runtime', (file) => {
    const source = readFileSync(join(dir, file), 'utf8');
    for (const [, from] of source.matchAll(/from '([^']+)'/g)) {
      expect(from, file).toMatch(/^(?:vue|@iconify\/vue\/offline|\.\/[\w.]+|\.\.\/\.\.\/lib\/[\w/]+)$/);
    }
  });

  it('renders the globals without Nuxt', () => {
    addIcon('lucide:copy', { body: '<path d="M0 0h1v1H0z"/>', width: 24, height: 24 });
    const global = { plugins: [{ install: installStandalone }], stubs: { NuxtLink: false, Icon: false } };
    const link = mount(AppButton, { props: { to: '/console/' }, slots: { default: 'Open' }, global });
    expect(link.get('a').attributes('href')).toBe('/console/');
    const copy = mount(CopyButton, { props: { value: 'x', label: 'Copy x' }, global });
    expect(copy.find('svg').exists()).toBe(true);
  });
});
