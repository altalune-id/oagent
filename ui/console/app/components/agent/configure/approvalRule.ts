import type { AgentToolConfig, FreeCondition } from './agentConfig';
import type { McpToolArg } from '~/stores/mcpServers';

type Translate = (key: string, params?: Record<string, unknown>) => string;

export type RuleMode = 'always' | 'never' | 'unless';

export interface RuleRow {
  arg: string;
  text: string;
  absent: boolean;
}

export const maxConditions = 8;
const maxValues = 16;
const maxValueBytes = 256;
const argName = /^[A-Z_]\w{0,63}$/i;

// scalarArgs lists the arguments a condition can usefully name: booleans, strings and numbers, never objects, arrays or untyped ones.
export function scalarArgs(args: McpToolArg[]): McpToolArg[] {
  return args.filter(a => a.type !== 'other');
}

// modeOf names a granted tool's approval: asked every time, never asked, or asked unless its free_when rule holds.
export function modeOf(t: AgentToolConfig): RuleMode {
  if (!t.requireApproval) {
    return 'never';
  }
  return t.freeWhen?.length ? 'unless' : 'always';
}

function shown(json: string): string {
  try {
    const v: unknown = JSON.parse(json);
    if (typeof v !== 'string') {
      return json;
    }
    return /[,"]/.test(v) || v !== v.trim() ? JSON.stringify(v) : v;
  } catch {
    return json;
  }
}

function tokens(text: string): string[] {
  return [...text.matchAll(/\s*("(?:[^"\\]|\\.)*"|[^,]+)/g)].map(m => m[1]!.trim()).filter(Boolean);
}

function jsonOf(token: string, type: McpToolArg['type'] | undefined): string {
  if (token.startsWith('"') && token.endsWith('"') && token.length > 1) {
    try {
      return JSON.stringify(JSON.parse(token));
    } catch {
      return JSON.stringify(token);
    }
  }
  if (type === 'string') {
    return JSON.stringify(token);
  }
  try {
    const v: unknown = JSON.parse(token);
    return typeof v === 'boolean' || typeof v === 'number' ? JSON.stringify(v) : JSON.stringify(token);
  } catch {
    return JSON.stringify(token);
  }
}

// rowsFrom turns stored conditions into editable rows, showing each JSON value as the text an admin would type.
export function rowsFrom(conditions: FreeCondition[] = []): RuleRow[] {
  return conditions.map(c => ({ arg: c.arg, text: c.values.map(shown).join(', '), absent: c.absent }));
}

// toConditions turns rows back into stored conditions, typing each value by the argument's schema type; a quoted value stays a string.
export function toConditions(rows: RuleRow[], args: McpToolArg[]): FreeCondition[] {
  return rows.map((r) => {
    const type = args.find(a => a.name === r.arg)?.type;
    return { arg: r.arg, values: tokens(r.text).map(v => jsonOf(v, type)), absent: r.absent };
  });
}

// checkRow returns the error that stops a row from being saved and the advice that doesn't; an unknown argument that skips asking when missing would never ask, so it is an error.
export function checkRow(row: RuleRow, args: McpToolArg[]): { error: string, warning: string } {
  if (!row.arg.trim()) {
    return { error: 'approvalrule.arg_required', warning: '' };
  }
  if (!argName.test(row.arg)) {
    return { error: 'approvalrule.arg_invalid', warning: '' };
  }
  const [condition] = toConditions([row], args);
  const values = condition!.values;
  if (!values.length && !row.absent) {
    return { error: 'approvalrule.values_required', warning: '' };
  }
  if (values.length > maxValues) {
    return { error: 'approvalrule.limit_values', warning: '' };
  }
  if (values.some(v => new TextEncoder().encode(v).length > maxValueBytes)) {
    return { error: 'approvalrule.value_too_long', warning: '' };
  }
  const known = args.find(a => a.name === row.arg);
  if (args.length && !known) {
    return row.absent ? { error: 'approvalrule.unknown_arg_missing_error', warning: '' } : { error: '', warning: 'approvalrule.unknown_arg_warning' };
  }
  if (known?.type === 'other') {
    return { error: '', warning: 'approvalrule.non_scalar' };
  }
  const typed = (v: string) => {
    if (known?.type === 'boolean') {
      return v === 'true' || v === 'false';
    }
    if (known?.type === 'number' || known?.type === 'integer') {
      return /^-?\d+(?:\.\d+)?(?:e[+-]?\d+)?$/i.test(v) && (known.type === 'number' || Number.isInteger(Number(v)));
    }
    return true;
  };
  return { error: '', warning: values.every(typed) ? '' : 'approvalrule.type_mismatch' };
}

// summaryOf is the one-sentence summary under the editor, or '' while no row names an argument.
export function summaryOf(rows: RuleRow[], t: Translate): string {
  const done = rows.filter(r => r.arg.trim());
  if (!done.length) {
    return '';
  }
  const parts = done.map(r => t(r.absent ? 'approvalrule.summary_row_missing' : 'approvalrule.summary_row', { arg: r.arg, values: tokens(r.text).join(` ${t('approvalrule.or')} `) }));
  return t('approvalrule.summary', { rules: parts.join(` ${t('approvalrule.and')} `) });
}
