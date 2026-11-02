<script setup lang="ts">
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FormField from '#base/components/ui/FormField.vue';
import { useAgentCreate } from './useAgentCreate';

const { orgId, projectId, label } = defineProps<{ orgId: string, projectId: string, label?: string }>();

const { t } = useI18n();
const { open, name, slug, description, busy, error, fieldErrors, show, hide, submit } = useAgentCreate(orgId, projectId);
</script>

<template>
  <AppButton size="sm" @click="show()">
    <Icon name="lucide:plus" size="16" />
    {{ label ?? t('agent.new') }}
  </AppButton>

  <AppModal :open="open" :title="t('agent.new')" @close="hide()">
    <form novalidate class="flex flex-col gap-3" @submit.prevent="submit()">
      <FormField v-slot="{ id, describedBy, invalid }" :label="t('common.name')" :error="fieldErrors.name">
        <input
          :id="id" v-model="name" type="text" class="input input-sm text-ui w-full" :class="{ 'input-error': invalid }"
          :placeholder="t('agent.name_placeholder')" :disabled="busy" :aria-invalid="invalid" :aria-describedby="describedBy"
        >
      </FormField>
      <FormField v-slot="{ id, describedBy, invalid }" :label="t('common.slug')" :hint="t('agent.slug_random_help')" :error="fieldErrors.slug">
        <input
          :id="id" v-model="slug" type="text" class="input input-sm text-ui w-full font-mono" :class="{ 'input-error': invalid }"
          :placeholder="t('agent.slug_placeholder')" :disabled="busy" :aria-invalid="invalid" :aria-describedby="describedBy"
        >
      </FormField>
      <FormField v-slot="{ id, describedBy, invalid }" :label="t('agent.description_label')" :error="fieldErrors.description">
        <textarea
          :id="id" v-model="description" rows="3" class="textarea text-ui w-full" :class="{ 'textarea-error': invalid }"
          :placeholder="t('agent.description_placeholder')" :disabled="busy" :aria-invalid="invalid" :aria-describedby="describedBy"
        />
      </FormField>
      <ErrorText :error="error" />
    </form>
    <template #footer>
      <AppButton variant="plain" size="sm" :disabled="busy" @click="hide()">
        {{ t('common.cancel') }}
      </AppButton>
      <AppButton size="sm" :busy="busy" :disabled="!name.trim()" @click="submit()">
        {{ busy ? t('common.creating') : t('agent.create_and_configure') }}
      </AppButton>
    </template>
  </AppModal>
</template>
