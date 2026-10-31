import { describe, expect, it } from 'vitest';

import { expiryState, keyStatus, relativeTime } from './keyStatus';

const now = new Date('2026-10-01T00:00:00Z');
const day = 86_400_000;

describe('expiryState', () => {
  it('has nothing to say without an expiry', () => {
    expect(expiryState(undefined, now)).toEqual({ kind: 'none' });
  });

  it('is expired at or after the expiry time', () => {
    expect(expiryState(now, now)).toEqual({ kind: 'expired' });
    expect(expiryState(new Date(now.getTime() - 1), now)).toEqual({ kind: 'expired' });
  });

  it('warns within seven days, counting a partial day as a day', () => {
    expect(expiryState(new Date(now.getTime() + 1), now)).toEqual({ kind: 'soon', days: 1 });
    expect(expiryState(new Date(now.getTime() + 5 * day), now)).toEqual({ kind: 'soon', days: 5 });
    expect(expiryState(new Date(now.getTime() + 7 * day), now)).toEqual({ kind: 'soon', days: 7 });
  });

  it('is fine beyond seven days', () => {
    expect(expiryState(new Date(now.getTime() + 7 * day + 1), now)).toEqual({ kind: 'ok' });
  });
});

describe('relativeTime', () => {
  it('is empty when the key was never used', () => {
    expect(relativeTime(undefined, now, 'en-US')).toBe('');
  });

  it('picks the largest unit that fits', () => {
    expect(relativeTime(new Date(now.getTime() - 30_000), now, 'en-US')).toBe('now');
    expect(relativeTime(new Date(now.getTime() - 5 * 60_000), now, 'en-US')).toBe('5 minutes ago');
    expect(relativeTime(new Date(now.getTime() - 2 * 3_600_000), now, 'en-US')).toBe('2 hours ago');
    expect(relativeTime(new Date(now.getTime() - 5 * day), now, 'en-US')).toBe('5 days ago');
    expect(relativeTime(new Date(now.getTime() - 60 * day), now, 'en-US')).toBe('2 months ago');
    expect(relativeTime(new Date(now.getTime() - 800 * day), now, 'en-US')).toBe('2 years ago');
  });

  it('speaks the given locale', () => {
    expect(relativeTime(new Date(now.getTime() - 5 * day), now, 'id-ID')).toBe('5 hari yang lalu');
  });
});

describe('keyStatus', () => {
  it('is revoked whenever the key was revoked, expired or not', () => {
    expect(keyStatus(true, { kind: 'ok' })).toBe('revoked');
    expect(keyStatus(true, { kind: 'expired' })).toBe('revoked');
  });

  it('is expired once a live key passes its expiry, so it offers no revoke', () => {
    expect(keyStatus(false, { kind: 'expired' })).toBe('expired');
  });

  it('is active while a live key has time left or no expiry', () => {
    expect(keyStatus(false, { kind: 'ok' })).toBe('active');
    expect(keyStatus(false, { kind: 'soon', days: 3 })).toBe('active');
    expect(keyStatus(false, { kind: 'none' })).toBe('active');
  });
});
