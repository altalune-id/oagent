import type { MountingOptions } from '@vue/test-utils';
import type { Component } from 'vue';
import { flushPromises, mount } from '@vue/test-utils';
import { defineComponent, h, Suspense } from 'vue';

export async function mountPage(page: Component, options: MountingOptions<unknown> & { pageProps?: Record<string, unknown> } = {}) {
  const { pageProps, ...rest } = options;
  const host = defineComponent({ render: () => h(Suspense, null, { default: () => h(page, pageProps) }) });
  const wrapper = mount(host, rest as MountingOptions<object>);
  await flushPromises();
  return wrapper;
}
