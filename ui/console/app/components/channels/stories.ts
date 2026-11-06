import type { ReplyTexts } from './replies';
import type { ReplyEditorLabels } from './ReplyTextsEditor.vue';

export const defaultReplies: ReplyTexts = {
  failure: 'Sorry, something went wrong on our side. Please try again in a moment.',
  voice: 'Voice notes are not supported yet. Please send your message as text.',
  photo: 'I could not read that photo. Please describe it in text instead.',
};

export const replyLabels: ReplyEditorLabels = {
  legend: 'Set replies',
  hint: 'What the agent sends on its own in a direct message. Groups get no set reply.',
  fields: {
    failure: { label: 'When the agent fails', hint: 'Sent after a run fails.' },
    voice: { label: 'Voice notes', hint: 'Sent when someone sends a voice note.' },
    photo: { label: 'Unreadable photo', hint: 'Sent when the model cannot read a photo.' },
  },
  reset: 'Use the default',
  save: 'Save replies',
};
