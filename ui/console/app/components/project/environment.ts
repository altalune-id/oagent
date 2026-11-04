import { ProjectEnvironment } from '#gen/project/v1/project_pb';

export interface EnvironmentOption {
  value: ProjectEnvironment;
  labelKey: string;
}

// environmentOptions is every tag a project may carry, "no tag" first.
export const environmentOptions: EnvironmentOption[] = [
  { value: ProjectEnvironment.UNSPECIFIED, labelKey: 'environment.none' },
  { value: ProjectEnvironment.PRODUCTION, labelKey: 'environment.production' },
  { value: ProjectEnvironment.STAGING, labelKey: 'environment.staging' },
  { value: ProjectEnvironment.DEVELOPMENT, labelKey: 'environment.development' },
  { value: ProjectEnvironment.LABS, labelKey: 'environment.labs' },
];

// environmentLabelKey names a tag; unknown values read as no tag.
export function environmentLabelKey(env?: ProjectEnvironment): string {
  return environmentOptions.find(o => o.value === env)?.labelKey ?? 'environment.none';
}

// showsBanner is true for a tag that is not production, so people know changes here stay out of production.
export function showsBanner(env?: ProjectEnvironment): boolean {
  return env === ProjectEnvironment.STAGING || env === ProjectEnvironment.DEVELOPMENT || env === ProjectEnvironment.LABS;
}
