<script setup lang="ts">
import EmptyState from '#base/components/ui/EmptyState.vue';

const { error } = defineProps<{ error: { statusCode?: number, statusMessage?: string } }>();

const { t } = useI18n();
const route = useRoute();

const kind = computed(() => {
  switch (error.statusCode) {
    case 404: return { title: t('error.page_not_found'), body: t('error.page_not_found_body'), icon: 'lucide:map-pin-off', retry: false };
    case 403: return { title: t('error.page_forbidden'), body: t('error.page_forbidden_body'), icon: 'lucide:lock', retry: false };
    default: return { title: t('error.page_failed'), body: t('error.page_failed_body'), icon: 'lucide:cloud-off', retry: true };
  }
});

useHead({ title: () => `${kind.value.title} · OAgent` });

function goHome() {
  return clearError({ redirect: '/orgs' });
}

function retry() {
  return clearError({ redirect: route.fullPath });
}
</script>

<template>
  <NuxtLayout>
    <main class="mx-auto w-full max-w-xl p-4 sm:p-10">
      <EmptyState :title="kind.title" :body="kind.body" :icon="kind.icon" :heading-level="1">
        <template #actions>
          <button type="button" class="btn btn-primary btn-sm" @click="goHome()">
            {{ t('error.go_orgs') }}
          </button>
          <button v-if="kind.retry" type="button" class="btn btn-sm" @click="retry()">
            {{ t('common.retry') }}
          </button>
        </template>
      </EmptyState>
      <p v-if="error.statusMessage" class="mt-4 text-center text-sm text-base-content/70">
        {{ error.statusMessage }}
      </p>
    </main>
  </NuxtLayout>
</template>
