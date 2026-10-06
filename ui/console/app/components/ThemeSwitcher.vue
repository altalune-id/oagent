<script setup lang="ts">
const theme = useState<'light' | 'dark'>('theme', () => 'light')

function apply(value: 'light' | 'dark') {
  theme.value = value
  if (import.meta.client) {
    document.documentElement.setAttribute('data-theme', value)
  }
}

function toggle() {
  apply(theme.value === 'light' ? 'dark' : 'light')
}

onMounted(() => apply(theme.value))
</script>

<template>
  <AppButton variant="ghost" size="sm" @click="toggle">
    {{ theme === 'light' ? 'Dark' : 'Light' }}
  </AppButton>
</template>
