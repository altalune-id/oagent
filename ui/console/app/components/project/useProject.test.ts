import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { setRoute } from '#test/nuxt-globals';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useProject } from './useProject';

describe('useProject', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    useOrgsStore().items = [{ id: 'B6rTn1YkM8wQz3XpL5vGd', slug: 'acme', name: 'Acme', role: 'owner' }];
    useProjectsStore().items = [{ id: 'x6MH9WPWjktMwCBKHy4pF', orgId: 'B6rTn1YkM8wQz3XpL5vGd', slug: 'rumah', name: 'Rumah Tangga' }];
  });

  it('resolves the org and project from the route', () => {
    setRoute({ params: { org: 'acme', project: 'rumah' } });
    const { org, project } = useProject();
    expect(org.value?.name).toBe('Acme');
    expect(project.value?.name).toBe('Rumah Tangga');
  });

  it('throws a 404 for an unknown project', () => {
    setRoute({ params: { org: 'acme', project: 'nope' } });
    expect(() => useProject()).toThrow(expect.objectContaining({ statusCode: 404 }));
  });
});
