import { beforeEach, describe, expect, it } from 'vitest';
import { enhance } from './field';

function render(): void {
  document.body.innerHTML = `
    <form>
      <input name="name" value="">
      <input name="email" type="email" value="">
      <div data-password-field data-email-input="email" data-name-input="name">
        <input type="password" name="password">
        <progress class="hidden" max="4" value="0" data-password-meter></progress>
        <p class="hidden" data-password-score></p>
        <ul data-password-rules>
          <li data-rule="length"><span data-rule-state></span></li>
          <li data-rule="common"><span data-rule-state></span></li>
          <li data-rule="personal"><span data-rule-state></span></li>
        </ul>
      </div>
    </form>`;
}

const el = <T extends Element>(selector: string) => document.querySelector<T>(selector)!;
const input = () => el<HTMLInputElement>('input[name="password"]');
const meter = () => el<HTMLProgressElement>('[data-password-meter]');
const scoreText = () => el<HTMLElement>('[data-password-score]');
const met = (rule: string) => el<HTMLElement>(`[data-rule="${rule}"]`).dataset.met;
const spoken = (rule: string) => el<HTMLElement>(`[data-rule="${rule}"] [data-rule-state]`).textContent;

function type(target: HTMLInputElement, value: string): void {
  target.value = value;
  target.dispatchEvent(new Event('input'));
}

describe('enhance', () => {
  beforeEach(() => {
    render();
    enhance(document);
  });

  it('flags a short password and blocks submit', () => {
    type(input(), 'abc');
    expect(met('length')).toBe('false');
    expect(meter().value).toBe(0);
    expect(scoreText().textContent).toBe('Too weak');
    expect(input().validationMessage).toBe('Use at least 12 characters.');
    expect(spoken('length')).toBe(' (not met)');
    expect(spoken('personal')).toBe(' (met)');
  });

  it('flags the email in the password', () => {
    type(el<HTMLInputElement>('input[name="email"]'), 'alice@example.com');
    type(input(), 'alice-is-great-2026');
    expect(met('personal')).toBe('false');
    expect(met('length')).toBe('true');
    expect(input().validationMessage).toBe('Don\'t use your email in the password.');
  });

  it('passes a strong password', () => {
    type(input(), 'correct horse battery staple');
    expect(['length', 'common', 'personal'].map(met)).toEqual(['true', 'true', 'true']);
    expect(meter().value).toBeGreaterThanOrEqual(2);
    expect(scoreText().textContent).toBe('Strong');
    expect(input().validationMessage).toBe('');
  });

  it('re-checks when a hint input changes', () => {
    type(input(), 'maria-loves-tea');
    expect(met('personal')).toBe('true');
    type(el<HTMLInputElement>('input[name="name"]'), 'Ana Maria');
    expect(met('personal')).toBe('false');
  });

  it('hides the meter and the score while empty', () => {
    type(input(), 'abc');
    expect(meter().classList.contains('hidden')).toBe(false);
    type(input(), '');
    expect(meter().classList.contains('hidden')).toBe(true);
    expect(scoreText().classList.contains('hidden')).toBe(true);
    expect(met('length')).toBe('false');
    expect(spoken('length')).toBe('');
    expect(input().validationMessage).toBe('');
  });

  it('binds a field once', () => {
    enhance(document);
    let updates = 0;
    new MutationObserver(records => (updates += records.length)).observe(scoreText(), { childList: true });
    type(input(), 'correct horse battery staple');
    return Promise.resolve().then(() => expect(updates).toBe(1));
  });

  it('works without hint inputs or a form', () => {
    document.body.innerHTML = '<div data-password-field><input type="password"></div>';
    enhance(document);
    type(input2(), 'abc');
    expect(input2().validationMessage).toBe('Use at least 12 characters.');
  });
});

function input2(): HTMLInputElement {
  return el<HTMLInputElement>('input');
}
