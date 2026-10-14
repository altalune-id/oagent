import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';

import { ErrorDetailSchema } from '../../../gen/apperror/v1/error_pb';
import { describeError, toShown } from './message';

const messages: Record<string, string> = {
  'apperror.ORG003': 'Slug {slug} must be {min} to {max} characters',
  'apperror.WHK002': 'Invalid webhook URL',
  'apperror.WHK002_https_required': 'Webhook URL must use https',
  'apperror.GEN004': '{field} is invalid',
  'apperror.GEN007': 'Missing permission: {permission}',
  'apperror.GEN900': 'Something went wrong',
  'field.organization_id': 'Organization ID',
  'apperror.WHK003_unknown': 'Unknown event {event}',
  'webhooks.event_agent_created': 'Agent created',
};
const seen: Record<string, unknown>[] = [];
function t(key: string, params?: Record<string, unknown>): string {
  seen.push(params ?? {});
  return (messages[key] ?? key).replace(/\{(\w+)\}/g, (_, name: string) => String(params?.[name] ?? `{${name}}`));
}
const te = (key: string) => key in messages;

function connectError(code: string, meta: Record<string, string> = {}, requestId = 'req-1'): ConnectError {
  return new ConnectError('server text', Code.InvalidArgument, undefined, [
    { desc: ErrorDetailSchema, value: create(ErrorDetailSchema, { code, meta, requestId }) },
  ]);
}

describe('describeError', () => {
  it('uses the reason key first', () => {
    const view = describeError(connectError('WHK002', { reason: 'https_required', max: '2048' }), t, te, 'fallback');
    expect(view.message).toBe('Webhook URL must use https');
    expect(view.code).toBe('WHK002');
  });

  it('falls back to the code key, with meta as params', () => {
    const view = describeError(connectError('ORG003', { reason: 'length', slug: 'a', min: '3', max: '64' }), t, te, 'fallback');
    expect(view.message).toBe('Slug a must be 3 to 64 characters');
  });

  it('shows the server message for a code it does not know, never a raw key', () => {
    const view = describeError(connectError('XYZ999', { reason: 'new' }), t, te, 'fallback');
    expect(view.message).toBe('server text');
    expect(view.message).not.toContain('apperror.');
    expect(view.code).toBe('XYZ999');
  });

  it('uses the fallback for a non-Connect error', () => {
    expect(describeError(new Error('boom'), t, te, 'fallback')).toEqual({ message: 'fallback' });
    expect(describeError(undefined, t, te, 'fallback')).toEqual({ message: 'fallback' });
  });

  it('uses the fallback for a Connect error without a detail', () => {
    const view = describeError(new ConnectError('Failed to fetch', Code.Unavailable), t, te, 'fallback');
    expect(view.message).toBe('fallback');
  });

  it('shows scope and field labels instead of raw names', () => {
    const scope = (name: string) => (name === 'agent:read' ? 'Read agents' : name);
    expect(describeError(connectError('GEN007', { permission: 'agent:read' }), t, te, 'fallback', { scope }).message)
      .toBe('Missing permission: Read agents');
    expect(describeError(connectError('GEN004', { field: 'organization_id', reason: 'invalid' }), t, te, 'fallback').message)
      .toBe('Organization ID is invalid');
    expect(describeError(connectError('GEN004', { field: 'mystery_id', reason: 'invalid' }), t, te, 'fallback').message)
      .toBe('mystery_id is invalid');
  });

  it('shows a webhook event label, or the raw name when it has none', () => {
    expect(describeError(connectError('WHK003', { reason: 'unknown', event: 'agent.created' }), t, te, 'fallback').message)
      .toBe('Unknown event Agent created');
    expect(describeError(connectError('WHK003', { reason: 'unknown', event: 'agent.gone' }), t, te, 'fallback').message)
      .toBe('Unknown event agent.gone');
  });

  it('never passes diagnostic meta to a message', () => {
    seen.length = 0;
    const view = describeError(connectError('GEN900', { trace_id: 't', underlying_error: 'pq: secret', request_id: 'r' }), t, te, 'fallback');
    expect(view.message).toBe('Something went wrong');
    for (const params of seen) {
      expect(Object.keys(params)).not.toEqual(expect.arrayContaining(['trace_id']));
      expect(Object.keys(params)).not.toEqual(expect.arrayContaining(['underlying_error']));
    }
    expect(JSON.stringify(view)).not.toContain('pq: secret');
  });

  it('returns the request id', () => {
    expect(describeError(connectError('GEN900', {}, 'req-42'), t, te, 'fallback').requestId).toBe('req-42');
    expect(describeError(connectError('GEN900', {}, ''), t, te, 'fallback').requestId).toBeUndefined();
  });
});

describe('toShown', () => {
  it('adds the reference line only when there is a request id', () => {
    const ref = (id: string) => `Ref: ${id}`;
    expect(toShown({ message: 'Nope', code: 'X', requestId: 'req-1' }, ref)).toEqual({ message: 'Nope', reference: 'Ref: req-1' });
    expect(toShown({ message: 'Nope' }, ref)).toEqual({ message: 'Nope' });
  });
});
