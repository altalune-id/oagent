<script setup lang="ts">
const { text = '', icon = 'lucide:eye' } = defineProps<{ text?: string, icon?: string }>();
const { t } = useI18n();
</script>

<template>
  <div role="note" class="alert items-start text-ui">
    <Icon :name="icon" size="16" class="shrink-0 text-info" aria-hidden="true" />
    <span>{{ text || t('project.read_only') }}</span>
  </div>
</template>
