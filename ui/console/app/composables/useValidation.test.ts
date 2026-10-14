import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it } from 'vitest';

import { ErrorDetailSchema, ViolationSchema } from '../../gen/apperror/v1/error_pb';
import { CreateOrgRequestSchema, UpdateOrgRequestSchema } from '../../gen/org/v1/org_pb';
import { connectError, setKnownKeys } from '../../test/nuxt-globals';
import { useValidation } from './useValidation';

function violations(...vs: { field: string, reason: string, param?: string }[]): ConnectError {
  return new ConnectError('bad', Code.InvalidArgument, undefined, [{
    desc: ErrorDetailSchema,
    value: create(ErrorDetailSchema, { code: 'GEN004', meta: { field: vs[0]!.field, reason: vs[0]!.reason }, violations: vs.map(v => create(ViolationSchema, v)) }),
  }]);
}

describe('useValidation', () => {
  beforeEach(() => setKnownKeys());

  it('returns no errors for a valid request', () => {
    const v = useValidation(CreateOrgRequestSchema, { fields: ['name', 'slug'] });
    expect(v.check(create(CreateOrgRequestSchema, { name: 'Acme' }))).toBe(true);
    expect(v.errors.value).toEqual({});
    expect(v.formError.value).toBeNull();
  });

  it('returns a message per refused field, with the limit', () => {
    const v = useValidation(CreateOrgRequestSchema, { fields: ['name', 'slug'] });
    expect(v.check(create(CreateOrgRequestSchema, { name: 'a'.repeat(201) }))).toBe(false);
    expect(v.errors.value).toEqual({ name: 'validation.too_long|{"max":"200"}' });
    expect(v.check(create(CreateOrgRequestSchema, {}))).toBe(false);
    expect(v.errors.value).toEqual({ name: 'validation.required' });
  });

  it('words a slug format refusal like the domain hint', () => {
    const v = useValidation(CreateOrgRequestSchema, { fields: ['name', 'slug'] });
    expect(v.check(create(CreateOrgRequestSchema, { name: 'Acme', slug: 'Acme Inc' }))).toBe(false);
    expect(v.errors.value).toEqual({ slug: 'validation.slug_format' });
    expect(v.check(create(CreateOrgRequestSchema, { name: 'Acme', slug: 'ab' }))).toBe(false);
    expect(v.errors.value).toEqual({ slug: 'validation.too_short|{"min":"3"}' });
  });

  it('reports a refused field the form does not show as a form error', () => {
    setKnownKeys('field.id');
    const v = useValidation(UpdateOrgRequestSchema, { fields: ['name', 'slug'] });
    expect(v.check(create(UpdateOrgRequestSchema, { id: 'x', name: 'Acme', slug: 'acme' }))).toBe(false);
    expect(v.errors.value).toEqual({});
    expect(v.formError.value).toEqual({ message: 'validation.other|{"field":"field.id","message":"validation.invalid_format"}' });
  });

  it('places server violations under the same inputs', () => {
    const v = useValidation(CreateOrgRequestSchema, { fields: ['name', 'slug'] });
    expect(v.fromServer(violations({ field: 'name', reason: 'too_short', param: '3' }, { field: 'slug', reason: 'invalid_format' }), 'error.create_org')).toBeNull();
    expect(v.errors.value).toEqual({ name: 'validation.too_short|{"min":"3"}', slug: 'validation.slug_format' });
  });

  it('places a domain error under the input its code names', () => {
    setKnownKeys('apperror.ORG002');
    const v = useValidation(CreateOrgRequestSchema, { fields: ['name', 'slug'], codes: { ORG002: 'slug' } });
    expect(v.fromServer(connectError('ORG002', { slug: 'acme' }), 'error.create_org')).toBeNull();
    expect(v.errors.value).toEqual({ slug: 'apperror.ORG002|{"slug":"acme"}' });
  });

  it('leaves any other error to the form', () => {
    setKnownKeys('apperror.ORG005');
    const v = useValidation(CreateOrgRequestSchema, { fields: ['name'] });
    expect(v.fromServer(connectError('ORG005'), 'error.create_org')).toEqual({ message: 'apperror.ORG005', reference: 'error.ref|{"id":"req-1"}' });
    expect(v.fromServer(violations({ field: 'org_id', reason: 'required' }), 'error.create_org')?.message).toBe('validation.other|{"field":"org_id","message":"validation.required"}');
    expect(v.errors.value).toEqual({});
  });

  it('clears', () => {
    const v = useValidation(CreateOrgRequestSchema, { fields: ['name'] });
    v.check(create(CreateOrgRequestSchema, {}));
    v.clear();
    expect(v.errors.value).toEqual({});
  });
});
