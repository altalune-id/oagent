import { describe, expect, it } from 'vitest';
import { grantable, hintBadges, nameParts, serverNameError, toolName } from './tools';

const tool = (name: string) => ({ name, description: '', hint: 'read_only' as const, args: [] });

describe('mcp tools', () => {
  it('prefixes the tool with its server', () => {
    expect(toolName('yasaku', 'record_expense')).toBe('yasaku__record_expense');
  });

  it('splits a name after each run of underscores so it wraps only there', () => {
    expect(nameParts('yasaku__record_expense')).toEqual(['yasaku__', 'record_', 'expense']);
    expect(nameParts('plain')).toEqual(['plain']);
    expect(nameParts('')).toEqual(['']);
  });

  it('grants only names the model accepts: 64 characters, letters, digits, _ and -', () => {
    expect(grantable('yasaku', tool('record_expense'))).toBe(true);
    expect(grantable('yasaku', tool('x'.repeat(56)))).toBe(true);
    expect(grantable('yasaku', tool('x'.repeat(57)))).toBe(false);
    expect(grantable('yasaku', tool('bad.name'))).toBe(false);
  });

  it('labels every hint with text and an icon', () => {
    expect(Object.values(hintBadges).every(b => b.labelKey && b.icon)).toBe(true);
  });

  it('refuses empty names, __ and other characters', () => {
    expect(serverNameError('')).toBe('mcpservers.name_required');
    expect(serverNameError('ya__saku')).toBe('mcpservers.name_double_underscore');
    expect(serverNameError('ya saku')).toBe('mcpservers.name_chars');
    expect(serverNameError('x'.repeat(33))).toBe('mcpservers.name_chars');
    expect(serverNameError('yasaku-2')).toBe('');
  });
});
