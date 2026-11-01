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

  it('keeps muted text at /70, the lowest opacity that passes AA in every theme', () => {
    expect(offenders(/\btext-base-content\/[1-6]\d\b/)).toEqual([]);
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

type RGB = [number, number, number];

function oklchToSRGB(value: string): RGB {
  const [l, c, h] = value.match(/oklch\(\s*([\d.]+)%\s+([\d.]+)\s+([\d.]+)/)!.slice(1).map(Number) as [number, number, number];
  const L = l / 100;
  const a = c * Math.cos((h * Math.PI) / 180);
  const b = c * Math.sin((h * Math.PI) / 180);
  const lms = [
    (L + 0.3963377774 * a + 0.2158037573 * b) ** 3,
    (L - 0.1055613458 * a - 0.0638541728 * b) ** 3,
    (L - 0.0894841775 * a - 1.291485548 * b) ** 3,
  ] as const;
  const linear = [
    4.0767416621 * lms[0] - 3.3077115913 * lms[1] + 0.2309699292 * lms[2],
    -1.2684380046 * lms[0] + 2.6097574011 * lms[1] - 0.3413193965 * lms[2],
    -0.0041960863 * lms[0] - 0.7034186147 * lms[1] + 1.707614701 * lms[2],
  ];
  return linear.map(x => Math.min(1, Math.max(0, x <= 0.0031308 ? 12.92 * x : 1.055 * x ** (1 / 2.4) - 0.055))) as RGB;
}

function luminance(rgb: RGB): number {
  const [r, g, b] = rgb.map(c => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)) as RGB;
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(fg: RGB, bg: RGB, alpha = 1): number {
  const mixed = fg.map((f, i) => alpha * f + (1 - alpha) * bg[i]!) as RGB;
  const [x, y] = [luminance(mixed), luminance(bg)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

describe('guard: theme contrast', () => {
  const theme = readFileSync(join(ui, 'theme.css'), 'utf8');
  const themes = [...theme.matchAll(/name: '([\w-]+)';([\s\S]*?)\n\}/g)].map(([, name, body]) => ({
    name: name!,
    colors: Object.fromEntries([...body!.matchAll(/--color-([\w-]+):\s*(oklch\([^)]*\))/g)].map(([, k, v]) => [k!, oklchToSRGB(v!)])),
  }));

  it('reads every theme', () => {
    expect(themes.length).toBe(14);
    expect(contrast([0, 0, 0], [1, 1, 1])).toBeCloseTo(21);
  });

  it('keeps muted text, *-content and status text at WCAG AA (4.5:1)', () => {
    const failures: string[] = [];
    for (const { name, colors: c } of themes) {
      const check = (label: string, ratio: number) => {
        if (ratio < 4.5) {
          failures.push(`${name}: ${label} ${ratio.toFixed(2)}`);
        }
      };
      for (const bg of ['base-100', 'base-200', 'base-300']) {
        check(`base-content/70 on ${bg}`, contrast(c['base-content']!, c[bg]!, 0.7));
      }
      for (const k of ['primary', 'secondary', 'accent', 'neutral', 'info', 'success', 'warning', 'error']) {
        check(`${k}-content on ${k}`, contrast(c[`${k}-content`]!, c[k]!));
      }
      for (const k of ['info', 'success', 'warning', 'error']) {
        check(`text-${k} on base-100`, contrast(c[k]!, c['base-100']!));
      }
    }
    expect(failures).toEqual([]);
  });
});
