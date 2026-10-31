import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { connectError, deferred, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useProjectsStore } from './projects';

const web = { id: 'x6MH9WPWjktMwCBKHy4pF', orgId: 'V3StGXR8Z5jdHj6BmyT2a', slug: 'web', name: 'Web' };
const api = { id: 'KdrF3t3FHtppBPmMgj4sy', orgId: 'YeDxg2kMYznPJJG3wVmjj', slug: 'api', name: 'API' };

describe('useProjectsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads the projects of an org', async () => {
    const listProjects = vi.fn().mockResolvedValue({ projects: [web, api] });
    setClient('$projectClient', { listProjects });
    const projects = useProjectsStore();
    await projects.load('V3StGXR8Z5jdHj6BmyT2a');
    expect(listProjects).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a' });
    expect(projects.items).toEqual([web, api]);
    expect(projects.forOrg('V3StGXR8Z5jdHj6BmyT2a')).toEqual([web]);
    expect(projects.loading).toBe(false);
  });

  it('treats a missing list as empty', async () => {
    setClient('$projectClient', { listProjects: vi.fn().mockResolvedValue({}) });
    const projects = useProjectsStore();
    await projects.load('V3StGXR8Z5jdHj6BmyT2a');
    expect(projects.items).toEqual([]);
  });

  it('shows a Connect error with its reference', async () => {
    setKnownKeys('apperror.GEN900');
    setClient('$projectClient', { listProjects: vi.fn().mockRejectedValue(connectError('GEN900')) });
    const projects = useProjectsStore();
    await projects.load('V3StGXR8Z5jdHj6BmyT2a');
    expect(projects.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
  });

  it('shows the fallback for a plain error and loads again next time', async () => {
    const listProjects = vi.fn().mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce({ projects: [web] });
    setClient('$projectClient', { listProjects });
    const projects = useProjectsStore();
    await projects.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a');
    expect(projects.error).toEqual({ message: 'error.load_projects' });
    await projects.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a');
    expect(projects.error).toBeNull();
    expect(projects.items).toEqual([web]);
  });

  it('shares an in-flight load and skips an org already loaded or empty', async () => {
    const pending = deferred<{ projects: typeof web[] }>();
    const listProjects = vi.fn(() => pending.promise);
    setClient('$projectClient', { listProjects });
    const projects = useProjectsStore();
    await projects.ensureLoaded('');
    const first = projects.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a');
    const second = projects.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a');
    expect(listProjects).toHaveBeenCalledTimes(1);
    pending.resolve({ projects: [web] });
    await Promise.all([first, second]);
    await projects.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a');
    expect(listProjects).toHaveBeenCalledTimes(1);
    listProjects.mockResolvedValueOnce({ projects: [api] });
    await projects.ensureLoaded('YeDxg2kMYznPJJG3wVmjj');
    expect(listProjects).toHaveBeenCalledTimes(2);
    expect(projects.items).toEqual([api]);
  });

  it('adds a project once per org and slug, and updates it', () => {
    setClient('$projectClient', {});
    const projects = useProjectsStore();
    projects.add(web);
    projects.add({ ...web, id: 'dup' });
    projects.add({ ...web, id: '2qRquVZHg9cwCDFt72Xp2', orgId: 'YeDxg2kMYznPJJG3wVmjj' });
    expect(projects.items.map(p => p.id)).toEqual(['x6MH9WPWjktMwCBKHy4pF', '2qRquVZHg9cwCDFt72Xp2']);
    projects.update('x6MH9WPWjktMwCBKHy4pF', { name: 'Website', slug: 'site' });
    expect(projects.items[0]).toEqual({ ...web, name: 'Website', slug: 'site' });
    expect(projects.items[1]?.name).toBe('Web');
  });
});
