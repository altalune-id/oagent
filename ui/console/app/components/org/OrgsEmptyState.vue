<script setup lang="ts">
export interface OrgsEmptyStateLabels {
  title: string;
  body: string;
  signOut: string;
}

const { logoutUrl, labels } = defineProps<{ logoutUrl: string, labels: OrgsEmptyStateLabels }>();
</script>

<template>
  <div class="card mt-6 border border-base-300 bg-base-100">
    <div class="card-body items-center text-center">
      <Icon name="lucide:building-2" size="32" class="text-base-content/40" />
      <h2 class="card-title text-base">
        {{ labels.title }}
      </h2>
      <p class="text-sm text-base-content/60">
        {{ labels.body }}
      </p>
      <div class="card-actions mt-2 justify-center">
        <form method="post" :action="logoutUrl">
          <button type="submit" class="btn btn-sm btn-ghost gap-2">
            <Icon name="lucide:log-out" size="16" />
            {{ labels.signOut }}
          </button>
        </form>
      </div>
    </div>
  </div>
</template>
