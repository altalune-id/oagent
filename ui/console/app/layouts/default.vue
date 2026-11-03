<script setup lang="ts">
import { provide } from 'vue';
import { useTheme } from '#base/components/layout/useTheme';
import { copiedLabelKey } from '#base/components/ui/copy';
import ToastHost from '#base/components/ui/ToastHost.vue';
import AppNavbar from '~/components/layout/AppNavbar.vue';
import OrgCreateModal from '~/components/org/create/OrgCreateModal.vue';
import ProjectCreateModal from '~/components/project/create/ProjectCreateModal.vue';
import { staticURL } from '~/lib/auth/urls';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const { t, locale } = useI18n();
const orgs = useOrgsStore();
provide(copiedLabelKey, () => t('common.copied'));
onMounted(useTheme().init);

const { apiUrl } = useRuntimeConfig().public;

useHead({
  htmlAttrs: { lang: locale },
  bodyAttrs: { class: 'text-sm leading-[normal]' },
  link: [
    { key: 'icon', rel: 'icon', href: staticURL(apiUrl, 'favicon.ico'), sizes: 'any' },
    { rel: 'icon', type: 'image/png', sizes: '32x32', href: staticURL(apiUrl, 'favicon-32x32.png') },
    { rel: 'apple-touch-icon', href: staticURL(apiUrl, 'apple-touch-icon.png') },
  ],
});

const orgId = computed(() => {
  const slug = route.params.org as string | undefined;
  return (slug ? orgs.bySlug(slug)?.id : '') ?? '';
});
</script>

<template>
  <div class="flex min-h-dvh flex-col bg-base-100 text-base-content">
    <AppNavbar />
    <slot />
    <ToastHost :close-label="t('common.close')" />
    <OrgCreateModal />
    <ProjectCreateModal v-if="orgId" :org-id="orgId" />
  </div>
</template>
