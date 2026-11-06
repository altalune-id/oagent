import type { Meta, StoryObj } from '@storybook/vue3-vite';
import ReplyTextsEditor from './ReplyTextsEditor.vue';
import { defaultReplies, replyLabels } from './stories';

const meta = { title: 'Channels/ReplyTextsEditor', component: ReplyTextsEditor, args: { modelValue: defaultReplies, defaults: defaultReplies, labels: replyLabels } } satisfies Meta<typeof ReplyTextsEditor>;

export default meta;
type Story = StoryObj<typeof meta>;

const render: Story['render'] = args => ({ components: { ReplyTextsEditor }, setup: () => ({ args }), template: '<div class="max-w-2xl p-8"><ReplyTextsEditor v-bind="args" /></div>' });

export const Defaults: Story = { render };

export const Edited: Story = { args: { modelValue: { ...defaultReplies, voice: 'Maaf, pesan suara belum didukung. Kirim lewat teks ya.' } }, render };

export const EmptyReply: Story = { args: { modelValue: { ...defaultReplies, photo: '' }, errors: { photo: 'Write a reply; the agent would otherwise send nothing' } }, render };

export const ReadOnly: Story = { args: { readOnly: true }, render };
