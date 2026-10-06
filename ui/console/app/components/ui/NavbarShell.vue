<template>
  <header class="navbar sticky top-0 z-40 min-h-14 gap-2 border-b border-base-300 bg-base-100/90 px-4 backdrop-blur">
    <div class="flex items-center gap-2">
      <slot name="brand" />
      <slot />
    </div>
    <div class="ml-auto flex items-center gap-1">
      <slot name="end" />
    </div>
  </header>
</template>
