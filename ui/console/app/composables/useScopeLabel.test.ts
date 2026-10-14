import { describe, expect, it } from 'vitest';

import { useScopeLabel } from './useScopeLabel';

describe('useScopeLabel', () => {
  it('labels a known scope', () => {
    expect(useScopeLabel()('agent:read')).toBe('scope.agent_read');
  });

  it('shows the raw name of an unknown scope', () => {
    expect(useScopeLabel()('mystery:do')).toBe('mystery:do');
  });
});
