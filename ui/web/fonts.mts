// Copies the bundled sign-in page fonts and their licenses from @fontsource into internal/web/static/fonts.
import { copyFile, mkdir } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';

const out = '../../internal/web/static/fonts';
const require = createRequire(import.meta.url);

function copies(name: string): Promise<void>[] {
  const pkg = dirname(require.resolve(`@fontsource-variable/${name}/package.json`));
  const fonts = ['latin', 'latin-ext'].map(subset => `${name}-${subset}-wght-normal.woff2`);
  return [
    copyFile(join(pkg, 'LICENSE'), join(out, `${name}-OFL.txt`)),
    ...fonts.map(file => copyFile(join(pkg, 'files', file), join(out, file))),
  ];
}

void mkdir(out, { recursive: true }).then(() => Promise.all(['instrument-sans', 'jetbrains-mono'].flatMap(copies)));
