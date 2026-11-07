import { describe, expect, it } from 'vitest';
import { isWebUrl } from './url';

describe('isWebUrl', () => {
  it('takes full http and https URLs and refuses other schemes and fragments of text', () => {
    expect(isWebUrl('https://openwa.example.id')).toBe(true);
    expect(isWebUrl('http://10.0.0.5:8080')).toBe(true);
    expect(['openwa', 'ftp://openwa.example.id', 'https://', ''].some(isWebUrl)).toBe(false);
  });
});
