import { describe, expect, it } from 'vitest';

import { modelSummary, timeoutText } from './providers';

describe('provider helpers', () => {
  it('shows micro-USD prices as USD and leaves odd values as they are', () => {
    expect(modelSummary([{ id: 'a', prices: { inputMicros: '0', outputMicros: '2500001' } }])).toBe('a (in $0 · out $2.500001 per 1M tokens)');
    expect(modelSummary([{ id: 'a', prices: { inputMicros: 'x' } }])).toBe('a (in x · out $0 per 1M tokens)');
    expect(modelSummary([{ prices: undefined }])).toBe(' (no price)');
    expect(modelSummary([])).toBe('—');
    expect(modelSummary(undefined)).toBe('—');
  });

  it('shows a timeout in seconds', () => {
    expect(timeoutText(1500)).toBe('1.5 s');
    expect(timeoutText(undefined)).toBe('0 s');
  });
});
