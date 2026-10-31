import type { App } from 'vue';
import { Icon as IconifyIcon } from '@iconify/vue/offline';
import { defineComponent, h } from 'vue';

// installStandalone registers the two Nuxt globals the ui components use, Icon and NuxtLink, for an app built without Nuxt.
export function installStandalone(app: App) {
  app.component('Icon', defineComponent({
    name: 'Icon',
    props: { name: { type: String, required: true }, size: { type: [String, Number], default: 16 } },
    setup: props => () => h(IconifyIcon, { icon: props.name, width: props.size, height: props.size }),
  }));
  app.component('NuxtLink', defineComponent({
    name: 'NuxtLink',
    props: { to: { type: String, required: true } },
    setup: (props, { slots }) => () => h('a', { href: props.to }, slots.default?.()),
  }));
}
