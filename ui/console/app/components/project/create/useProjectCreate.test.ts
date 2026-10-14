import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useProjectsStore } from '~/stores/projects';
import { connectError, deferred, setClient, setKnownKeys } from '../../../../test/nuxt-globals';
import { useProjectCreate } from './useProjectCreate';

const project = { id: 'x6MH9WPWjktMwCBKHy4pF', orgId: 'V3StGXR8Z5jdHj6BmyT2a', slug: 'web', name: 'Web' };

describe('useProjectCreate', () => {
  let createProject: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    createProject = vi.fn().mockResolvedValue({ project });
    setClient('$projectClient', { createProject });
  });

  it('creates a project, adds it and closes', async () => {
    const form = useProjectCreate('V3StGXR8Z5jdHj6BmyT2a');
    form.show();
    expect(form.open.value).toBe(true);
    form.name.value = ' Web ';
    form.slug.value = ' web ';
    await form.submit();
    expect(createProject).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a', name: 'Web', slug: 'web' });
    expect(useProjectsStore().items).toEqual([project]);
    expect(form.open.value).toBe(false);
    expect([form.name.value, form.slug.value]).toEqual(['', '']);
  });

  it('stays open when the response has no project', async () => {
    createProject.mockResolvedValue({});
    const form = useProjectCreate('V3StGXR8Z5jdHj6BmyT2a');
    form.show();
    form.name.value = 'Web';
    await form.submit();
    expect(form.open.value).toBe(true);
    expect(useProjectsStore().items).toEqual([]);
  });

  it('does not submit a blank name or while busy', async () => {
    const pending = deferred<{ project: typeof project }>();
    createProject.mockReturnValue(pending.promise);
    const form = useProjectCreate('V3StGXR8Z5jdHj6BmyT2a');
    form.show();
    form.name.value = '  ';
    await form.submit();
    expect(createProject).not.toHaveBeenCalled();
    form.name.value = 'Web';
    const first = form.submit();
    await form.submit();
    form.hide();
    expect(form.open.value).toBe(true);
    pending.resolve({ project });
    await first;
    expect(createProject).toHaveBeenCalledTimes(1);
  });

  it('shows a Connect error with its reference, then the fallback for a plain error', async () => {
    setKnownKeys('apperror.GEN007');
    createProject.mockRejectedValueOnce(connectError('GEN007')).mockRejectedValueOnce(new Error('boom'));
    const form = useProjectCreate('V3StGXR8Z5jdHj6BmyT2a');
    form.show();
    form.name.value = 'Web';
    await form.submit();
    expect(form.error.value).toEqual({ message: 'apperror.GEN007', reference: 'error.ref|{"id":"req-1"}' });
    await form.submit();
    expect(form.error.value).toEqual({ message: 'error.create_project' });
    form.hide();
    expect(form.open.value).toBe(false);
    expect(form.error.value).toBeNull();
  });

  it('shows a taken slug under the slug input and never sends a request its rules refuse', async () => {
    setKnownKeys('apperror.PRJ002');
    createProject.mockRejectedValueOnce(connectError('PRJ002'));
    const form = useProjectCreate('V3StGXR8Z5jdHj6BmyT2a');
    form.show();
    form.name.value = 'Web';
    await form.submit();
    expect(form.fieldErrors.value).toEqual({ slug: 'apperror.PRJ002' });
    expect(form.error.value).toBeNull();
    form.name.value = ' ';
    await form.submit();
    expect(createProject).toHaveBeenCalledTimes(1);
    expect(form.fieldErrors.value).toEqual({ name: 'validation.required' });
  });
});
