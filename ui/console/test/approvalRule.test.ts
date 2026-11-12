// @vitest-environment happy-dom
import type { AgentToolConfig } from '~/components/agent/configure/agentConfig';
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import ApprovalRule from '~/components/agent/configure/ApprovalRule.vue';

const args = [{ name: 'confirm', type: 'boolean' as const }, { name: 'status', type: 'string' as const }];
const grant: AgentToolConfig = { name: 'record_expense', requireApproval: true, freeWhen: [] };

function rule(props: Partial<{ grant: AgentToolConfig, args: typeof args, trusted: boolean }> = {}) {
  const w = mount(ApprovalRule, { props: { grant, args, serverName: 'yasaku', trusted: true, ...props } });
  const radio = (m: string) => w.get(`input[aria-label="approvalrule.${m}"]`);
  const changes = () => (w.emitted('change') ?? []).map(e => e[0] as AgentToolConfig);
  return { w, radio, changes };
}

describe('approval rule editor', () => {
  it('is a labelled radio group of the three modes, starting from the grant', () => {
    const { w, radio } = rule();
    expect(w.get('legend').text()).toBe('approvalrule.label');
    expect(w.findAll('.join input[type="radio"]').map(i => i.attributes('aria-label'))).toEqual(['approvalrule.always', 'approvalrule.never', 'approvalrule.unless']);
    expect((radio('always').element as HTMLInputElement).checked).toBe(true);
    expect((rule({ grant: { ...grant, freeWhen: [{ arg: 'confirm', values: ['false'], absent: false }] } }).radio('unless').element as HTMLInputElement).checked).toBe(true);
  });

  it('saves Don\'t ask at once, and Ask unless only once the row is complete, with a one-line summary', async () => {
    const { w, radio, changes } = rule();
    await radio('never').setValue(true);
    expect(changes().at(-1)).toEqual({ ...grant, requireApproval: false, freeWhen: [] });
    await radio('unless').setValue(true);
    expect(changes()).toHaveLength(1);
    const row = w.get('[data-testid="rule-row"]');
    expect((row.findAll('input[type="text"]')[0]!.element as HTMLInputElement).value).toBe('confirm');
    await row.findAll('input[type="text"]')[1]!.setValue('false');
    expect(changes().at(-1)).toEqual({ ...grant, requireApproval: true, freeWhen: [{ arg: 'confirm', values: ['false'], absent: false }] });
    const summary = w.get('[data-testid="rule-summary"]');
    expect(summary.text()).toBe('approvalrule.summary|{"rules":"approvalrule.summary_row|{\\"arg\\":\\"confirm\\",\\"values\\":\\"false\\"}"}');
    expect(w.get('fieldset').attributes('aria-describedby')).toBe(summary.attributes('id'));
  });

  it('warns but saves a row naming an argument the server doesn\'t list, with the recheck hint', async () => {
    const { w, radio, changes } = rule();
    await radio('unless').setValue(true);
    const [arg, values] = w.get('[data-testid="rule-row"]').findAll('input[type="text"]');
    await arg!.setValue('mode');
    await values!.setValue('quick');
    expect(w.text()).toContain('approvalrule.unknown_arg_warning');
    expect(w.text()).toContain('approvalrule.recheck_hint');
    expect(changes().at(-1)!.freeWhen).toEqual([{ arg: 'mode', values: ['"quick"'], absent: false }]);
  });

  it('blocks saving an unknown argument that skips asking when missing, and offers Refresh tools and Turn off', async () => {
    const { w, radio, changes } = rule();
    await radio('unless').setValue(true);
    const row = () => w.get('[data-testid="rule-row"]');
    await row().findAll('input[type="text"]')[0]!.setValue('mode');
    await row().get('input[type="checkbox"]').setValue(true);
    const saved = changes().length;
    expect(row().text()).toContain('approvalrule.unknown_arg_missing_error');
    expect(row().get('input[type="text"]').attributes('aria-invalid')).toBe('true');
    await row().findAll('button').find(b => b.text() === 'approvalrule.refresh_tools')!.trigger('click');
    expect(w.emitted('refresh')).toHaveLength(1);
    expect(changes()).toHaveLength(saved);
    await w.setProps({ args: [...args, { name: 'mode', type: 'string' as const }] });
    expect(row().text()).not.toContain('approvalrule.unknown_arg_missing_error');
    expect(changes().at(-1)!.freeWhen).toEqual([{ arg: 'mode', values: [], absent: true }]);
    await w.setProps({ args });
    await row().findAll('button').find(b => b.text() === 'approvalrule.turn_off_missing')!.trigger('click');
    expect((row().get('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false);
    expect(row().text()).toContain('approvalrule.values_required');
  });

  it('never saves an empty row, even with skip-when-missing on, so a blank rule can never become Don\'t ask', async () => {
    const { w, radio, changes } = rule({ args: [] });
    await radio('unless').setValue(true);
    const row = w.get('[data-testid="rule-row"]');
    expect((row.get('input[type="text"]').element as HTMLInputElement).value).toBe('');
    await row.get('input[type="checkbox"]').setValue(true);
    await w.findAll('button').find(b => b.text().includes('approvalrule.add_condition'))!.trigger('click');
    expect(changes()).toEqual([]);
    expect(row.text()).toContain('approvalrule.arg_required');
  });

  it('adds up to eight conditions, and removing the last one goes back to Ask every time', async () => {
    const { w, radio, changes } = rule();
    await radio('unless').setValue(true);
    const add = () => w.findAll('button').find(b => b.text().includes('approvalrule.add_condition'))!;
    for (let i = 0; i < 8; i++) {
      await add().trigger('click');
    }
    expect(w.findAll('[data-testid="rule-row"]')).toHaveLength(8);
    expect(add().attributes('disabled')).toBeDefined();
    expect(w.text()).toContain('approvalrule.limit_conditions|{"n":8}');
    while (w.findAll('[data-testid="rule-row"]').length) {
      await w.get('[data-testid="rule-row"] button[aria-label="approvalrule.remove_condition"]').trigger('click');
    }
    expect((radio('always').element as HTMLInputElement).checked).toBe(true);
    expect(changes().at(-1)).toEqual({ ...grant, requireApproval: true, freeWhen: [] });
  });

  it('offers only scalar arguments in the picker and as the first row', async () => {
    const { w, radio } = rule({ args: [{ name: 'filter', type: 'other' as const }, ...args] as typeof args });
    expect(w.findAll('datalist option').map(o => o.attributes('value'))).toEqual(['confirm', 'status']);
    await radio('unless').setValue(true);
    expect((w.get('[data-testid="rule-row"] input[type="text"]').element as HTMLInputElement).value).toBe('confirm');
  });

  it('warns on an untrusted server for Don\'t ask and Ask unless, never for Ask every time, and notes a missing tool list', async () => {
    const { w, radio } = rule({ trusted: false, args: [] });
    expect(w.find('[role="alert"]').exists()).toBe(false);
    await radio('never').setValue(true);
    expect(w.get('[role="alert"]').text()).toBe('grants.untrusted_no_ask');
    await radio('unless').setValue(true);
    expect(w.get('[role="alert"]').text()).toBe('grants.untrusted_no_ask');
    expect(w.text()).toContain('approvalrule.no_schema');
  });
});
