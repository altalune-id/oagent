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
  <button class="btn btn-ghost btn-sm" type="button" @click="toggle">
    {{ theme === 'light' ? 'Dark' : 'Light' }}
  </button>
</template>
