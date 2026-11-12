import type { Meta, StoryObj } from '@storybook/vue3-vite';
import FileUpload from './FileUpload.vue';

const meta = {
  title: 'UI/FileUpload',
  component: FileUpload,
  args: {
    label: 'Choose files',
    hint: 'PDF, DOCX, Markdown or TXT. Up to 10 MiB each, 100 per source. Scanned PDFs need a text layer.',
    drop: 'or drop files here',
    queue: 'Uploads',
    states: { queued: 'Waiting', uploading: 'Uploading', done: 'Uploaded', failed: 'Failed' },
    accept: '.pdf,.docx,.md,.txt',
    maxBytes: 10 * 1024 * 1024,
    items: [],
  },
} satisfies Meta<typeof FileUpload>;

export default meta;
type Story = StoryObj<typeof meta>;

const render: Story['render'] = args => ({
  components: { FileUpload },
  setup: () => ({ args }),
  template: '<div class="max-w-3xl p-8"><FileUpload v-bind="args" /></div>',
});

const queue = [
  { key: 'a', name: 'refund-policy-2026.pdf', size: '1.2 MiB', state: 'uploading' as const, progress: 0.42 },
  { key: 'b', name: 'shipping-faq.md', size: '14 KiB', state: 'done' as const, progress: 1 },
  { key: 'c', name: 'warranty-terms.docx', size: '220 KiB', state: 'queued' as const, progress: 0 },
  { key: 'd', name: 'store-photo.png', size: '2.4 MiB', state: 'failed' as const, progress: 0, error: 'This file type isn\'t supported. Use PDF, DOCX, Markdown or TXT.' },
];

export const Empty: Story = { render };
export const Uploading: Story = { args: { items: queue }, render };
export const Settled: Story = { args: { items: [queue[1]!, queue[3]!] }, render };
export const Disabled: Story = { args: { disabled: true }, render };
export const Dragover: Story = {
  render,
  play: ({ canvasElement }) => {
    canvasElement.querySelector('[data-dropzone]')!.dispatchEvent(new Event('dragover', { cancelable: true }));
  },
};
export const At390: Story = {
  args: { items: queue },
  render: args => ({
    components: { FileUpload },
    setup: () => ({ args }),
    template: '<div class="w-[358px] p-4"><FileUpload v-bind="args" /></div>',
  }),
};
