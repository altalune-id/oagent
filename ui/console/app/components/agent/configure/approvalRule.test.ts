import { describe, expect, it } from 'vitest';
import { checkRow, modeOf, rowsFrom, scalarArgs, summaryOf, toConditions } from './approvalRule';

const args = [{ name: 'confirm', type: 'boolean' as const }, { name: 'amount', type: 'number' as const }, { name: 'status', type: 'string' as const }];
const t = (key: string, params?: Record<string, unknown>) => (params ? `${key}|${JSON.stringify(params)}` : key);

describe('approval rule', () => {
  it('names the mode from the grant: ask every time, don\'t ask, or ask unless', () => {
    expect(modeOf({ name: 'x', requireApproval: true })).toBe('always');
    expect(modeOf({ name: 'x', requireApproval: true, freeWhen: [] })).toBe('always');
    expect(modeOf({ name: 'x', requireApproval: false, freeWhen: [{ arg: 'a', values: ['1'], absent: false }] })).toBe('never');
    expect(modeOf({ name: 'x', requireApproval: true, freeWhen: [{ arg: 'a', values: ['1'], absent: false }] })).toBe('unless');
  });

  it('shows stored JSON values as typed text and turns text back into JSON by the argument\'s schema type', () => {
    const rows = rowsFrom([{ arg: 'confirm', values: ['false'], absent: false }, { arg: 'status', values: ['"draft"', '"a, b"'], absent: true }]);
    expect(rows).toEqual([{ arg: 'confirm', text: 'false', absent: false }, { arg: 'status', text: 'draft, "a, b"', absent: true }]);
    expect(toConditions([{ arg: 'confirm', text: 'false', absent: false }, { arg: 'amount', text: '1, 2.5', absent: false }, { arg: 'status', text: 'draft, 123', absent: true }, { arg: 'other', text: 'true, x', absent: false }], args)).toEqual([
      { arg: 'confirm', values: ['false'], absent: false },
      { arg: 'amount', values: ['1', '2.5'], absent: false },
      { arg: 'status', values: ['"draft"', '"123"'], absent: true },
      { arg: 'other', values: ['true', '"x"'], absent: false },
    ]);
    expect(rowsFrom(toConditions([{ arg: 'status', text: 'draft, "a, b"', absent: false }], args))).toEqual([{ arg: 'status', text: 'draft, "a, b"', absent: false }]);
  });

  it('refuses a row it cannot save: no argument, a bad name, no values, too many or too long values', () => {
    const ok = { arg: 'confirm', text: 'false', absent: false };
    expect(checkRow(ok, args)).toEqual({ error: '', warning: '' });
    expect(checkRow({ ...ok, arg: '' }, args).error).toBe('approvalrule.arg_required');
    expect(checkRow({ ...ok, arg: '9bad' }, args).error).toBe('approvalrule.arg_invalid');
    expect(checkRow({ ...ok, text: ' ' }, args).error).toBe('approvalrule.values_required');
    expect(checkRow({ ...ok, text: ' ', absent: true }, args).error).toBe('');
    expect(checkRow({ arg: 'status', text: Array.from({ length: 17 }, (_, i) => `v${i}`).join(','), absent: false }, args).error).toBe('approvalrule.limit_values');
    expect(checkRow({ arg: 'status', text: 'x'.repeat(300), absent: false }, args).error).toBe('approvalrule.value_too_long');
  });

  it('warns about an unknown argument or a mistyped value, and blocks an unknown argument that skips asking when missing', () => {
    expect(checkRow({ arg: 'nope', text: 'x', absent: false }, args)).toEqual({ error: '', warning: 'approvalrule.unknown_arg_warning' });
    expect(checkRow({ arg: 'confirm', text: 'maybe', absent: false }, args)).toEqual({ error: '', warning: 'approvalrule.type_mismatch' });
    expect(checkRow({ arg: 'amount', text: 'lots', absent: false }, args)).toEqual({ error: '', warning: 'approvalrule.type_mismatch' });
    expect(checkRow({ arg: 'amount', text: 'false', absent: false }, args).warning).toBe('approvalrule.type_mismatch');
    expect(checkRow({ arg: 'amount', text: '-2.5, 1e3', absent: false }, args).warning).toBe('');
    expect(checkRow({ arg: 'nope', text: '', absent: true }, args)).toEqual({ error: 'approvalrule.unknown_arg_missing_error', warning: '' });
    expect(checkRow({ arg: 'nope', text: '', absent: true }, [])).toEqual({ error: '', warning: '' });
  });

  it('takes whole numbers only for an integer argument, and says a non-scalar argument never matches', () => {
    const typed = [{ name: 'count', type: 'integer' as const }, { name: 'filter', type: 'other' as const }, { name: 'confirm', type: 'boolean' as const }];
    expect(checkRow({ arg: 'count', text: '3', absent: false }, typed)).toEqual({ error: '', warning: '' });
    expect(checkRow({ arg: 'count', text: '2.5', absent: false }, typed).warning).toBe('approvalrule.type_mismatch');
    expect(toConditions([{ arg: 'count', text: '3', absent: false }], typed)[0]!.values).toEqual(['3']);
    expect(checkRow({ arg: 'filter', text: 'x', absent: false }, typed)).toEqual({ error: '', warning: 'approvalrule.non_scalar' });
    expect(scalarArgs(typed).map(a => a.name)).toEqual(['count', 'confirm']);
  });

  it('sums the rule up in one sentence', () => {
    expect(summaryOf([{ arg: 'confirm', text: 'false', absent: false }, { arg: 'status', text: 'draft, final', absent: true }], t)).toBe(
      'approvalrule.summary|{"rules":"approvalrule.summary_row|{\\"arg\\":\\"confirm\\",\\"values\\":\\"false\\"} approvalrule.and approvalrule.summary_row_missing|{\\"arg\\":\\"status\\",\\"values\\":\\"draft approvalrule.or final\\"}"}',
    );
    expect(summaryOf([{ arg: '', text: '', absent: false }], t)).toBe('');
  });
});
