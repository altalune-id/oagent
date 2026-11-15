<script setup lang="ts">
import KeyValue from '#base/components/ui/KeyValue.vue';
import Section from '#base/components/ui/Section.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';

const { voice, vision, model, configureTo } = defineProps<{
  voice: { on: boolean, model: string, afterPublish: boolean },
  vision: boolean,
  model: string,
  configureTo: string,
}>();

const { t } = useI18n();
const items = computed(() => [{ label: t('configure.voice') }, { label: t('channels.photos') }]);
</script>

<template>
  <Section :title="t('channels.media_title')" :description="t('channels.media_hint')">
    <template #actions>
      <NuxtLink :to="`${configureTo}#configure-voice`" class="link text-ui">
        {{ t('channels.change_in_configure') }}
      </NuxtLink>
    </template>
    <div class="-mx-4 border-t border-base-300" :class="voice.afterPublish ? 'border-b' : '-mb-4'">
      <KeyValue :items="items">
        <template #value-0>
          <StatusLabel v-if="voice.on" tone="success" :label="t('channels.on')" :detail="voice.model" />
          <StatusLabel v-else tone="neutral" :label="t('channels.off')" :detail="t('channels.voice_off_detail', { reply: t('channels.reply_voice') })" />
        </template>
        <template #value-1>
          <StatusLabel v-if="vision" tone="success" :label="t('channels.on')" :detail="model" />
          <StatusLabel v-else tone="neutral" :label="t('channels.off')" :detail="model ? t('channels.photos_off_detail', { model }) : ''" />
          <NuxtLink v-if="!vision && model" :to="`${configureTo}#configure-model`" class="link text-ui">
            {{ t('channels.pick_vision') }}
          </NuxtLink>
        </template>
      </KeyValue>
    </div>
    <p v-if="voice.afterPublish" role="note" class="text-xs text-base-content/70">
      {{ t('channels.after_publish') }}
    </p>
  </Section>
</template>
