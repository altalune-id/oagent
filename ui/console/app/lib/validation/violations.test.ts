import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';

import ruleReasons from '../../../../../internal/controlplane/testdata/rule_reasons.json';
import { CreateAPIKeyRequestSchema } from '../../../gen/apikey/v1/apikey_pb';
import { ErrorDetailSchema, ViolationSchema } from '../../../gen/apperror/v1/error_pb';
import { ChangePasswordRequestSchema } from '../../../gen/auth/v1/auth_pb';
import { CreateOrgRequestSchema, UpdateOrgRequestSchema } from '../../../gen/org/v1/org_pb';
import { CreateWebhookRequestSchema } from '../../../gen/webhook/v1/webhook_pb';
import { checkRequest, reasonOf, serverIssues } from './violations';

const goodID = 'abcdefghjkmnpqrstuvwx';

// The Go mapping is tested against the same table (internal/controlplane TestReasonOf).
describe('reasonOf', () => {
  it.each(ruleReasons as { rule: string, reason: string, param?: string }[])('$rule is $reason', ({ rule, reason, param }) => {
    expect(reasonOf(rule)).toEqual(param ? { reason, param } : { reason });
  });
});

describe('checkRequest', () => {
  it('passes a valid request', () => {
    expect(checkRequest(CreateOrgRequestSchema, create(CreateOrgRequestSchema, { name: 'Acme' }))).toEqual({});
  });

  it('names each refused field with its reason and limit', () => {
    expect(checkRequest(UpdateOrgRequestSchema, create(UpdateOrgRequestSchema, { id: 'x', name: 'a'.repeat(201), slug: 'acme' }))).toEqual({
      id: { reason: 'invalid_format' },
      name: { reason: 'too_long', param: '200' },
    });
    expect(checkRequest(CreateOrgRequestSchema, create(CreateOrgRequestSchema, { name: '  ' }))).toEqual({ name: { reason: 'required' } });
  });

  it('takes a shared rule limit from its id and paths items like the server', () => {
    const msg = create(CreateAPIKeyRequestSchema, { orgId: goodID, kind: 'org', name: 'k'.repeat(101), scopes: ['agent:*'] });
    expect(checkRequest(CreateAPIKeyRequestSchema, msg)).toEqual({
      'name': { reason: 'too_long', param: '100' },
      'scopes[0]': { reason: 'invalid_format' },
      'expires_at': { reason: 'required' },
    });
  });

  it('reads a standard rule limit from the field options', () => {
    const msg = create(ChangePasswordRequestSchema, { currentPassword: 'x', newPassword: 'p'.repeat(257) });
    expect(checkRequest(ChangePasswordRequestSchema, msg)).toEqual({ new_password: { reason: 'too_long', param: '256' } });
  });

  it('checks slugs with the domain shape and leaves an empty one to be minted', () => {
    expect(checkRequest(CreateOrgRequestSchema, create(CreateOrgRequestSchema, { name: 'Acme' }))).toEqual({});
    expect(checkRequest(CreateOrgRequestSchema, create(CreateOrgRequestSchema, { name: 'Acme', slug: 'Acme Inc' }))).toEqual({ slug: { reason: 'invalid_format' } });
    expect(checkRequest(CreateOrgRequestSchema, create(CreateOrgRequestSchema, { name: 'Acme', slug: 'ab' }))).toEqual({ slug: { reason: 'too_short', param: '3' } });
    expect(checkRequest(UpdateOrgRequestSchema, create(UpdateOrgRequestSchema, { id: goodID, name: 'Acme' }))).toEqual({ slug: { reason: 'required' } });
  });

  it('reads the password minimum from the field options', () => {
    const msg = create(ChangePasswordRequestSchema, { currentPassword: 'x', newPassword: 'short' });
    expect(checkRequest(ChangePasswordRequestSchema, msg)).toEqual({ new_password: { reason: 'too_short', param: '12' } });
  });

  it('leaves the limit out when the rule has none to read', () => {
    const msg = create(CreateWebhookRequestSchema, { orgId: goodID, projectId: goodID, url: 'https://e.com' });
    expect(checkRequest(CreateWebhookRequestSchema, msg)).toEqual({ event_types: { reason: 'required' } });
  });

  it('accepts what the domain accepts', () => {
    const msg = create(CreateWebhookRequestSchema, { orgId: goodID, projectId: goodID, url: ' HTTPS://example.com/a b ', eventTypes: ['agent.created'] });
    expect(checkRequest(CreateWebhookRequestSchema, msg)).toEqual({});
  });
});

describe('serverIssues', () => {
  it('reads the violations of a GEN004', () => {
    const err = new ConnectError('bad', Code.InvalidArgument, undefined, [{
      desc: ErrorDetailSchema,
      value: create(ErrorDetailSchema, {
        code: 'GEN004',
        violations: [
          create(ViolationSchema, { field: 'name', reason: 'too_long', param: '200' }),
          create(ViolationSchema, { field: 'name', reason: 'required' }),
          create(ViolationSchema, { field: 'slug', reason: 'invalid_format' }),
        ],
      }),
    }]);
    expect(serverIssues(err)).toEqual({ name: { reason: 'too_long', param: '200' }, slug: { reason: 'invalid_format' } });
  });

  it('has none for other errors', () => {
    expect(serverIssues(new Error('boom'))).toEqual({});
    expect(serverIssues(new ConnectError('x', Code.NotFound))).toEqual({});
  });
});
