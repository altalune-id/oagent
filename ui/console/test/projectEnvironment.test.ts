// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { ProjectEnvironment, ProjectSchema } from '#gen/project/v1/project_pb';
import { mountPage } from '#test/mount-page';
import { connectError, setClient, setKnownKeys, setRoute } from '#test/nuxt-globals';
import OrgShell from '~/pages/o/[org].vue';
import SettingsGeneral from '~/pages/o/[org]/projects/[project]/settings.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
let updateProjectEnvironment: ReturnType<typeof vi.fn>;

function seed(environment: ProjectEnvironment, permissions = ['project:read', 'project:write']) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'web', name: 'Web', environment }];
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { permissions })) });
}

beforeEach(() => {
  setActivePinia(createPinia());
  updateProjectEnvironment = vi.fn().mockResolvedValue({ project: create(ProjectSchema, { id: projectId, environment: ProjectEnvironment.LABS }) });
  setClient('$projectClient', { updateProjectEnvironment, listProjects: vi.fn().mockResolvedValue({ projects: [] }) });
  setRoute({ path: '/o/acme/projects/web/settings', params: { org: 'acme', project: 'web' } });
});

describe('project environment tag', () => {
  it('lets an editor tag the project and keeps the tag in the project list', async () => {
    seed(ProjectEnvironment.UNSPECIFIED);
    const w = await mountPage(SettingsGeneral);
    const select = w.get('[data-testid="environment-card"] select');
    expect(select.findAll('option').map(o => o.text())).toEqual(['environment.none', 'environment.production', 'environment.staging', 'environment.development', 'environment.labs']);
    await select.setValue(String(ProjectEnvironment.LABS));
    await flushPromises();
    expect(updateProjectEnvironment).toHaveBeenCalledWith({ id: projectId, environment: ProjectEnvironment.LABS });
    expect(useProjectsStore().items[0]!.environment).toBe(ProjectEnvironment.LABS);
    expect(w.text()).toContain('common.saved');
  });

  it('shows a failed save, and the tag read-only to members', async () => {
    setKnownKeys('apperror.GEN007');
    updateProjectEnvironment.mockRejectedValue(connectError('GEN007'));
    seed(ProjectEnvironment.STAGING);
    const editor = await mountPage(SettingsGeneral);
    await editor.get('[data-testid="environment-card"] select').setValue(String(ProjectEnvironment.LABS));
    await flushPromises();
    expect(editor.get('[data-testid="environment-card"]').text()).toContain('apperror.GEN007');
    editor.unmount();

    setActivePinia(createPinia());
    seed(ProjectEnvironment.STAGING, ['project:read']);
    const member = await mountPage(SettingsGeneral);
    expect(member.find('[data-testid="environment-card"] select').exists()).toBe(false);
    expect(member.get('[data-testid="environment-card"]').text()).toContain('environment.staging');
    member.unmount();
  });

  it.each([
    [ProjectEnvironment.LABS, true],
    [ProjectEnvironment.STAGING, true],
    [ProjectEnvironment.PRODUCTION, false],
    [ProjectEnvironment.UNSPECIFIED, false],
  ])('puts the slim banner over every project page for tag %s: %s', async (env, shown) => {
    setClient('$projectClient', { listProjects: vi.fn().mockResolvedValue({ projects: [create(ProjectSchema, { id: projectId, orgId, slug: 'web', name: 'Web', environment: env })] }) });
    seed(env);
    useOrgsStore().loaded = true;
    vi.stubGlobal('definePageMeta', vi.fn());
    const stubs = { NuxtPage: true, ProjectSidebar: true, AppSidebar: true };
    const w = await mountPage(OrgShell, { global: { stubs } });
    expect(w.find('[data-testid="environment-banner"]').exists()).toBe(shown);
  });
});
