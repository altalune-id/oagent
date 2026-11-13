<script setup lang="ts">
import type { AgentConfig } from './agentConfig';
import type { ConfigureLinks } from './runtime';
import type { FieldError as DraftFieldError } from './useAgentDraft';
import type { AgentRuntimeConfig } from '~/stores/agentRuntime';
import type { KnowledgeSource, RetrievalDefaults } from '~/stores/knowledge';
import type { McpServer } from '~/stores/mcpServers';
import type { ModelProvider, ProviderModel } from '~/stores/modelProviders';
import AppButton from '#base/components/ui/AppButton.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import Section from '#base/components/ui/Section.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { ModelProviderKind } from '#gen/modelprovider/v1/modelprovider_pb';
import { sourceState } from '~/components/knowledge/knowledge';
import { usdText } from '~/lib/money/usd';
import { describedBy } from '~/lib/names/names';
import KnowledgeTuningForm from './KnowledgeTuning.vue';
import McpGrants from './McpGrants.vue';
import { bindable, chosenModel, costCapError, helperCapError, parseDollars, parseDomains, readsPrivateData } from './runtime';

const { providers, servers, sources, searchReady, embeddings, links, retrieval = null, legacyModel = '', disabled = false, fieldError = null } = defineProps<{
  providers: ModelProvider[],
  servers: McpServer[],
  sources: KnowledgeSource[],
  searchReady: boolean,
  embeddings: boolean,
  links: ConfigureLinks,
  retrieval?: RetrievalDefaults | null,
  legacyModel?: string,
  disabled?: boolean,
  fieldError?: DraftFieldError | null,
}>();

const config = defineModel<AgentConfig>({ required: true });
const runtime = defineModel<AgentRuntimeConfig>('runtime', { required: true });
const { t } = useI18n();
const uid = useId();
const ids = { count: `${uid}-count`, temperature: `${uid}-temp`, helper: `${uid}-helper`, provider: `${uid}-provider`, model: `${uid}-model`, tools: `${uid}-tools`, domains: `${uid}-domains`, steps: `${uid}-steps`, cost: `${uid}-cost` };

const sections = computed(() => [
  { id: 'configure-instructions', label: t('configure.instructions') },
  { id: 'configure-model', label: t('configure.model') },
  { id: 'configure-tools', label: t('grants.title') },
  { id: 'configure-web', label: t('webtools.title') },
  { id: 'configure-memory', label: t('configure.memory_knowledge') },
  { id: 'configure-limits', label: t('limits.title') },
]);

const chosen = computed(() => chosenModel(config.value, providers));
const temperatureOff = computed(() => chosen.value.provider?.kind === ModelProviderKind.ANTHROPIC);
const temperatureText = computed(() => (temperatureOff.value ? t('configure.temperature_off') : config.value.temperature.toFixed(1)));
const chatModels = computed(() => (chosen.value.provider?.models ?? []).filter(m => !m.embedding || m.id === config.value.model || m.id === config.value.helperModel));
const allowListRequired = computed(() => readsPrivateData(config.value, runtime.value));
const serverError = (...reasons: string[]) => (fieldError && reasons.includes(fieldError.reason) ? fieldError.message : '');

function modelLabel(m: ProviderModel) {
  const name = m.vision ? t('configure.model_vision', { model: m.id }) : m.id;
  return m.prices ? name : t('configure.model_unpriced_option', { model: name });
}

const priceLine = computed(() => {
  const p = chosen.value.model?.prices;
  if (!chosen.value.model) {
    return '';
  }
  return p ? t('configure.model_price', { input: usdText(p.inputMicros), output: usdText(p.outputMicros) }) : t('configure.model_unpriced');
});
const allowListError = computed(() => runtime.value.webFetch && allowListRequired.value && runtime.value.allowedDomains.length === 0 ? t('webtools.allowlist_required') : '');
const costText = ref(config.value.maxCostCents > 0 ? (config.value.maxCostCents / 100).toFixed(2) : '');
const costTyped = computed(() => parseDollars(costText.value));
const helperError = computed(() => {
  const key = helperCapError(config.value, providers);
  return key ? t(key, { model: config.value.helperModel, provider: chosen.value.provider?.name ?? '' }) : serverError('helper_model');
});
const costError = computed(() => {
  if (costTyped.value === null) {
    return t('limits.not_amount');
  }
  const key = costCapError(config.value, providers);
  return key ? t(key, { provider: chosen.value.provider?.name ?? '' }) : serverError('budget_unpriced');
});

watch(costTyped, (cents) => {
  if (cents !== null && cents !== config.value.maxCostCents) {
    config.value = { ...config.value, maxCostCents: cents };
  }
});

const domainsText = ref(runtime.value.allowedDomains.join('\n'));
watch(domainsText, (text) => {
  runtime.value = { ...runtime.value, allowedDomains: parseDomains(text) };
});
watch(() => runtime.value.allowedDomains, (domains) => {
  if (parseDomains(domainsText.value).join() !== domains.join()) {
    domainsText.value = domains.join('\n');
  }
});

function goTo(id: string) {
  const section = document.getElementById(id);
  section?.scrollIntoView({ block: 'start' });
  section?.querySelector<HTMLElement>('select, input, textarea, button')?.focus({ preventScroll: true });
}

function setProvider(id: string) {
  const provider = providers.find(p => p.id === id);
  const keep = (m: string) => (provider?.models.some(x => x.id === m) ? m : '');
  config.value = { ...config.value, providerId: id, providerMissing: false, model: keep(config.value.model), helperModel: keep(config.value.helperModel) };
}

function toggleSource(id: string, on: boolean) {
  const rest = config.value.knowledgeSourceIds.filter(k => k !== id);
  config.value = { ...config.value, knowledgeSourceIds: on ? [...rest, id] : rest };
}
</script>

<template>
  <div class="flex min-w-0 flex-col">
    <nav class="sticky top-0 z-10 border-b border-base-300 bg-base-100 px-4 py-2 sm:px-2 sm:py-0 lg:px-4" :aria-label="t('configure.sections')">
      <label class="flex items-center gap-2 text-ui sm:hidden">
        <span class="shrink-0 text-base-content/70">{{ t('configure.jump_to') }}</span>
        <select class="select select-sm w-full text-ui" @change="goTo(($event.target as HTMLSelectElement).value)">
          <option v-for="s in sections" :key="s.id" :value="s.id">
            {{ s.label }}
          </option>
        </select>
      </label>
      <ul class="menu menu-horizontal menu-sm w-full flex-nowrap overflow-x-auto max-sm:hidden">
        <li v-for="s in sections" :key="s.id">
          <a :href="`#${s.id}`" class="whitespace-nowrap">{{ s.label }}</a>
        </li>
      </ul>
    </nav>

    <form class="flex min-w-0 flex-col gap-4 p-4 lg:p-6" @submit.prevent>
      <fieldset class="contents" :disabled="disabled">
        <Section id="configure-instructions" :title="t('configure.instructions')" class="scroll-mt-14">
          <label class="flex flex-col gap-1">
            <span class="flex items-center justify-between">
              <span class="label text-ui text-base-content">{{ t('configure.instructions_label') }}</span>
              <span :id="ids.count" class="text-xs tabular-nums text-base-content/70">{{ config.instructions.length }} / 32000</span>
            </span>
            <textarea v-model="config.instructions" maxlength="32000" class="textarea h-40 w-full text-ui leading-relaxed" :aria-describedby="ids.count" />
          </label>
        </Section>

        <Section id="configure-model" :title="t('configure.model')" :description="t('configure.model_hint')" class="scroll-mt-14">
          <p v-if="legacyModel" class="text-ui text-base-content/70">
            {{ t('legacy.was_model', { model: legacyModel }) }}
          </p>
          <div v-if="config.providerMissing" role="alert" class="alert alert-warning alert-soft text-base-content text-ui">
            <Icon name="lucide:triangle-alert" size="16" class="shrink-0" aria-hidden="true" />
            <span>
              {{ t('configure.provider_missing') }}
              <NuxtLink :to="links.providers" class="link">{{ t('providers.title') }}</NuxtLink>
            </span>
          </div>
          <p v-if="!providers.length" class="text-ui text-base-content/70">
            {{ t('configure.no_providers') }}
            <NuxtLink :to="links.providers" class="link">
              {{ t('setup.step_provider') }}
            </NuxtLink>
          </p>
          <div class="grid gap-4 sm:grid-cols-2">
            <div class="flex flex-col gap-1">
              <label class="flex flex-col gap-1">
                <span class="label text-ui text-base-content">{{ t('configure.provider') }}</span>
                <select
                  class="select select-sm w-full text-ui" data-field="model" :class="{ 'select-error': serverError('provider') }" :value="config.providerId"
                  :aria-invalid="!!serverError('provider')" :aria-describedby="describedBy(!!serverError('provider') && `${ids.provider}-err`)"
                  @change="setProvider(($event.target as HTMLSelectElement).value)"
                >
                  <option value="" disabled>
                    {{ t('configure.choose_provider') }}
                  </option>
                  <option v-for="p in providers" :key="p.id" :value="p.id">
                    {{ p.name }}
                  </option>
                </select>
              </label>
              <FieldError :id="`${ids.provider}-err`" :message="serverError('provider')" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="flex flex-col gap-1">
                <span class="label text-ui text-base-content">{{ t('configure.model') }}</span>
                <select
                  v-model="config.model" class="select select-sm w-full font-mono text-xs" :class="{ 'select-error': serverError('model') }" :disabled="!chosen.provider"
                  :aria-invalid="!!serverError('model')" :aria-describedby="describedBy(`${ids.model}-hint`, !!serverError('model') && `${ids.model}-err`)"
                >
                  <option value="" disabled>
                    {{ t('configure.choose_model') }}
                  </option>
                  <option v-for="m in chatModels" :key="m.id" :value="m.id">
                    {{ modelLabel(m) }}
                  </option>
                </select>
              </label>
              <p :id="`${ids.model}-hint`" class="text-xs text-base-content/70">
                {{ priceLine }}
              </p>
              <FieldError :id="`${ids.model}-err`" :message="serverError('model')" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="flex flex-col gap-1">
                <span class="label text-ui text-base-content">{{ t('configure.helper_model') }}</span>
                <select
                  v-model="config.helperModel" data-field="helper" class="select select-sm w-full font-mono text-xs" :class="{ 'select-error': helperError }" :disabled="!chosen.provider"
                  :aria-invalid="!!helperError" :aria-describedby="describedBy(`${ids.helper}-hint`, !!helperError && `${ids.helper}-err`)"
                >
                  <option value="">
                    {{ t('configure.helper_none') }}
                  </option>
                  <option v-for="m in chatModels" :key="m.id" :value="m.id">
                    {{ modelLabel(m) }}
                  </option>
                </select>
              </label>
              <p :id="`${ids.helper}-hint`" class="text-xs text-base-content/70">
                {{ t('configure.helper_hint') }}
              </p>
              <FieldError :id="`${ids.helper}-err`" :message="helperError" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="flex flex-col gap-1">
                <span class="flex items-center justify-between">
                  <span class="label text-ui text-base-content">{{ t('configure.temperature') }}</span>
                  <span class="text-xs tabular-nums" aria-hidden="true">{{ temperatureText }}</span>
                </span>
                <input
                  v-model.number="config.temperature" type="range" min="0" max="2" step="0.1" class="range range-xs w-full" :disabled="temperatureOff"
                  :aria-valuetext="temperatureText" :aria-describedby="`${ids.temperature}-hint`"
                >
              </label>
              <p :id="`${ids.temperature}-hint`" class="text-xs text-base-content/70">
                {{ t(temperatureOff ? 'configure.temperature_unsupported' : 'configure.temperature_hint') }}
              </p>
            </div>
          </div>
        </Section>

        <Section id="configure-tools" :title="t('grants.title')" :description="t('grants.hint')" class="scroll-mt-14">
          <McpGrants v-model="config" :servers="servers" :disabled="disabled" :servers-link="links.servers" />
          <FieldError :id="ids.tools" :message="serverError('server', 'tool_name', 'legacy_tools')" />
          <div v-if="config.tools.length" class="flex flex-col gap-2 rounded-box border border-base-300 px-4 py-3">
            <h3 class="text-ui font-semibold">
              {{ t('legacy.old_tools') }}
            </h3>
            <p class="text-xs text-base-content/70">
              {{ t('legacy.old_tools_hint') }}
            </p>
            <ul class="flex flex-wrap gap-2">
              <li v-for="tool in config.tools" :key="tool.name" class="badge badge-sm badge-ghost font-mono">
                {{ tool.name }}
              </li>
            </ul>
            <div>
              <AppButton variant="plain" size="xs" @click="config = { ...config, tools: [] }">
                {{ t('legacy.remove_old_tools') }}
              </AppButton>
            </div>
          </div>
        </Section>

        <Section id="configure-web" :title="t('webtools.title')" :description="t('configure.preview_section')" class="scroll-mt-14">
          <div class="flex flex-col gap-2">
            <label class="label cursor-pointer gap-2 text-ui text-base-content">
              <input v-model="runtime.webFetch" type="checkbox" class="toggle toggle-sm">
              <span><span class="font-mono text-xs">web_fetch</span> · {{ t('webtools.fetch') }}</span>
            </label>
            <div v-if="runtime.webFetch" class="flex flex-col gap-1 ps-12">
              <label class="flex flex-col gap-1">
                <span class="label text-ui text-base-content">{{ t('webtools.allowed_domains') }}<span v-if="allowListRequired" class="text-base-content/70"> ({{ t('validation.required') }})</span></span>
                <textarea
                  v-model="domainsText" data-field="web" rows="3" class="textarea textarea-sm w-full font-mono text-ui" :class="{ 'textarea-error': allowListError }" placeholder="bca.co.id"
                  :aria-invalid="!!allowListError" :aria-describedby="describedBy(`${ids.domains}-hint`, !!allowListError && `${ids.domains}-err`)"
                />
              </label>
              <p :id="`${ids.domains}-hint`" class="text-xs text-base-content/70">
                {{ allowListRequired ? t('webtools.allowlist_why') : t('webtools.allowlist_hint') }}
              </p>
              <FieldError :id="`${ids.domains}-err`" :message="allowListError" />
            </div>
          </div>
          <div class="flex flex-col gap-1">
            <label class="label cursor-pointer gap-2 text-ui text-base-content">
              <input v-model="runtime.webSearch" type="checkbox" class="toggle toggle-sm" :disabled="!searchReady">
              <span><span class="font-mono text-xs">web_search</span> · {{ t('webtools.search') }}</span>
            </label>
            <p v-if="!searchReady" class="ps-12 text-xs text-base-content/70">
              {{ t('webtools.search_needs_provider') }}
              <NuxtLink :to="links.webSearch" class="link">
                {{ t('webtools.search_link') }}
              </NuxtLink>
            </p>
          </div>
        </Section>

        <Section id="configure-memory" :title="t('configure.memory_knowledge')" class="scroll-mt-14">
          <div class="flex flex-col gap-1">
            <label class="label cursor-pointer gap-2 text-ui text-base-content">
              <input v-model="runtime.memory" type="checkbox" class="toggle toggle-sm">
              {{ t('configure.memory') }}
            </label>
            <p class="ps-12 text-xs text-base-content/70">
              {{ t('configure.memory_hint') }} {{ t('configure.preview_section') }}
            </p>
          </div>
          <fieldset class="flex flex-col gap-2">
            <legend class="text-ui font-semibold">
              {{ t('configure.knowledge') }}
            </legend>
            <p class="text-xs text-base-content/70">
              {{ t('configure.knowledge_hint') }}
              <NuxtLink :to="links.knowledge" class="link">
                {{ t('configure.knowledge_manage') }}
              </NuxtLink>
            </p>
            <p v-if="config.knowledgeSourcesMissing" role="note" class="flex items-center gap-1.5 text-xs">
              <Icon name="lucide:triangle-alert" size="14" class="shrink-0 text-warning" aria-hidden="true" />
              {{ t('configure.knowledge_missing', { n: config.knowledgeSourcesMissing }, config.knowledgeSourcesMissing) }}
            </p>
            <p v-if="!embeddings" class="text-xs text-base-content/70">
              {{ t('configure.knowledge_needs_embedding') }}
              <NuxtLink :to="links.knowledge" class="link">
                {{ t('knowledge.title') }}
              </NuxtLink>
            </p>
            <p v-else-if="!sources.length" class="text-xs text-base-content/70">
              {{ t('configure.knowledge_empty') }}
              <NuxtLink :to="links.knowledge" class="link">
                {{ t('configure.knowledge_link') }}
              </NuxtLink>
            </p>
            <ul v-else class="flex flex-col gap-2">
              <li v-for="s in sources" :key="s.id" class="flex flex-wrap items-center gap-x-3 gap-y-1 text-ui">
                <label class="flex items-center gap-2" :class="{ 'text-base-content/70': !bindable(s) }">
                  <input
                    type="checkbox" class="checkbox checkbox-sm rounded-sm" :checked="config.knowledgeSourceIds.includes(s.id)" :disabled="!config.knowledgeSourceIds.includes(s.id) && (!embeddings || !bindable(s))"
                    @change="toggleSource(s.id, ($event.target as HTMLInputElement).checked)"
                  >
                  {{ s.name }}
                </label>
                <span v-if="bindable(s)" class="text-xs text-base-content/70">{{ t('configure.knowledge_ready_docs', { n: s.ready }, s.ready) }}</span>
                <StatusLabel v-else :tone="sourceState(s).tone" :label="t(sourceState(s).key)" />
              </li>
            </ul>
            <KnowledgeTuningForm
              v-if="retrieval" :defaults="retrieval" :off="config.knowledgeSourceIds.length === 0"
              :model-value="config.knowledgeTuning" @update:model-value="config = { ...config, knowledgeTuning: $event }"
            />
          </fieldset>
        </Section>

        <Section id="configure-limits" :title="t('limits.title')" class="scroll-mt-14">
          <div class="grid gap-4 sm:grid-cols-2">
            <div class="flex flex-col gap-1">
              <label class="flex flex-col gap-1">
                <span class="label text-ui text-base-content">{{ t('configure.max_steps') }}</span>
                <input v-model.number="config.maxSteps" type="number" min="0" max="100" class="input input-sm w-full text-ui tabular-nums" :aria-describedby="`${ids.steps}-hint`">
              </label>
              <p :id="`${ids.steps}-hint`" class="text-xs text-base-content/70">
                {{ t('configure.max_steps_hint') }}
              </p>
            </div>
            <div class="flex flex-col gap-1">
              <label class="flex flex-col gap-1">
                <span class="label text-ui text-base-content">{{ t('limits.max_cost') }}</span>
                <span class="input input-sm w-full text-ui tabular-nums" :class="{ 'input-error': costError }">
                  <span aria-hidden="true">$</span>
                  <input
                    v-model="costText" type="text" inputmode="decimal" data-field="limits" class="grow" placeholder="0.00"
                    :aria-invalid="!!costError" :aria-describedby="describedBy(`${ids.cost}-hint`, !!costError && `${ids.cost}-err`)"
                  >
                </span>
              </label>
              <p :id="`${ids.cost}-hint`" class="text-xs text-base-content/70">
                {{ t('limits.max_cost_hint') }}
              </p>
              <FieldError :id="`${ids.cost}-err`" :message="costError" />
            </div>
          </div>
        </Section>
      </fieldset>
    </form>
  </div>
</template>
