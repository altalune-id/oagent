<script setup lang="ts">
import type { SetupStep } from './setupSteps';
import Section from '#base/components/ui/Section.vue';

const { steps } = defineProps<{ steps: SetupStep[] }>();

const { t } = useI18n();
const done = computed(() => steps.filter(s => s.state === 'done').length);
</script>

<template>
  <Section id="setup" :title="t('setup.title')" :description="t('setup.progress', { done, total: steps.length })" flush class="max-w-2xl" data-testid="setup-checklist">
    <template #actions>
      <span class="badge badge-info badge-soft text-base-content badge-sm">{{ t('preview.badge') }}</span>
    </template>
    <div class="border-b border-base-300 px-4 py-3">
      <progress class="progress block w-full" :value="done" :max="steps.length" :aria-label="t('setup.title')" :aria-valuetext="t('setup.progress', { done, total: steps.length })" />
    </div>
    <ol class="divide-y divide-base-300">
      <li v-for="(step, i) in steps" :key="step.key" class="flex items-center gap-3 px-4 py-2 text-ui" :aria-current="step.state === 'next' ? 'step' : undefined">
        <Icon v-if="step.state === 'done'" name="lucide:circle-check" size="16" class="shrink-0 text-success" aria-hidden="true" />
        <Icon v-else name="lucide:circle" size="16" class="shrink-0 text-base-content/70" aria-hidden="true" />
        <span class="min-w-0 flex-1">
          <span class="me-1 tabular-nums">{{ i + 1 }}.</span>
          <NuxtLink :to="step.to" class="link link-hover">{{ t(step.labelKey) }}</NuxtLink>
        </span>
        <NuxtLink v-if="step.state === 'next'" :to="step.to" class="btn btn-primary btn-xs" :aria-label="t('setup.start_label', { step: t(step.labelKey) })">
          {{ t('setup.start') }}
          <Icon name="lucide:arrow-right" size="12" aria-hidden="true" />
        </NuxtLink>
        <span v-else class="text-xs text-base-content/70">{{ step.state === 'done' ? t('setup.done') : t('setup.not_started') }}</span>
      </li>
    </ol>
  </Section>
</template>
