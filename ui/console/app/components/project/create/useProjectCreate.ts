import type { ShownError } from '~/lib/apperror/message';

import { create } from '@bufbuild/protobuf';
import { CreateProjectRequestSchema } from '~~/gen/project/v1/project_pb';
import { useValidation } from '~/composables/useValidation';
import { useProjectsStore } from '~/stores/projects';
import { useUIStore } from '~/stores/ui';

export function useProjectCreate(orgId: string) {
  const name = ref('');
  const slug = ref('');
  const busy = ref(false);
  const error = ref<ShownError | null>(null);

  const projects = useProjectsStore();
  const ui = useUIStore();
  const validation = useValidation(CreateProjectRequestSchema, {
    fields: ['name', 'slug'],
    codes: { PRJ002: 'slug', PRJ003: 'slug', PRJ004: 'name' },
  });
  const { $projectClient: client } = useNuxtApp();

  const open = computed(() => ui.projectCreateOpen);

  function hide() {
    if (busy.value) {
      return;
    }
    ui.closeProjectCreate();
    error.value = null;
    validation.clear();
  }

  async function submit() {
    if (busy.value) {
      return;
    }
    const req = { orgId, name: name.value.trim(), slug: slug.value.trim() };
    if (!validation.check(create(CreateProjectRequestSchema, req))) {
      error.value = validation.formError.value;
      return;
    }
    busy.value = true;
    error.value = null;
    try {
      const res = await client.createProject(req);
      const project = res.project;
      if (project) {
        projects.add({ id: project.id, orgId: project.orgId, slug: project.slug, name: project.name });
        ui.closeProjectCreate();
        name.value = '';
        slug.value = '';
      }
    } catch (e) {
      error.value = validation.fromServer(e, 'error.create_project');
    } finally {
      busy.value = false;
    }
  }

  return { open, name, slug, busy, error, fieldErrors: validation.errors, show: ui.openProjectCreate, hide, submit };
}
