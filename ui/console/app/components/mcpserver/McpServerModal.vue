<script setup lang="ts">
import type { useMcpServerForm } from './useMcpServerForm';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import CodeText from '~/components/layout/CodeText.vue';
import { describedBy } from '~/lib/names/names';
import { useMcpServersStore } from '~/stores/mcpServers';
import McpServerTools from './McpServerTools.vue';

const { form, readOnly = false } = defineProps<{ form: ReturnType<typeof useMcpServerForm>, readOnly?: boolean }>();

const { t } = useI18n();
const uid = useId();
const ids = { name: `${uid}-name`, url: `${uid}-url`, header: `${uid}-header`, secret: `${uid}-secret`, timeout: `${uid}-timeout` };
const { open, editing, name, url, auth, headerName, secret, replacingSecret, trusted, timeout, errors, formError, busy, urlChanged, needsSecret, secretShown, secretRequired } = form;
const servers = useMcpServersStore();

const shownName = computed(() => editing.value?.name ?? (name.value.trim() || t('mcpservers.example_server')));
const title = computed(() => (editing.value ? editing.value.name : t('mcpservers.add')));
const saved = computed(() => servers.items.find(s => s.id === editing.value?.id));
</script>

<template>
  <AppModal :open="open" :title="title" wide sticky-footer @close="form.hide()">
    <form novalidate class="flex flex-col gap-3" @submit.prevent="form.submit()">
      <fieldset class="flex flex-col gap-3" :disabled="readOnly">
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('common.name') }}</span>
          <input v-model="name" type="text" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.name }" placeholder="yasaku" :disabled="!!editing" :aria-invalid="!!errors.name" :aria-describedby="describedBy(`${ids.name}-hint`, !!errors.name && `${ids.name}-err`)">
        </label>
        <p :id="`${ids.name}-hint`" class="-mt-2 text-xs text-base-content/70">
          {{ t('mcpservers.name_hint', { example: `${shownName}__<tool>` }) }}
        </p>
        <FieldError :id="`${ids.name}-err`" :message="errors.name" />
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('mcpservers.url') }}</span>
          <input v-model="url" type="url" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.url }" placeholder="https://yasaku.altalune.id/mcp" :aria-invalid="!!errors.url" :aria-describedby="describedBy(`${ids.url}-hint`, !!errors.url && `${ids.url}-err`)">
        </label>
        <p :id="`${ids.url}-hint`" class="-mt-2 text-xs text-base-content/70">
          {{ t('mcpservers.url_hint') }}
        </p>
        <FieldError :id="`${ids.url}-err`" :message="errors.url" />
        <div v-if="urlChanged && needsSecret" role="alert" class="alert alert-warning alert-soft text-base-content text-ui">
          <Icon name="lucide:triangle-alert" size="16" class="shrink-0" aria-hidden="true" />
          <span>{{ t('mcpservers.url_changed_secret') }}</span>
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <label class="flex flex-col gap-1">
            <span class="label text-ui font-semibold text-base-content">{{ t('mcpservers.auth') }}</span>
            <select v-model="auth" class="select select-sm w-full text-ui">
              <option value="none">
                {{ t('mcpservers.auth_none') }}
              </option>
              <option value="bearer">
                {{ t('mcpservers.auth_bearer') }}
              </option>
              <option value="header">
                {{ t('mcpservers.auth_header') }}
              </option>
            </select>
          </label>
          <div v-if="auth === 'header'" class="flex flex-col gap-1">
            <label class="flex flex-col gap-1">
              <span class="label text-ui font-semibold text-base-content">{{ t('mcpservers.header_name') }}</span>
              <input v-model="headerName" type="text" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.headerName }" placeholder="X-Api-Key" :aria-invalid="!!errors.headerName" :aria-describedby="describedBy(!!errors.headerName && `${ids.header}-err`)">
            </label>
            <FieldError :id="`${ids.header}-err`" :message="errors.headerName">
              <CodeText :text="errors.headerName ?? ''" />
            </FieldError>
          </div>
        </div>
        <template v-if="needsSecret">
          <div v-if="secretShown" class="flex flex-col gap-1">
            <label class="flex flex-col gap-1">
              <span class="label text-ui font-semibold text-base-content">{{ t('mcpservers.secret') }}<span v-if="secretRequired" class="font-normal text-base-content/70"> ({{ t('validation.required') }})</span></span>
              <input v-model="secret" type="password" autocomplete="new-password" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.secret }" :aria-invalid="!!errors.secret" :aria-describedby="describedBy(`${ids.secret}-hint`, !!errors.secret && `${ids.secret}-err`)">
            </label>
            <p :id="`${ids.secret}-hint`" class="text-xs text-base-content/70">
              {{ t('secret.write_only') }}
            </p>
            <FieldError :id="`${ids.secret}-err`" :message="errors.secret" />
          </div>
          <div v-else class="flex flex-wrap items-center gap-3">
            <StatusLabel tone="success" :label="t('mcpservers.secret_set')" />
            <AppButton variant="plain" size="xs" @click="replacingSecret = true">
              {{ t('mcpservers.replace_secret') }}
            </AppButton>
          </div>
        </template>
        <div class="flex flex-col gap-1">
          <label class="label cursor-pointer gap-2 text-ui text-base-content">
            <input v-model="trusted" type="checkbox" class="toggle toggle-sm">
            {{ t('mcpservers.trusted') }}
          </label>
          <p class="text-xs text-base-content/70">
            {{ t('mcpservers.trusted_hint') }}
          </p>
        </div>
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('mcpservers.timeout') }}</span>
          <input v-model.number="timeout" type="number" min="1" max="20" step="any" class="input input-sm w-32 tabular-nums" :class="{ 'input-error': errors.timeout }" :aria-invalid="!!errors.timeout" :aria-describedby="describedBy(`${ids.timeout}-hint`, !!errors.timeout && `${ids.timeout}-err`)">
        </label>
        <p :id="`${ids.timeout}-hint`" class="-mt-2 text-xs text-base-content/70">
          {{ t('mcpservers.timeout_hint') }}
        </p>
        <FieldError :id="`${ids.timeout}-err`" :message="errors.timeout" />
      </fieldset>

      <ErrorText :error="formError" />

      <McpServerTools v-if="saved" :server="saved" :trusted="trusted" @replace-secret="replacingSecret = true" />
      <p v-else class="text-xs text-base-content/70">
        {{ t('mcpservers.tools_after_save') }}
      </p>
    </form>

    <template #footer>
      <AppButton variant="plain" size="sm" @click="form.hide()">
        {{ readOnly ? t('common.close') : t('common.cancel') }}
      </AppButton>
      <AppButton v-if="!readOnly" size="sm" :busy="busy" @click="form.submit()">
        {{ t('mcpservers.save') }}
      </AppButton>
    </template>
  </AppModal>
</template>
