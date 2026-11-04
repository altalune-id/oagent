import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

// useProject resolves the org and project slugs in the route, and throws a 404 when the project is unknown.
export function useProject() {
  const route = useRoute();
  const { t } = useI18n();
  const orgs = useOrgsStore();
  const projects = useProjectsStore();
  const org = computed(() => orgs.bySlug(route.params.org as string));
  const project = computed(() => projects.items.find(p => p.orgId === org.value?.id && p.slug === route.params.project));
  if (!project.value) {
    throw createError({ statusCode: 404, statusMessage: t('error.project_not_found'), fatal: true });
  }
  return { org, project };
}
