import type { ShownError } from '#base/lib/apperror/message';

import type { Project } from '~/stores/projects';
import { create } from '@bufbuild/protobuf';
import { useValidation } from '#base/composables/useValidation';
import { UpdateProjectRequestSchema } from '#gen/project/v1/project_pb';
import { useProjectsStore } from '~/stores/projects';

export function useProjectEdit() {
  const open = ref(false);
  const id = ref('');
  const name = ref('');
  const slug = ref('');
  const busy = ref(false);
  const error = ref<ShownError | null>(null);

  const projects = useProjectsStore();
  const validation = useValidation(UpdateProjectRequestSchema, {
    fields: ['name', 'slug'],
    codes: { PRJ002: 'slug', PRJ003: 'slug', PRJ004: 'name' },
  });
  const { $projectClient: client } = useNuxtApp();

  function show(project: Project) {
    id.value = project.id;
    name.value = project.name;
    slug.value = project.slug;
    error.value = null;
    validation.clear();
    open.value = true;
  }

  function hide() {
    if (busy.value) {
      return;
    }
    open.value = false;
    error.value = null;
    validation.clear();
  }

  async function submit() {
    if (busy.value) {
      return;
    }
    const req = { id: id.value, name: name.value.trim(), slug: slug.value.trim() };
    if (!validation.check(create(UpdateProjectRequestSchema, req))) {
      error.value = validation.formError.value;
      return;
    }
    busy.value = true;
    error.value = null;
    try {
      const res = await client.updateProject(req);
      if (res.project) {
        projects.update(res.project.id, { name: res.project.name, slug: res.project.slug });
      }
      open.value = false;
      name.value = '';
      slug.value = '';
    } catch (e) {
      error.value = validation.fromServer(e, 'error.rename_project');
    } finally {
      busy.value = false;
    }
  }

  return { open, name, slug, busy, error, fieldErrors: validation.errors, show, hide, submit };
}
