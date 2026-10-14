export const useUIStore = defineStore('ui', () => {
  const orgCreateOpen = ref(false);
  const projectCreateOpen = ref(false);
  const navOpen = ref(false);

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

  return { orgCreateOpen, projectCreateOpen, navOpen, openOrgCreate, closeOrgCreate, openProjectCreate, closeProjectCreate };
});
