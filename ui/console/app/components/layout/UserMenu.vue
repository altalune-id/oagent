<script setup lang="ts">
import { computed } from 'vue';
import Avatar from '../ui/Avatar.vue';

export interface UserMenuIdentity {
  source: string;
  userId?: string;
  name?: string;
  email?: string;
  avatarUrl?: string;
  keyHint?: string;
  operator?: boolean;
}

export interface UserMenuLabels {
  signOut: string;
  signedInAs: string;
  operator: string;
  apikey: string;
  basic: string;
  profile: string;
}

const { me, logoutUrl, profileUrl = '/profile', labels = {
  signOut: 'Sign out',
  signedInAs: 'Signed in as',
  operator: 'Operator',
  apikey: 'API key',
  basic: 'Basic auth',
  profile: 'Profile',
} } = defineProps<{ me: UserMenuIdentity | null, logoutUrl: string, profileUrl?: string, labels?: UserMenuLabels }>();

const display = computed(() => me?.name || me?.email || (me?.source === 'apikey' ? labels.apikey : ''));
const isPerson = computed(() => me?.source === 'local' && !!me.userId);
const sourceBadge = computed(() => {
  switch (me?.source) {
    case 'apikey': return labels.apikey;
    case 'basic': return labels.basic;
    default: return '';
  }
});
</script>

<template>
  <div v-if="me" class="dropdown dropdown-end">
    <div tabindex="0" role="button" class="btn btn-ghost btn-sm gap-2 px-1.5" :aria-label="`${labels.signedInAs} ${display}`">
      <Avatar :id="me.userId ?? ''" :name="me.name || display" :email="me.email" :avatar-url="me.avatarUrl" size="sm" />
      <span class="max-w-40 truncate max-sm:hidden">{{ display }}</span>
    </div>
    <div tabindex="0" class="dropdown-content z-30 mt-2 w-64 rounded-box border border-base-300 bg-base-100 p-3 shadow-lg">
      <p class="text-xs text-base-content/60">
        {{ labels.signedInAs }}
      </p>
      <p class="truncate font-medium">
        {{ display }}
      </p>
      <p v-if="me.email && me.email !== display" class="truncate text-sm text-base-content/70">
        {{ me.email }}
      </p>
      <div v-if="sourceBadge || me.operator" class="mt-2 flex flex-wrap gap-1">
        <span v-if="sourceBadge" class="badge badge-sm badge-outline">{{ sourceBadge }}</span>
        <span v-if="me.operator" class="badge badge-sm badge-warning">{{ labels.operator }}</span>
      </div>
      <NuxtLink v-if="isPerson" :to="profileUrl" class="btn btn-ghost btn-sm btn-block mt-3 justify-start gap-2">
        <Icon name="lucide:user-round" size="16" />
        {{ labels.profile }}
      </NuxtLink>
      <form v-if="me.source === 'local'" method="post" :action="logoutUrl" :class="isPerson ? 'mt-1' : 'mt-3'">
        <button type="submit" class="btn btn-sm btn-block justify-start gap-2">
          <Icon name="lucide:log-out" size="16" />
          {{ labels.signOut }}
        </button>
      </form>
    </div>
  </div>
</template>
