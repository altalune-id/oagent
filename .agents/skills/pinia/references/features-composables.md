---
name: composables-in-stores
description: Using Vue composables within Pinia stores
---

# Composables in Stores

Pinia stores can leverage Vue composables for reusable stateful logic.

## Option Stores

Call composables inside the `state` property, but only those returning writable refs:

```ts
import { defineStore } from 'pinia'
import { useLocalStorage } from '@vueuse/core'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: useLocalStorage('pinia/auth/login', 'bob'),
  }),
})
```

**Works:** Composables returning `ref()`:
- `useLocalStorage`
- `useAsyncState`

**Doesn't work in Option Stores:**
- Composables exposing functions
- Composables exposing readonly data

## Setup Stores

More flexible - can use almost any composable:

```ts
import { defineStore } from 'pinia'
import { useMediaControls } from '@vueuse/core'
import { ref } from 'vue'

export const useVideoPlayer = defineStore('video', () => {
  const videoElement = ref<HTMLVideoElement>()
  const src = ref('/data/video.mp4')
  const { playing, volume, currentTime, togglePictureInPicture } =
    useMediaControls(videoElement, { src })

  function loadVideo(element: HTMLVideoElement, newSrc: string) {
    videoElement.value = element
    src.value = newSrc
  }

  return {
    src,
    playing,
    volume,
    currentTime,
    loadVideo,
    togglePictureInPicture,
  }
})
```

**Note:** Don't return non-serializable DOM refs like `videoElement` - they're internal implementation details.

<!--
Source references:
- https://pinia.vuejs.org/cookbook/composables.html
-->
