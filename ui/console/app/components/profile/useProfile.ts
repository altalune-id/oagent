import type { AvatarPerson } from '~/components/ui/avatar'
import type { ShownError } from '~/lib/apperror/message'
import { errorCode } from '~/lib/apperror/message'
import { useErrorMessage } from '~/composables/useErrorMessage'
import { siblingURL } from '~/lib/auth/urls'
import { useAuthStore } from '~/stores/auth'

export function useProfile() {
  const auth = useAuthStore()
  const errorMessage = useErrorMessage()
  const { t } = useI18n()
  const { $authClient: client } = useNuxtApp()
  const apiUrl = useRuntimeConfig().public.apiUrl

  const busy = ref(false)
  const name = ref('')
  const nameNotice = ref('')
  const nameError = ref<ShownError | null>(null)
  const currentPassword = ref('')
  const newPassword = ref('')
  const passwordNotice = ref('')
  const passwordError = ref<ShownError | null>(null)
  const confirmingSignOut = ref(false)
  const signOutError = ref<ShownError | null>(null)

  const isPerson = computed(() => auth.me?.source === 'local' && !!auth.me.userId)
  const person = computed<AvatarPerson | null>(() => {
    const me = auth.me
    return isPerson.value && me ? { id: me.userId, name: me.name, email: me.email, avatarUrl: me.avatarUrl } : null
  })

  async function load() {
    await auth.load()
    name.value = auth.me?.name ?? ''
  }

  async function run(action: () => Promise<void>) {
    if (busy.value) {
      return
    }
    busy.value = true
    try {
      await action()
    }
    finally {
      busy.value = false
    }
  }

  function saveName() {
    return run(async () => {
      nameNotice.value = ''
      nameError.value = null
      try {
        const res = await client.updateProfile({ name: name.value.trim() })
        name.value = res.name
        if (auth.me) {
          auth.me = { ...auth.me, name: res.name }
        }
        nameNotice.value = 'profile.name_saved'
      }
      catch (e) {
        nameError.value = errorMessage(e, 'error.update_profile')
      }
    })
  }

  function changePassword() {
    return run(async () => {
      passwordNotice.value = ''
      passwordError.value = null
      try {
        await client.changePassword({ currentPassword: currentPassword.value, newPassword: newPassword.value })
        currentPassword.value = ''
        newPassword.value = ''
        passwordNotice.value = 'profile.password_changed'
      }
      catch (e) {
        const shown = errorMessage(e, 'error.change_password')
        passwordError.value = errorCode(e) === 'AUT001' ? { ...shown, message: t('profile.wrong_current_password') } : shown
      }
    })
  }

  function askSignOut() {
    signOutError.value = null
    confirmingSignOut.value = true
  }

  function cancelSignOut() {
    confirmingSignOut.value = false
  }

  function signOutEverywhere() {
    return run(async () => {
      try {
        await client.signOutEverywhere({})
      }
      catch (e) {
        signOutError.value = errorMessage(e, 'error.sign_out_everywhere')
        return
      }
      finally {
        confirmingSignOut.value = false
      }
      await navigateTo(siblingURL(apiUrl, '/login'), { external: true })
    })
  }

  return {
    busy, name, nameNotice, nameError, currentPassword, newPassword, passwordNotice, passwordError,
    confirmingSignOut, signOutError, isPerson, person,
    load, saveName, changePassword, askSignOut, cancelSignOut, signOutEverywhere,
  }
}
