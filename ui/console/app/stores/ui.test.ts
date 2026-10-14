import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';

import { useUIStore } from './ui';

describe('useUIStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('opens and closes the create modals', () => {
    const ui = useUIStore();
    expect(ui.orgCreateOpen).toBe(false);
    ui.openOrgCreate();
    expect(ui.orgCreateOpen).toBe(true);
    ui.closeOrgCreate();
    expect(ui.orgCreateOpen).toBe(false);
    ui.openProjectCreate();
    expect(ui.projectCreateOpen).toBe(true);
    ui.closeProjectCreate();
    expect(ui.projectCreateOpen).toBe(false);
    expect(ui.navOpen).toBe(false);
  });
});
