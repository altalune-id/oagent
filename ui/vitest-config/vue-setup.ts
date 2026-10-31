import { config, enableAutoUnmount, RouterLinkStub } from '@vue/test-utils';
import { afterEach } from 'vitest';
import { defineComponent, h } from 'vue';

const Icon = defineComponent({
  name: 'Icon',
  props: { name: { type: String, required: true } },
  setup: props => () => h('i', { 'data-icon': props.name, 'aria-hidden': 'true' }),
});

config.global.stubs = {
  ...config.global.stubs,
  Icon,
  NuxtLink: RouterLinkStub,
};

enableAutoUnmount(afterEach);
