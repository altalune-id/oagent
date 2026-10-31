<script setup lang="ts">
const { text = '', icon = 'lucide:eye' } = defineProps<{ text?: string, icon?: string }>();
const { t } = useI18n();
</script>

<template>
  <div role="note" class="alert alert-soft">
    <Icon :name="icon" size="18" class="text-info" />
    <span class="text-sm text-base-content">{{ text || t('project.read_only') }}</span>
  </div>
</template>
