export const useUIStore = defineStore('ui', () => {
  const orgCreateOpen = ref(false);
  const projectCreateOpen = ref(false);
  const navOpen = ref(false);
  const rail = ref(false);

  function openOrgCreate() {
    orgCreateOpen.value = true;
  }

  function closeOrgCreate() {
    orgCreateOpen.value = false;
  }

  function openProjectCreate() {
    projectCreateOpen.value = true;
  }

  function closeProjectCreate() {
    projectCreateOpen.value = false;
  }

  return { orgCreateOpen, projectCreateOpen, navOpen, rail, openOrgCreate, closeOrgCreate, openProjectCreate, closeProjectCreate };
});
