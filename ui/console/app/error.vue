<script setup lang="ts">
import type { NuxtError } from '#app';
import { errorKind } from '#base/components/ui/errorPage';
import ErrorPage from '#base/components/ui/ErrorPage.vue';

const { error } = defineProps<{ error: NuxtError }>();

const { t } = useI18n();
const route = useRoute();
const kind = computed(() => errorKind(error.statusCode));
const title = computed(() => t(`error_page.${kind.value}_title`));
const body = computed(() => (kind.value !== 'failed' && error.statusMessage) || t(`error_page.${kind.value}_body`));

function home() {
  return clearError({ redirect: '/orgs' });
}

function retry() {
  return clearError({ redirect: route.fullPath });
}

useHead({ title: () => `${title.value} · OAgent` });
</script>

<template>
  <NuxtLayout>
    <ErrorPage
      :kind="kind"
      :title="title"
      :body="body"
      :home-label="t('org.all')"
      :retry-label="t('common.retry')"
      @home="home"
      @retry="retry"
    />
  </NuxtLayout>
</template>
