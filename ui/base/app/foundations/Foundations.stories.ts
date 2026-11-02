import type { Meta, StoryObj } from '@storybook/vue3-vite';

const meta = {
  title: 'Foundations',
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const sizes = [
  { name: 'text-2xl', cls: 'text-2xl font-semibold', use: 'Page title' },
  { name: 'text-xl', cls: 'text-xl font-semibold', use: 'Section title' },
  { name: 'text-base', cls: 'text-base', use: 'Body' },
  { name: 'text-sm', cls: 'text-sm', use: 'Table cells, form controls' },
  { name: 'text-ui', cls: 'text-ui', use: 'Dense UI: sidebar items, menus, badges in rows' },
  { name: 'text-xs', cls: 'text-xs', use: 'Captions, hints, meta' },
];

export const TypeScale: Story = {
  render: () => ({
    setup: () => ({ sizes }),
    template: `
      <div class="flex flex-col gap-4 p-6">
        <div v-for="s in sizes" :key="s.name" class="grid grid-cols-[8rem_minmax(0,1fr)] items-baseline gap-4 border-b border-base-300 pb-3">
          <code class="font-mono text-xs text-base-content/70">{{ s.name }}</code>
          <div>
            <p :class="s.cls">Support agent answers billing questions</p>
            <p class="text-xs text-base-content/70">{{ s.use }}</p>
          </div>
        </div>
      </div>`,
  }),
};

export const Families: Story = {
  render: () => ({
    template: `
      <div class="flex flex-col gap-6 p-6">
        <section class="flex flex-col gap-1">
          <p class="text-xs text-base-content/70">Sans: Instrument Sans</p>
          <p class="text-2xl font-semibold">Agents</p>
          <p class="text-sm">Instructions, model, temperature, tools and budget for every published version.</p>
          <p class="text-sm"><span class="font-normal">Regular</span> · <span class="font-medium">Medium</span> · <span class="font-semibold">Semibold</span> · <span class="font-bold">Bold</span></p>
        </section>
        <section class="flex flex-col gap-1">
          <p class="text-xs text-base-content/70">Mono: JetBrains Mono, for ids, hosts, models and code</p>
          <p class="font-mono text-sm">acme-prod · agt_9f2c3d4e5f6g7h8j9k0m · anthropic/claude-sonnet-5-5</p>
          <pre class="rounded-box bg-base-200 p-4 font-mono text-xs">oagent agent publish --id agt_9f2c3d4e5f6g7h8j9k0m</pre>
        </section>
      </div>`,
  }),
};

const colors = ['bg-primary', 'bg-secondary', 'bg-accent', 'bg-neutral', 'bg-info', 'bg-success', 'bg-warning', 'bg-error'];

export const Colors: Story = {
  render: () => ({
    setup: () => ({ colors }),
    template: `
      <div class="flex flex-col gap-6 p-6">
        <div class="flex flex-wrap gap-3">
          <div class="flex h-16 w-28 items-end rounded-box border border-base-300 bg-base-100 p-2 text-xs">base-100</div>
          <div class="flex h-16 w-28 items-end rounded-box bg-base-200 p-2 text-xs">base-200</div>
          <div class="flex h-16 w-28 items-end rounded-box bg-base-300 p-2 text-xs">base-300</div>
          <div class="flex h-16 w-28 items-end rounded-box bg-base-content p-2 text-xs text-base-100">base-content</div>
        </div>
        <div class="flex flex-wrap gap-3">
          <figure v-for="c in colors" :key="c" class="flex w-28 flex-col gap-1 text-xs">
            <div class="h-16 rounded-box" :class="c"></div>
            <figcaption>{{ c.slice(3) }}</figcaption>
          </figure>
        </div>
        <p class="text-sm">Body text <span class="text-base-content/70">muted text at /70</span></p>
      </div>`,
  }),
};

export const Radii: Story = {
  render: () => ({
    template: `
      <div class="flex flex-wrap items-end gap-6 p-6 text-xs">
        <div class="flex flex-col items-center gap-2"><div class="size-16 rounded-selector border border-base-300 bg-base-200"></div>selector (badge, toggle)</div>
        <div class="flex flex-col items-center gap-2"><div class="size-16 rounded-field border border-base-300 bg-base-200"></div>field (input, button)</div>
        <div class="flex flex-col items-center gap-2"><div class="size-16 rounded-box border border-base-300 bg-base-200"></div>box (card, table)</div>
      </div>`,
  }),
};
