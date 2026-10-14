import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const appDir = join(__dirname, '..', '..');

function vueFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) {
      return vueFiles(path);
    }
    return path.endsWith('.vue') ? [path] : [];
  });
}

// The proto rules give the messages, so the browser's own checks must not run first.
describe('forms that show field errors', () => {
  const files = vueFiles(appDir).filter(f => readFileSync(f, 'utf8').includes('fieldErrors'));

  it('finds the forms', () => {
    expect(files.length).toBeGreaterThan(5);
  });

  for (const file of files) {
    it(`${file.slice(appDir.length + 1)} turns native validation off`, () => {
      const forms = readFileSync(file, 'utf8').match(/<form\b[^>]*>/g) ?? [];
      for (const form of forms) {
        expect(form, form).toContain('novalidate');
      }
    });
  }
});
