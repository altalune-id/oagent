import type { DescField, DescMessage, MessageShape } from '@bufbuild/protobuf';
import type { Violation } from '@bufbuild/protovalidate';
import type { FieldRules } from '../../../gen/buf/validate/validate_pb';
import { createRegistry, getOption } from '@bufbuild/protobuf';
import { pathToString } from '@bufbuild/protobuf/reflect';
import { createValidator } from '@bufbuild/protovalidate';

import { ConnectError } from '@connectrpc/connect';
import { ErrorDetailSchema } from '../../../gen/apperror/v1/error_pb';
import { field as fieldRules } from '../../../gen/buf/validate/validate_pb';
import { file_oagent_v1_rules } from '../../../gen/oagent/v1/rules_pb';

// FieldIssue is why one request field was refused, as the server's violations say it: never the value.
export interface FieldIssue {
  reason: string;
  param?: string;
}

// FieldIssues holds the first issue of each refused field, keyed by its proto path (name, scopes[0]).
export type FieldIssues = Record<string, FieldIssue>;

const validator = createValidator({ registry: createRegistry(file_oagent_v1_rules) });

const formatRules = new Set([
  'len',
  'len_bytes',
  'pattern',
  'prefix',
  'suffix',
  'contains',
  'not_contains',
  'email',
  'hostname',
  'ip',
  'ipv4',
  'ipv6',
  'uri',
  'uri_ref',
  'address',
  'uuid',
  'tuuid',
  'ulid',
  'host_and_port',
  'well_known_regex',
  'format',
  'public_id',
]);

// reasonOf maps a protovalidate rule id onto the server's reason; a trailing number (oagent.name.max_len.200) is the param.
export function reasonOf(ruleId: string): FieldIssue {
  const parts = ruleId.split('.');
  const last = parts.at(-1) ?? '';
  const param = /^\d+$/.test(last) ? parts.pop() : undefined;
  const rule = parts.at(-1) ?? '';
  const issue = (reason: string): FieldIssue => (param ? { reason, param } : { reason });
  if (rule === 'required' || rule === 'blank' || rule === 'min_items' || rule.endsWith('_empty')) {
    return issue('required');
  }
  if (rule === 'min_len' || rule === 'min_bytes') {
    return issue('too_short');
  }
  if (rule === 'max_len' || rule === 'max_bytes') {
    return issue('too_long');
  }
  if (rule === 'in' || rule === 'not_in' || rule === 'const' || rule === 'defined_only') {
    return issue('not_allowed');
  }
  if (rule === 'max_items' || rule === 'max_pairs') {
    return issue('too_many');
  }
  return issue(formatRules.has(rule) ? 'invalid_format' : 'invalid');
}

// checkRequest runs msg through the same rules the server checks and returns its refused fields.
export function checkRequest<Desc extends DescMessage>(schema: Desc, msg: MessageShape<Desc>): FieldIssues {
  const result = validator.validate(schema, msg);
  if (result.kind !== 'invalid') {
    return {};
  }
  const issues: FieldIssues = {};
  for (const v of result.violations) {
    const path = pathToString(v.field);
    if (!issues[path]) {
      issues[path] = withLimit(reasonOf(v.ruleId), v);
    }
  }
  return issues;
}

function withLimit(issue: FieldIssue, v: Violation): FieldIssue {
  if (issue.param || !['too_short', 'too_long', 'too_many'].includes(issue.reason)) {
    return issue;
  }
  const desc = [...v.field].reverse().find((p): p is DescField => p.kind === 'field');
  const limit = desc ? limitOf(getOption(desc, fieldRules), v.ruleId) : undefined;
  return limit === undefined ? issue : { ...issue, param: limit };
}

function limitOf(rules: FieldRules, ruleId: string): string | undefined {
  const type = rules.type;
  if (!type.value) {
    return undefined;
  }
  const key = ({ min_len: 'minLen', max_len: 'maxLen', min_bytes: 'minBytes', max_bytes: 'maxBytes', max_items: 'maxItems', max_pairs: 'maxPairs' } as Record<string, string>)[ruleId.split('.').at(-1) ?? ''];
  const value = key ? (type.value as unknown as Record<string, unknown>)[key] : undefined;
  return typeof value === 'bigint' || typeof value === 'number' ? String(value) : undefined;
}

// serverIssues reads the per-field violations of a refused request, or none.
export function serverIssues(err: unknown): FieldIssues {
  if (!(err instanceof ConnectError)) {
    return {};
  }
  const issues: FieldIssues = {};
  for (const v of err.findDetails(ErrorDetailSchema)[0]?.violations ?? []) {
    if (!issues[v.field]) {
      issues[v.field] = v.param ? { reason: v.reason, param: v.param } : { reason: v.reason };
    }
  }
  return issues;
}
