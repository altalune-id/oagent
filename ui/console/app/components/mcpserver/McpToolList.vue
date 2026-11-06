<script setup lang="ts">
import type { McpTool } from '~/stores/mcpServers';
import { hintBadges, nameParts, toolName } from './tools';

const { server, tools, trusted } = defineProps<{ server: string, tools: McpTool[], trusted: boolean }>();

const { t } = useI18n();
</script>

<template>
  <ul class="list rounded-box border border-base-300">
    <li v-for="tool in tools" :key="tool.name" class="list-row flex flex-wrap items-start gap-x-2 gap-y-1 py-2">
      <div class="min-w-0 flex-1 basis-48" :class="{ 'text-base-content/70': !tool.grantable }">
        <p class="font-mono text-xs">
          <template v-for="(part, i) in nameParts(toolName(server, tool.name))" :key="i">
            {{ part }}<wbr>
          </template>
        </p>
        <p class="text-xs text-base-content/70">
          {{ tool.description }}
        </p>
      </div>
      <span v-if="tool.hint === 'destructive' && trusted" class="badge badge-sm badge-warning badge-soft text-base-content shrink-0">
        <Icon :name="hintBadges[tool.hint].icon" size="12" aria-hidden="true" />
        {{ t(hintBadges[tool.hint].labelKey) }}
      </span>
      <span v-else class="badge badge-sm badge-ghost shrink-0">
        <Icon :name="hintBadges[tool.hint].icon" size="12" aria-hidden="true" />
        {{ t(hintBadges[tool.hint].labelKey) }}<span v-if="!trusted">{{ t('mcptools.not_verified') }}</span>
      </span>
      <p v-if="!tool.grantable" class="flex basis-full items-start gap-1.5 text-xs text-base-content/70">
        <Icon name="lucide:ban" size="12" class="mt-0.5 shrink-0" aria-hidden="true" />
        <span class="min-w-0">{{ t('mcptools.not_grantable') }}</span>
      </p>
    </li>
  </ul>
</template>
