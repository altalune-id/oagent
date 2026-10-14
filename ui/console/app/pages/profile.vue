<script setup lang="ts">
import ProfileName from '~/components/profile/ProfileName.vue';
import ProfilePassword from '~/components/profile/ProfilePassword.vue';
import ProfileSessions from '~/components/profile/ProfileSessions.vue';
import { useProfile } from '~/components/profile/useProfile';
import Avatar from '~/components/ui/Avatar.vue';

const { t } = useI18n();
const profile = useProfile();
const {
  busy,
  name,
  nameNotice,
  nameError,
  nameFieldErrors,
  currentPassword,
  newPassword,
  passwordNotice,
  passwordError,
  passwordFieldErrors,
  confirmingSignOut,
  signOutError,
  isPerson,
  person,
} = profile;

const nameLabels = computed(() => ({ title: t('profile.name_title'), name: t('common.name'), hint: t('profile.name_hint'), save: t('common.save') }));
const passwordLabels = computed(() => ({
  title: t('profile.password_title'),
  current: t('profile.current_password'),
  next: t('profile.new_password'),
  rules: t('profile.password_rules'),
  hint: t('profile.password_hint'),
  change: t('profile.change_password'),
}));
const sessionLabels = computed(() => ({
  title: t('profile.sessions_title'),
  body: t('profile.sessions_body'),
  signOut: t('profile.sign_out_everywhere'),
  confirmTitle: t('profile.sign_out_confirm_title'),
  confirmBody: t('profile.sign_out_confirm_body'),
  cancel: t('common.cancel'),
}));

await profile.load();

useHead({ title: () => `${t('profile.title')} · oagent` });
</script>

<template>
  <div class="mx-auto w-full max-w-2xl space-y-6 px-4 py-8 sm:px-6 sm:py-10">
    <div class="flex items-center gap-4">
      <Avatar v-if="person" :id="person.id" :name="person.name" :email="person.email" :avatar-url="person.avatarUrl" size="lg" />
      <div class="min-w-0">
        <h1 class="text-2xl font-bold">
          {{ t('profile.title') }}
        </h1>
        <p v-if="person" class="truncate text-sm text-base-content/60">
          {{ person.email }}
        </p>
      </div>
    </div>
    <p v-if="!isPerson" class="text-sm text-base-content/70">
      {{ t('profile.not_a_person') }}
    </p>
    <template v-else>
      <ProfileName v-model:name="name" :busy="busy" :notice="nameNotice ? t(nameNotice) : ''" :error="nameError" :field-error="nameFieldErrors.name" :labels="nameLabels" @save="profile.saveName" />
      <ProfilePassword
        v-model:current="currentPassword"
        v-model:next="newPassword"
        :email="person?.email"
        :busy="busy"
        :notice="passwordNotice ? t(passwordNotice) : ''"
        :error="passwordError"
        :field-errors="passwordFieldErrors"
        :labels="passwordLabels"
        @submit="profile.changePassword"
      />
      <ProfileSessions
        :confirming="confirmingSignOut"
        :busy="busy"
        :error="signOutError"
        :labels="sessionLabels"
        @ask="profile.askSignOut"
        @confirm="profile.signOutEverywhere"
        @cancel="profile.cancelSignOut"
      />
    </template>
  </div>
</template>
