<script setup lang="ts">
import type { AgentLimits, WebToolsConfig } from './agentConfig';
import type { ConfigureLinks } from './runtime';
import type { FieldError as DraftFieldError } from './useAgentDraft';
import FieldError from '#base/components/ui/FieldError.vue';
import Section from '#base/components/ui/Section.vue';
import { focusIfLost } from '~/lib/focus/focus';
import { describedBy } from '~/lib/names/names';
import { parseCount, parseLines, parseSeconds, secretsFor, webHeaderMax, webListMax } from './runtime';

const { searchReady, secrets, links, fieldError = null, limits = {}, disabled = false } = defineProps<{
  searchReady?: boolean,
  secrets: { host: string }[],
  links: ConfigureLinks,
  fieldError?: DraftFieldError | null,
  limits?: AgentLimits,
  disabled?: boolean,
}>();

const web = defineModel<WebToolsConfig>({ required: true });
const invalidField = defineModel<string>('invalidField', { default: '' });
const { t } = useI18n();
const uid = useId();
const ids = { search: `${uid}-search`, domains: `${uid}-domains`, hosts: `${uid}-hosts`, headers: `${uid}-headers`, timeout: `${uid}-timeout`, calls: `${uid}-calls` };

const serverError = (...reasons: string[]) => (fieldError && reasons.includes(fieldError.reason) ? fieldError.message : '');
const anyOn = computed(() => web.value.search || web.value.fetch || web.value.http);
const secretCount = computed(() => secretsFor(web.value.httpHosts, secrets));
const maxTimeoutSeconds = computed(() => (limits.maxWebTimeoutMs ? limits.maxWebTimeoutMs / 1000 : undefined));
const tooMany = (max: number) => t('webtools.too_many', { max });

function useLines(key: 'fetchAllowDomains' | 'httpHosts' | 'requestHeaders', host: boolean, max: number) {
  const text = ref(web.value[key].join('\n'));
  const over = computed(() => parseLines(text.value, { host, max }).over);
  watch(text, (next) => {
    web.value = { ...web.value, [key]: parseLines(next, { host, max }).lines };
  });
  watch(() => web.value[key], (lines) => {
    if (parseLines(text.value, { host, max }).lines.join() !== lines.join()) {
      text.value = lines.join('\n');
    }
  });
  return { text, over };
}

function useNumber(key: 'timeoutMs' | 'maxCallsPerTurn', parse: (text: string) => number | null, toText: (n: number) => string) {
  const text = ref(web.value[key] ? toText(web.value[key]) : '');
  const invalid = computed(() => parse(text.value) === null);
  watch(text, (next) => {
    const n = parse(next);
    if (n !== null && n !== web.value[key]) {
      web.value = { ...web.value, [key]: n };
    }
  });
  watch(() => web.value[key], (n) => {
    if (parse(text.value) !== n) {
      text.value = n ? toText(n) : '';
    }
  });
  return { text, invalid };
}

const domains = useLines('fetchAllowDomains', true, webListMax);
const hosts = useLines('httpHosts', true, webListMax);
const headers = useLines('requestHeaders', false, webHeaderMax);
const timeout = useNumber('timeoutMs', text => parseSeconds(text, limits.maxWebTimeoutMs), n => String(n / 1000));
const calls = useNumber('maxCallsPerTurn', text => parseCount(text, limits.maxWebMaxCallsPerTurn), String);

watch(() => (anyOn.value && timeout.invalid.value ? 'web-timeout' : anyOn.value && calls.invalid.value ? 'web-calls' : ''), (field) => {
  invalidField.value = field;
}, { immediate: true });

const searchError = computed(() => serverError('web_search_provider'));
const searchHint = ref<HTMLElement>();
watch(() => web.value.search, (on) => {
  if (!on && searchReady === false) {
    setTimeout(() => focusIfLost(searchHint.value?.querySelector('a')));
  }
});
const domainsError = computed(() => {
  if (domains.over.value) {
    return tooMany(webListMax);
  }
  const required = web.value.fetchAllowDomains.length === 0 ? t('webtools.allowlist_required') : '';
  return serverError('fetch_allow_domains', 'config.web.fetch_allow_domains') || required;
});
const hostsError = computed(() => {
  if (hosts.over.value) {
    return tooMany(webListMax);
  }
  return serverError('http_hosts', 'config.web.http_hosts') || (web.value.httpHosts.length === 0 ? t('webtools.hosts_required') : '');
});
const headersError = computed(() => (headers.over.value ? tooMany(webHeaderMax) : serverError('config.web.request_headers')));
const timeoutError = computed(() => (timeout.invalid.value ? t('webtools.timeout_invalid', { max: maxTimeoutSeconds.value ?? '' }) : serverError('config.web.timeout_ms')));
const callsError = computed(() => (calls.invalid.value ? t('webtools.calls_invalid', { max: limits.maxWebMaxCallsPerTurn ?? '' }) : serverError('config.web.max_calls_per_turn')));
</script>

<template>
  <Section id="configure-web" :title="t('webtools.title')" :description="t('webtools.section_hint')" class="scroll-mt-14">
    <div class="flex flex-col gap-1">
      <label class="label cursor-pointer gap-2 text-ui text-base-content">
        <input
          v-model="web.search" type="checkbox" class="toggle toggle-sm" data-field="web-search" :disabled="disabled || (!web.search && searchReady === false)"
          :aria-invalid="!!searchError" :aria-describedby="describedBy(searchReady === false && `${ids.search}-hint`, !!searchError && `${ids.search}-err`)"
        >
        <span><span class="font-mono text-xs">web_search</span> · {{ t('webtools.search') }}</span>
      </label>
      <p v-if="searchReady === false" :id="`${ids.search}-hint`" ref="searchHint" class="flex items-start gap-1 text-xs text-base-content/70 sm:ps-12">
        <Icon v-if="web.search" name="lucide:triangle-alert" size="14" class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
        <span>
          {{ web.search ? t('webtools.search_on_no_provider') : t('webtools.search_needs_provider') }}
          <NuxtLink :to="links.webTools" class="link">
            {{ t('webtools.search_link') }}
          </NuxtLink>
        </span>
      </p>
      <FieldError :id="`${ids.search}-err`" :message="searchError" class="sm:ps-12" />
    </div>

    <div class="flex flex-col gap-2">
      <label class="label cursor-pointer gap-2 text-ui text-base-content">
        <input v-model="web.fetch" type="checkbox" class="toggle toggle-sm" :disabled="disabled">
        <span><span class="font-mono text-xs">web_fetch</span> · {{ t('webtools.fetch') }}</span>
      </label>
      <div v-if="web.fetch" class="flex flex-col gap-1 ps-0 sm:ps-12">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ t('webtools.allowed_domains') }}<span class="text-base-content/70"> ({{ t('validation.required') }})</span></span>
          <textarea
            v-model="domains.text.value" data-field="web-domains" rows="3" class="textarea textarea-sm w-full font-mono text-ui" :class="{ 'textarea-error': domainsError }" placeholder="bca.co.id" :disabled="disabled"
            :aria-invalid="!!domainsError" :aria-describedby="describedBy(`${ids.domains}-hint`, !!domainsError && `${ids.domains}-err`)"
          />
        </label>
        <p :id="`${ids.domains}-hint`" class="text-xs text-base-content/70">
          {{ t('webtools.allowlist_hint') }} {{ t('webtools.host_punycode') }}
        </p>
        <FieldError :id="`${ids.domains}-err`" :message="domainsError" />
      </div>
    </div>

    <div class="flex flex-col gap-2">
      <label class="label cursor-pointer gap-2 text-ui text-base-content">
        <input v-model="web.http" type="checkbox" class="toggle toggle-sm" :disabled="disabled">
        <span><span class="font-mono text-xs">http_request</span> · {{ t('webtools.http') }}</span>
      </label>
      <div v-if="web.http" class="flex flex-col gap-3 ps-0 sm:ps-12">
        <p class="text-xs text-base-content/70">
          {{ t('webtools.http_get_note') }}
        </p>
        <div class="flex flex-col gap-1">
          <label class="flex flex-col gap-1">
            <span class="label text-ui text-base-content">{{ t('webtools.http_hosts') }}<span class="text-base-content/70"> ({{ t('validation.required') }})</span></span>
            <textarea
              v-model="hosts.text.value" data-field="web-hosts" rows="3" class="textarea textarea-sm w-full font-mono text-ui" :class="{ 'textarea-error': hostsError }" placeholder="api.example.com" :disabled="disabled"
              :aria-invalid="!!hostsError" :aria-describedby="describedBy(`${ids.hosts}-hint`, !!hostsError && `${ids.hosts}-err`)"
            />
          </label>
          <p :id="`${ids.hosts}-hint`" class="text-xs text-base-content/70">
            {{ t('webtools.http_hosts_hint') }} {{ t('webtools.host_punycode') }}
            <NuxtLink :to="links.webTools" class="link">
              {{ t('webtools.secrets_count', { n: secretCount }) }}
            </NuxtLink>
          </p>
          <FieldError :id="`${ids.hosts}-err`" :message="hostsError" />
        </div>
        <div class="flex flex-col gap-1">
          <label class="flex flex-col gap-1">
            <span class="label text-ui text-base-content">{{ t('webtools.request_headers') }}</span>
            <textarea
              v-model="headers.text.value" data-field="web-headers" rows="2" class="textarea textarea-sm w-full font-mono text-ui" :class="{ 'textarea-error': headersError }" placeholder="X-Request-Id" :disabled="disabled"
              :aria-invalid="!!headersError" :aria-describedby="describedBy(`${ids.headers}-hint`, !!headersError && `${ids.headers}-err`)"
            />
          </label>
          <p :id="`${ids.headers}-hint`" class="text-xs text-base-content/70">
            {{ t('webtools.request_headers_hint') }}
          </p>
          <FieldError :id="`${ids.headers}-err`" :message="headersError" />
        </div>
      </div>
    </div>

    <div v-if="anyOn" class="flex flex-col gap-3 sm:flex-row sm:gap-6">
      <div class="flex flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ t('webtools.timeout') }}</span>
          <input
            v-model="timeout.text.value" type="text" inputmode="numeric" data-field="web-timeout" class="input input-sm w-24 tabular-nums" :class="{ 'input-error': timeoutError }" :disabled="disabled"
            :placeholder="limits.webTimeoutMs ? String(limits.webTimeoutMs / 1000) : undefined"
            :aria-invalid="!!timeoutError" :aria-describedby="describedBy(`${ids.timeout}-hint`, !!timeoutError && `${ids.timeout}-err`)"
          >
        </label>
        <p :id="`${ids.timeout}-hint`" class="text-xs text-base-content/70">
          {{ t('webtools.timeout_hint') }}
        </p>
        <FieldError :id="`${ids.timeout}-err`" :message="timeoutError" />
      </div>
      <div class="flex flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ t('webtools.max_calls') }}</span>
          <input
            v-model="calls.text.value" type="text" inputmode="numeric" data-field="web-calls" class="input input-sm w-24 tabular-nums" :class="{ 'input-error': callsError }" :disabled="disabled"
            :placeholder="limits.webMaxCallsPerTurn ? String(limits.webMaxCallsPerTurn) : undefined"
            :aria-invalid="!!callsError" :aria-describedby="describedBy(`${ids.calls}-hint`, !!callsError && `${ids.calls}-err`)"
          >
        </label>
        <p :id="`${ids.calls}-hint`" class="text-xs text-base-content/70">
          {{ t('webtools.max_calls_hint') }}
        </p>
        <FieldError :id="`${ids.calls}-err`" :message="callsError" />
      </div>
    </div>
  </Section>
</template>
