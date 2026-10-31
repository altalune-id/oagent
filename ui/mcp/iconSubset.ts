import type { Plugin } from 'vite';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { createRequire } from 'node:module';
import { join } from 'node:path';

const id = 'virtual:icons';
const lucideName = /lucide:([a-z0-9-]+)/g;

// sourceFiles lists the .vue and .ts files under dirs, tests left out.
export function sourceFiles(dirs: string[]): string[] {
  const files: string[] = [];
  const walk = (dir: string) => {
    for (const entry of readdirSync(dir)) {
      const path = join(dir, entry);
      if (statSync(path).isDirectory()) {
        walk(path);
      } else if (/\.(?:vue|ts)$/.test(entry) && !entry.endsWith('.test.ts')) {
        files.push(path);
      }
    }
  };
  dirs.forEach(walk);
  return files;
}

// usedIcons collects every lucide icon named in the .vue and .ts files under dirs.
export function usedIcons(dirs: string[]): string[] {
  const names = new Set<string>();
  for (const file of sourceFiles(dirs)) {
    for (const [, name] of readFileSync(file, 'utf8').matchAll(lucideName)) {
      names.add(name!);
    }
  }
  return [...names].sort();
}

// iconSubset serves virtual:icons, an Iconify collection holding only the lucide icons the sources use.
export function iconSubset(dirs: string[]): Plugin {
  return {
    name: 'oagent-icon-subset',
    resolveId: source => (source === id ? `\0${id}` : undefined),
    load(resolved) {
      if (resolved !== `\0${id}`) {
        return undefined;
      }
      const lucide = JSON.parse(readFileSync(createRequire(import.meta.url).resolve('@iconify-json/lucide/icons.json'), 'utf8'));
      const icons: Record<string, unknown> = {};
      const aliases: Record<string, { parent: string }> = {};
      for (const file of sourceFiles(dirs)) {
        this.addWatchFile(file);
      }
      for (const name of usedIcons(dirs)) {
        const alias = lucide.aliases?.[name];
        const parent = alias?.parent ?? name;
        if (!lucide.icons[parent]) {
          this.error(`unknown icon lucide:${name}`);
        }
        icons[parent] = lucide.icons[parent];
        if (alias) {
          aliases[name] = alias;
        }
      }
      return `export default ${JSON.stringify({ prefix: 'lucide', icons, aliases, width: lucide.width, height: lucide.height })};`;
    },
  };
}
