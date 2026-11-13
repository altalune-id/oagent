import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { fakeKnowledgeClient, orgId, projectId } from '~~/test/fixtures/knowledge';
import { connectError, setKnownKeys } from '#test/nuxt-globals';
import { useKnowledgeStore } from '~/stores/knowledge';
import { useSourceCreate } from './useSourceCreate';

let client: ReturnType<typeof fakeKnowledgeClient>;

beforeEach(() => {
  setActivePinia(createPinia());
  client = fakeKnowledgeClient();
  setKnownKeys();
});

describe('useSourceCreate', () => {
  it('opens on Website with empty fields', () => {
    const form = useSourceCreate(orgId, projectId);
    form.seedUrl.value = 'x';
    form.kind.value = 'upload';
    form.show();
    expect([form.open.value, form.kind.value, form.name.value, form.seedUrl.value, form.error.value]).toEqual([true, 'url', '', '', null]);
  });

  it('refuses a non-https address before sending, and names a website from its host and path', async () => {
    const form = useSourceCreate(orgId, projectId);
    form.show();
    form.seedUrl.value = 'http://help.example.com/docs';
    expect(await form.submit()).toBeNull();
    expect(form.errors.value.seed_url).toBe('knowledge.url_invalid');
    expect(client.createURLSource).not.toHaveBeenCalled();
    form.seedUrl.value = ' https://help.example.com/docs/ ';
    const source = await form.submit();
    expect(client.createURLSource).toHaveBeenCalledWith({ orgId, projectId, name: 'help.example.com/docs', seedUrl: 'https://help.example.com/docs/' });
    expect(source?.kind).toBe('url');
    expect(form.open.value).toBe(false);
    expect(useKnowledgeStore().sources[0]!.id).toBe(source!.id);
  });

  it('needs a name for an upload source and places a taken name (KNW012) under it', async () => {
    const form = useSourceCreate(orgId, projectId);
    form.show();
    form.kind.value = 'upload';
    expect(await form.submit()).toBeNull();
    expect(form.errors.value.name).toBeTruthy();
    setKnownKeys('apperror.KNW012');
    client.createUploadSource.mockRejectedValueOnce(connectError('KNW012', { name: 'Manuals' }));
    form.name.value = ' Manuals ';
    expect(await form.submit()).toBeNull();
    expect(form.errors.value.name).toBe('apperror.KNW012|{"name":"Manuals"}');
    expect(form.open.value).toBe(true);
    const source = await form.submit();
    expect(client.createUploadSource).toHaveBeenLastCalledWith({ orgId, projectId, name: 'Manuals' });
    expect(source?.kind).toBe('upload');
  });

  it('places a server address refusal (KNW002) under the URL', async () => {
    setKnownKeys('apperror.KNW002_unparsable');
    client.createURLSource.mockRejectedValueOnce(connectError('KNW002', { reason: 'unparsable' }));
    const form = useSourceCreate(orgId, projectId);
    form.show();
    form.seedUrl.value = 'https://help.example.com';
    await form.submit();
    expect(form.errors.value.seed_url).toBe('apperror.KNW002_unparsable');
  });
});
