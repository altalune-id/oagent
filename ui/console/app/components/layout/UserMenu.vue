<script setup lang="ts">
import { computed, useId } from 'vue';
import AppDropdown from '#base/components/ui/AppDropdown.vue';
import Avatar from '#base/components/ui/Avatar.vue';
import { isPerson as personOf } from '~/lib/auth/person';

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
  apiKeys: string;
}

const { me, logoutUrl, profileUrl = '/profile', apiKeysUrl = '', placement = 'end', version = '', labels = {
  signOut: 'Sign out',
  signedInAs: 'Signed in as',
  operator: 'Operator',
  apikey: 'API key',
  basic: 'Basic auth',
  profile: 'Profile',
  apiKeys: 'My API keys',
} } = defineProps<{ me: UserMenuIdentity | null, logoutUrl: string, profileUrl?: string, apiKeysUrl?: string, placement?: 'end' | 'top', version?: string, labels?: UserMenuLabels }>();

const logoutForm = `logout-${useId()}`;

const display = computed(() => me?.name || me?.email || (me?.source === 'apikey' ? labels.apikey : ''));
const isPerson = computed(() => personOf(me));
const sourceBadge = computed(() => {
  switch (me?.source) {
    case 'apikey': return labels.apikey;
    case 'basic': return labels.basic;
    default: return '';
  }
});
</script>

<template>
  <AppDropdown v-if="me" :placement="placement" :class="{ 'w-full': placement === 'top' }" content-class="w-60" :label="labels.signedInAs" :trigger-class="placement === 'top' ? 'w-full justify-start text-ui' : 'gap-2 px-1.5'">
    <template #trigger>
      <span aria-hidden="true">
        <Avatar :id="me.userId ?? ''" :name="me.name || display" :email="me.email" :avatar-url="me.avatarUrl" :size="placement === 'top' ? 'xs' : 'sm'" />
      </span>
      <span class="truncate" :class="{ 'max-w-40 max-sm:sr-only': placement !== 'top' }">{{ display }}</span>
    </template>
    <ul class="menu menu-sm w-full p-0 text-ui">
      <li class="menu-title text-base-content/70">
        <span class="truncate">{{ me.email || display }}</span>
      </li>
      <li v-if="sourceBadge || me.operator" class="menu-title">
        <span class="flex flex-wrap gap-1">
          <span v-if="sourceBadge" class="badge badge-sm badge-outline">{{ sourceBadge }}</span>
          <span v-if="me.operator" class="badge badge-sm badge-warning">{{ labels.operator }}</span>
        </span>
      </li>
      <template v-if="isPerson">
        <li>
          <NuxtLink :to="profileUrl">
            <Icon name="lucide:user" size="16" class="shrink-0" />
            {{ labels.profile }}
          </NuxtLink>
        </li>
        <li v-if="apiKeysUrl">
          <NuxtLink :to="apiKeysUrl">
            <Icon name="lucide:key-round" size="16" class="shrink-0" />
            {{ labels.apiKeys }}
          </NuxtLink>
        </li>
        <li>
          <button type="submit" :form="logoutForm">
            <Icon name="lucide:log-out" size="16" class="shrink-0" />
            {{ labels.signOut }}
          </button>
        </li>
      </template>
    </ul>
    <p v-if="version" class="px-2 pt-1 text-xs text-base-content/70">
      OAgent v{{ version }}
    </p>
    <form v-if="isPerson" :id="logoutForm" method="post" :action="logoutUrl" hidden />
  </AppDropdown>
</template>
