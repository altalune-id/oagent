import { describe, expect, it } from 'vitest';

import { connectError, setKnownKeys } from '../../test/nuxt-globals';
import { useErrorMessage } from './useErrorMessage';

describe('useErrorMessage', () => {
  it('turns a Connect error into a translated message with a reference', () => {
    setKnownKeys('apperror.ORG003');
    const shown = useErrorMessage()(connectError('ORG003', { slug: 'a' }, 'req-9'), 'error.load_orgs');
    expect(shown).toEqual({ message: 'apperror.ORG003|{"slug":"a"}', reference: 'error.ref|{"id":"req-9"}' });
  });

  it('passes scope labels to the message', () => {
    setKnownKeys('apperror.GEN007');
    const shown = useErrorMessage()(connectError('GEN007', { permission: 'agent:read' }, ''), 'error.load_orgs');
    expect(shown).toEqual({ message: 'apperror.GEN007|{"permission":"scope.agent_read"}' });
  });

  it('passes field labels to the message', () => {
    setKnownKeys('apperror.GEN004', 'field.organization_id');
    const shown = useErrorMessage()(connectError('GEN004', { field: 'organization_id' }, ''), 'error.load_orgs');
    expect(shown).toEqual({ message: 'apperror.GEN004|{"field":"field.organization_id"}' });
  });

  it('translates the fallback key for a plain error', () => {
    expect(useErrorMessage()(new Error('boom'), 'error.load_orgs')).toEqual({ message: 'error.load_orgs' });
  });
});
