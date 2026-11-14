import type { ReplyTexts } from './replies';
import type { ReplyEditorLabels } from './ReplyTextsEditor.vue';

export const defaultReplies: ReplyTexts = {
  failure: 'Sorry, something went wrong on our side. Please try again in a moment.',
  voice: 'Sorry, I couldn\'t understand that voice message. Please type your message instead.',
  photo: 'I could not read that photo. Please describe it in text instead.',
  approvalPrompt: 'Need your OK first:\n{preview}\nReply YES or NO.',
  approvalExpired: 'That request expired, so nothing was done.',
};

export const replyLabels: ReplyEditorLabels = {
  legend: 'Set replies',
  hint: 'What the agent sends on its own in a direct message. Groups get no set reply.',
  fields: {
    failure: { label: 'When the agent fails', hint: 'Sent after a run fails.' },
    voice: { label: 'Unclear voice note', hint: 'Sent in direct chats when a message has only voice notes and none could be transcribed, or the agent has voice notes off. Not sent in groups.' },
    photo: { label: 'Unreadable photo', hint: 'Sent when the model cannot read a photo.' },
    approvalPrompt: { label: 'Approval request', hint: 'Sent when the agent needs a person to approve a tool call. {preview} is replaced by what the agent wants to do.' },
    approvalExpired: { label: 'Approval expired', hint: 'Sent when an approval expires before anyone answers.' },
  },
  reset: 'Use the default',
  save: 'Save replies',
};
