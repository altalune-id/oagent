import { describe, expect, it } from 'vitest';
import { headerNameError, hintBadges, nameParts, toolName } from './tools';

describe('mcp tools', () => {
  it('prefixes the tool with its server', () => {
    expect(toolName('yasaku', 'record_expense')).toBe('yasaku__record_expense');
  });

  it('splits a name after each run of underscores so it wraps only there', () => {
    expect(nameParts('yasaku__record_expense')).toEqual(['yasaku__', 'record_', 'expense']);
    expect(nameParts('plain')).toEqual(['plain']);
    expect(nameParts('')).toEqual(['']);
  });

  it('refuses a header name that is not an HTTP token or that oagent sets itself', () => {
    expect(headerNameError('X-Api-Key')).toBe('');
    expect(headerNameError('X_Key!#$%&\'*+.^`|~')).toBe('');
    expect(headerNameError('')).toBe('mcpservers.header_required');
    expect(headerNameError('bad header')).toBe('mcpservers.header_required');
    expect(headerNameError('Mcp-Session-Id')).toBe('mcpservers.header_reserved');
    expect(headerNameError('HOST')).toBe('mcpservers.header_reserved');
  });

  it('labels every hint with text and an icon', () => {
    expect(Object.values(hintBadges).every(b => b.labelKey && b.icon)).toBe(true);
  });
});
