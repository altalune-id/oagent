const now = ref(new Date());

// useNow returns one shared clock, so expiries change on screen without a reload.
export function useNow() {
  return now;
}

// tick moves the shared clock to the current time.
export function tick() {
  now.value = new Date();
}
