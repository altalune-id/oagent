import type { Reason } from './policy';
import { policy, violations } from './policy';
import { label, score } from './strength';

type Rule = 'length' | 'common' | 'personal';

const ruleOf: Record<Reason, Rule> = {
  too_short: 'length',
  too_long: 'length',
  common: 'common',
  repeated: 'common',
  sequential: 'common',
  contains_email: 'personal',
  contains_name: 'personal',
};

const messages: Record<Reason, string> = {
  too_short: `Use at least ${policy.minLength} characters.`,
  too_long: `Use at most ${policy.maxLength} characters.`,
  common: 'This password is too common.',
  repeated: 'Avoid a repeated pattern.',
  sequential: 'Avoid a simple sequence.',
  contains_email: 'Don\'t use your email in the password.',
  contains_name: 'Don\'t use your name in the password.',
};

export function enhance(root: ParentNode): void {
  root.querySelectorAll<HTMLElement>('[data-password-field]').forEach(bind);
}

function bind(field: HTMLElement): void {
  if (field.dataset.passwordBound === 'true') {
    return;
  }
  field.dataset.passwordBound = 'true';
  const input = field.querySelector<HTMLInputElement>('input');
  if (!input) {
    return;
  }
  const form = field.closest('form');
  const hint = (name?: string) => (name && form ? form.querySelector<HTMLInputElement>(`[name="${CSS.escape(name)}"]`) : null);
  const email = hint(field.dataset.emailInput);
  const name = hint(field.dataset.nameInput);
  const meter = field.querySelector<HTMLProgressElement>('[data-password-meter]');
  const text = field.querySelector<HTMLElement>('[data-password-score]');
  const update = () => {
    const empty = input.value === '';
    const v = violations(input.value, { email: email?.value, name: name?.value });
    const broken = new Set(v.map(r => ruleOf[r]));
    field.querySelectorAll<HTMLElement>('[data-rule]').forEach((li) => {
      const met = !empty && !broken.has(li.dataset.rule as Rule);
      li.dataset.met = String(met);
      const state = li.querySelector('[data-rule-state]');
      if (state) {
        state.textContent = empty ? '' : met ? ' (met)' : ' (not met)';
      }
    });
    const s = score(input.value, v);
    meter?.classList.toggle('hidden', empty);
    text?.classList.toggle('hidden', empty);
    if (meter) {
      meter.value = s;
    }
    if (text) {
      text.textContent = label(s);
    }
    input.setCustomValidity(!empty && v.length > 0 ? messages[v[0]!] : '');
  };
  for (const el of [input, email, name]) {
    el?.addEventListener('input', update);
  }
}
