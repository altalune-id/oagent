import { describe, expect, it } from 'vitest';

import { loginURL, mcpURL, siblingURL, staticURL } from './urls';

describe('siblingURL', () => {
  it.each([
    ['', '/login'],
    ['/api', '/login'],
    ['/api/', '/login'],
    ['/app/api', '/app/login'],
    ['/app/api/', '/app/login'],
    ['https://oagent.example.com/app/api', 'https://oagent.example.com/app/login'],
  ])('%s → %s', (apiUrl, want) => {
    expect(siblingURL(apiUrl, '/login')).toBe(want);
  });

  it('finds logout too', () => {
    expect(siblingURL('/app/api', '/logout')).toBe('/app/logout');
  });
});

describe('loginURL', () => {
  it('keeps path, query and hash, encoded', () => {
    const url = loginURL('/app/api', { pathname: '/app/console/o/acme/agents', search: '?tab=keys&x=1', hash: '#top' });
    expect(url).toBe('/app/login?return_to=%2Fapp%2Fconsole%2Fo%2Facme%2Fagents%3Ftab%3Dkeys%26x%3D1%23top');
    expect(decodeURIComponent(url.split('return_to=')[1]!)).toBe('/app/console/o/acme/agents?tab=keys&x=1#top');
  });

  it('works at the root', () => {
    expect(loginURL('', { pathname: '/console/', search: '', hash: '' })).toBe('/login?return_to=%2Fconsole%2F');
  });
});

describe('mcpURL', () => {
  it('is absolute and next to the API', () => {
    expect(mcpURL('/app/api', 'https://oagent.example.com')).toBe('https://oagent.example.com/app/mcp');
    expect(mcpURL('', 'http://localhost:5150')).toBe('http://localhost:5150/mcp');
  });
});

describe('staticURL', () => {
  it.each([
    ['', '/static/favicon.ico'],
    ['/app/api', '/app/static/favicon.ico'],
    ['https://oagent.example.com/app/api/', 'https://oagent.example.com/app/static/favicon.ico'],
  ])('%s → %s', (apiUrl, want) => {
    expect(staticURL(apiUrl, 'favicon.ico')).toBe(want);
  });
});
