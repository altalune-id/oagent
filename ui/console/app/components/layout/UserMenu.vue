<script setup lang="ts">
import { computed } from 'vue'

export interface UserMenuIdentity {
  source: string
  name?: string
  email?: string
  keyHint?: string
  operator?: boolean
}

export interface UserMenuLabels {
  signOut: string
  signedInAs: string
  operator: string
  apikey: string
  basic: string
}

const { me, logoutUrl, labels = {
  signOut: 'Sign out',
  signedInAs: 'Signed in as',
  operator: 'Operator',
  apikey: 'API key',
  basic: 'Basic auth',
} } = defineProps<{ me: UserMenuIdentity | null, logoutUrl: string, labels?: UserMenuLabels }>()

const display = computed(() => me?.name || me?.email || (me?.source === 'apikey' ? labels.apikey : ''))
const initial = computed(() => (display.value || '?').charAt(0).toUpperCase())
const sourceBadge = computed(() => {
  switch (me?.source) {
    case 'apikey': return labels.apikey
    case 'basic': return labels.basic
    default: return ''
  }
})
</script>

<template>
  <div v-if="me" class="dropdown dropdown-end">
    <div tabindex="0" role="button" class="btn btn-ghost btn-sm gap-2 px-1.5" :aria-label="`${labels.signedInAs} ${display}`">
      <span class="grid size-7 place-items-center rounded-full bg-primary text-sm font-semibold text-primary-content">{{ initial }}</span>
      <span class="max-w-40 truncate max-sm:hidden">{{ display }}</span>
    </div>
    <div tabindex="0" class="dropdown-content z-30 mt-2 w-64 rounded-box border border-base-300 bg-base-100 p-3 shadow-lg">
      <p class="text-xs text-base-content/60">{{ labels.signedInAs }}</p>
      <p class="truncate font-medium">{{ display }}</p>
      <p v-if="me.email && me.email !== display" class="truncate text-sm text-base-content/70">{{ me.email }}</p>
      <div v-if="sourceBadge || me.operator" class="mt-2 flex flex-wrap gap-1">
        <span v-if="sourceBadge" class="badge badge-sm badge-outline">{{ sourceBadge }}</span>
        <span v-if="me.operator" class="badge badge-sm badge-warning">{{ labels.operator }}</span>
      </div>
      <form v-if="me.source === 'local'" method="post" :action="logoutUrl" class="mt-3">
        <button type="submit" class="btn btn-sm btn-block justify-start gap-2">
          <Icon name="lucide:log-out" size="16" />
          {{ labels.signOut }}
        </button>
      </form>
    </div>
  </div>
</template>
