import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const apps = ['base', 'console'];
const ui = fileURLToPath(new URL('../../', import.meta.url));

function sources(dir: string, ext: RegExp): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return sources(path, ext);
    }
    return ext.test(entry.name) && !entry.name.endsWith('.test.ts') ? [path] : [];
  });
}

const files = apps.flatMap(app => sources(join(ui, app, 'app'), /\.(?:vue|ts)$/).map(path => ({ app, path: path.slice(ui.length), text: readFileSync(path, 'utf8') })));
const offenders = (re: RegExp) => files.filter(f => re.test(f.text)).map(f => f.path);

describe('guard: markup in every app', () => {
  it('finds the sources of base and the console', () => {
    for (const app of apps) {
      expect(files.filter(f => f.app === app).length, app).toBeGreaterThan(5);
    }
  });

  it('uses no daisyUI 4 bordered or small form controls', () => {
    expect(offenders(/\b(?:input|select|textarea|file-input)-(?:bordered|sm|xs)\b/)).toEqual([]);
  });

  it('uses the daisyUI 5 row-hover, not the removed table hover class', () => {
    expect(offenders(/<tr\s(?:[^>]*\s)?class="(?:[^"]*\s)?hover(?:\s[^"]*)?"/)).toEqual([]);
  });

  it('keeps text neutral: error, warning, success and info colours go on icons only', () => {
    expect(offenders(/<(?:p|span|div|dd|li|label)\s[^>]*\bclass="[^"]*\btext-(?:error|warning|success|info)\b/)).toEqual([]);
    expect(offenders(/<(?:p|span|div|dd|li|label)\s[^>]*:class="[^"]*'text-(?:error|warning|success|info)'/)).toEqual([]);
  });

  it('keeps destructive ghost buttons neutral with a red icon', () => {
    expect(offenders(/<(?:button|NuxtLink|label|a)\s[^>]*\bclass="(?:[^"]*\s)?btn\s(?:[^"]*\s)?text-(?:error|warning|success|info)\b/)).toEqual([]);
  });

  it('opens every focus dropdown from a focusable trigger, never a <button> Safari will not focus', () => {
    expect(offenders(/class="dropdown\b[^"]*"[^>]*>\s*<button\b/)).toEqual([]);
    const own = files.filter(f => /class="dropdown[\s"]/.test(f.text)).map(f => f.path);
    expect(own).toEqual(['base/app/components/ui/AppDropdown.vue']);
  });

  it('gives small checkboxes square corners so they do not look like radios', () => {
    const round = files.flatMap(f => (f.text.match(/class="checkbox checkbox-(?:xs|sm)\b[^"]*"/g) ?? []).filter(c => !c.includes('rounded-sm')).map(c => `${f.path}: ${c}`));
    expect(round).toEqual([]);
  });

  it('announces page spinners through LoadingStatus and hides decorative ones from screen readers', () => {
    expect(offenders(/class="loading loading-spinner loading-sm"(?![^>]*aria-hidden="true")/)).toEqual([]);
    expect(offenders(/aria-busy="true"/)).toEqual([]);
  });
});

describe('guard: forms that show field errors', () => {
  const forms = files.filter(f => f.path.endsWith('.vue') && f.text.includes('fieldErrors'));

  it('finds the forms in the console', () => {
    expect(forms.some(f => f.app === 'console')).toBe(true);
  });

  it('turn native validation off, so the proto rules give the messages', () => {
    const bad = forms.flatMap(f => (f.text.match(/<form\b[^>]*>/g) ?? []).filter(form => !form.includes('novalidate')).map(form => `${f.path}: ${form}`));
    expect(bad).toEqual([]);
  });
});

describe('guard: theme contrast', () => {
  it('darkens the oagent-dark primary for button text contrast', () => {
    const theme = readFileSync(join(ui, 'theme.css'), 'utf8');
    const dark = theme.slice(theme.indexOf('name: \'oagent-dark\''));
    expect(dark.slice(0, dark.indexOf('}'))).toContain('--color-primary: oklch(55% 0.233 277.117);');
  });
});
