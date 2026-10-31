import { describe, expect, it } from 'vitest';

import { mcpSnippet } from './mcpSnippet';

describe('mcpSnippet', () => {
  it('is a valid .mcp.json entry with the bearer key and the URL', () => {
    const snippet = mcpSnippet('oagent', 'https://oagent.example.com/app/mcp', 'oagent_secret');
    const parsed = JSON.parse(snippet);
    expect(parsed).toEqual({
      mcpServers: {
        oagent: {
          type: 'http',
          url: 'https://oagent.example.com/app/mcp',
          headers: { Authorization: 'Bearer oagent_secret' },
        },
      },
    });
    expect(snippet).toContain('\n  ');
  });
});
