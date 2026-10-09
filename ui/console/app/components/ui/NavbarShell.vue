<template>
  <header class="navbar sticky top-0 z-50 min-h-14 gap-2 border-b border-base-300 bg-base-100/90 px-2 backdrop-blur sm:px-4">
    <div class="flex min-w-0 flex-1 items-center gap-2">
      <slot name="brand" />
      <slot />
    </div>
    <div class="flex shrink-0 items-center gap-1">
      <slot name="end" />
    </div>
  </header>
</template>
