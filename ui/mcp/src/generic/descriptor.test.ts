import { describe, expect, it } from 'vitest';
import { badgeFor, formatValue, isEmpty, objectAt, pick, rowKey, rowsAt, valueAt } from './descriptor';

describe('descriptor helpers', () => {
  const data = { project: { id: 'p1', org: { slug: 'acme' } }, orgs: [{ id: 'o1' }, null, 'x', { id: 'o2' }] };

  it('reads paths, rows and objects from structuredContent', () => {
    expect(valueAt(data, 'project.org.slug')).toBe('acme');
    expect(valueAt(data, 'project.nope.slug')).toBeUndefined();
    expect(valueAt(data)).toBe(data);
    expect(rowsAt(data, 'orgs')).toEqual([{ id: 'o1' }, { id: 'o2' }]);
    expect(rowsAt(data, 'project')).toEqual([]);
    expect(objectAt(data, 'project')?.id).toBe('p1');
    expect(objectAt(data, 'orgs')).toBeNull();
    expect(objectAt({ id: 'x' })).toEqual({ id: 'x' });
  });

  it('treats what protojson leaves out as empty', () => {
    expect([undefined, null, '', false, []].every(isEmpty)).toBe(true);
    expect(isEmpty(0)).toBe(false);
    expect(isEmpty('x')).toBe(false);
  });

  it('formats dates, booleans and lists', () => {
    expect(formatValue({ key: 'a', label: 'A', kind: 'date' }, '2026-10-27T03:04:05Z')).toBe('2026-10-27 03:04 UTC');
    expect(formatValue({ key: 'a', label: 'A', kind: 'date' }, 'soon')).toBe('soon');
    expect(formatValue({ key: 'a', label: 'A', kind: 'bool' }, true)).toBe('Yes');
    expect(formatValue({ key: 'a', label: 'A', kind: 'bool' }, undefined)).toBe('No');
    expect(formatValue({ key: 'a', label: 'A', kind: 'list' }, ['read', 'write'])).toBe('read, write');
    expect(formatValue({ key: 'a', label: 'A', kind: 'list' }, 'one')).toBe('one');
    expect(formatValue({ key: 'a', label: 'A' }, '')).toBe('—');
    expect(formatValue({ key: 'a', label: 'A' }, 7)).toBe('7');
  });

  it('maps badges, falling back to the raw value', () => {
    const field = { key: 's', label: 'S', kind: 'badge' as const, badges: { VERIFIED: { label: 'Verified', tone: 'success' as const } } };
    expect(badgeFor(field, 'VERIFIED')).toEqual({ label: 'Verified', tone: 'success' });
    expect(badgeFor(field, 'ODD')).toEqual({ label: 'ODD', tone: 'ghost' });
    expect(badgeFor(field, undefined)).toEqual({ label: '—', tone: 'ghost' });
  });

  it('picks the set arguments for a refresh, in either spelling', () => {
    expect(pick({ orgId: 'o1', page: '', other: 1 }, ['orgId', 'page', 'missing'])).toEqual({ orgId: 'o1' });
    expect(pick({ project_id: 'p1' }, ['projectId'])).toEqual({ projectId: 'p1' });
  });

  it('keys a row by one field or several', () => {
    expect(rowKey({ a: 'x', b: { c: 'y' } }, ['a', 'b.c'])).toBe('x/y');
    expect(rowKey({ a: 'x' }, 'a')).toBe('x');
  });
});
