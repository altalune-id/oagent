import { describe, expect, it } from 'vitest';
import { t } from './copy';

describe('copy', () => {
  it('reads the shared base copy and fills params', () => {
    expect(t('mcp.title_agent')).toBe('Agent');
    expect(t('common.copy_label', { label: 'ID' })).toBe('Copy ID');
    expect(t('common.copy_label')).toBe('Copy {label}');
    expect(t('no.such.key')).toBe('no.such.key');
  });
});
