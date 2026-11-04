import { tick } from '~/composables/useNow';

export default defineNuxtPlugin(() => {
  setInterval(tick, 30_000);
});
