<script setup lang="ts">
import type { ReplyKey, ReplyTexts } from './replies';
import { useId } from 'vue';
import AppButton from '#base/components/ui/AppButton.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import { replyKeys } from './replies';

export interface ReplyEditorLabels {
  legend: string;
  hint: string;
  fields: Record<ReplyKey, { label: string, hint: string }>;
  reset: string;
  save: string;
}

const { defaults, labels, errors = {}, readOnly = false } = defineProps<{ defaults: ReplyTexts, labels: ReplyEditorLabels, errors?: Partial<Record<ReplyKey, string>>, readOnly?: boolean }>();
const emit = defineEmits<{ save: [] }>();
const uid = useId();
const replies = defineModel<ReplyTexts>({ required: true });

function reset(key: ReplyKey) {
  replies.value = { ...replies.value, [key]: defaults[key] };
}
</script>

<template>
  <form novalidate class="flex flex-col gap-3" @submit.prevent="emit('save')">
    <fieldset class="fieldset gap-3" :disabled="readOnly">
      <legend class="sr-only">
        {{ labels.legend }}
      </legend>
      <p class="text-xs text-base-content/70">
        {{ labels.hint }}
      </p>
      <div v-for="key in replyKeys" :key="key" class="flex min-w-0 flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ labels.fields[key].label }}</span>
          <textarea
            v-model="replies[key]" rows="2" class="textarea textarea-sm w-full text-ui" :class="{ 'textarea-error': errors[key] }"
            :aria-invalid="!!errors[key]" :aria-describedby="`${uid}-${key}-hint ${uid}-${key}-err`"
          />
        </label>
        <p :id="`${uid}-${key}-hint`" class="label whitespace-normal text-xs text-base-content/70">
          {{ labels.fields[key].hint }}
        </p>
        <FieldError :id="`${uid}-${key}-err`" :message="errors[key]" />
        <div v-if="!readOnly && replies[key] !== defaults[key]">
          <AppButton variant="plain" size="xs" @click="reset(key)">
            <Icon name="lucide:rotate-ccw" size="14" aria-hidden="true" />
            {{ labels.reset }}
          </AppButton>
        </div>
      </div>
    </fieldset>
    <div v-if="!readOnly">
      <AppButton type="submit" size="sm">
        {{ labels.save }}
      </AppButton>
    </div>
  </form>
</template>
