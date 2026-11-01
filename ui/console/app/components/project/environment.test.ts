import { describe, expect, it } from 'vitest';
import { ProjectEnvironment } from '#gen/project/v1/project_pb';
import { environmentLabelKey, environmentOptions, showsBanner } from './environment';

describe('project environment', () => {
  it('offers no tag first, then the four tags', () => {
    expect(environmentOptions.map(o => o.labelKey)).toEqual(['environment.none', 'environment.production', 'environment.staging', 'environment.development', 'environment.labs']);
  });

  it('names a tag and reads an unknown one as none', () => {
    expect(environmentLabelKey(ProjectEnvironment.LABS)).toBe('environment.labs');
    expect(environmentLabelKey(99 as ProjectEnvironment)).toBe('environment.none');
    expect(environmentLabelKey()).toBe('environment.none');
  });

  it('shows the banner for staging, development and labs, never for production or no tag', () => {
    expect([ProjectEnvironment.UNSPECIFIED, ProjectEnvironment.PRODUCTION, ProjectEnvironment.STAGING, ProjectEnvironment.DEVELOPMENT, ProjectEnvironment.LABS].map(showsBanner)).toEqual([false, false, true, true, true]);
  });
});
