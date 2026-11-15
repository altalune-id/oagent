import { getGlobalDispatcher, setGlobalDispatcher } from 'undici';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { installGuard } from '../../egress/index.ts';
import { ScriptedModel } from '../../test/scripted-model.ts';
import { failed, webTurn } from '../../test/web.ts';
import { fetchTool } from './fetch.ts';

describe('web_fetch through the egress guard', () => {
  const previous = getGlobalDispatcher();

  afterEach(() => {
    setGlobalDispatcher(previous);
  });

  it('refuses a public name that resolves to a private address, with no fetch stub', async () => {
    const log = vi.fn();
    const resolve = vi.fn(async () => [{ address: '127.0.0.1', family: 4 }]);
    installGuard({ resolve, log });
    const turn = webTurn();
    const error = await failed(fetchTool(turn, { allowDomains: [] }, () => new ScriptedModel([])).invoke({ url: 'https://evil.example/', prompt: 'p' }));
    expect(error.message).toBe('web_fetch failed: blocked_host (host evil.example)');
    expect(resolve).toHaveBeenCalledWith('evil.example');
    expect(log).toHaveBeenCalledWith('ssrf refused', { host: 'evil.example' });
  });
});
