// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import en from '~~/i18n/locales/en-US.json';
import id from '~~/i18n/locales/id-ID.json';
import { maxReplyLength, replyErrors, seconds, timingErrors, toMs } from './replies';
import ReplyTextsEditor from './ReplyTextsEditor.vue';
import { defaultReplies, replyLabels } from './stories';

describe('set replies', () => {
  it('refuses a blank reply and shows timing in seconds', () => {
    expect(replyErrors({ ...defaultReplies, voice: '  ' })).toEqual({ voice: 'channels.error_reply_required' });
    expect(replyErrors(defaultReplies)).toEqual({});
    expect(replyErrors({ ...defaultReplies, approvalPrompt: ' ', approvalExpired: 'x'.repeat(maxReplyLength + 1) })).toEqual({ approvalPrompt: 'channels.error_reply_required', approvalExpired: 'channels.error_reply_too_long' });
    expect(replyErrors({ ...defaultReplies, approvalExpired: 'é'.repeat(maxReplyLength) })).toEqual({});
    expect(replyErrors({ ...defaultReplies, approvalPrompt: 'Boleh saya lakukan ini? Balas YA.' })).toEqual({ approvalPrompt: 'channels.error_reply_preview' });
    expect(replyErrors({ ...defaultReplies, approvalExpired: 'no preview here is fine' })).toEqual({});
    expect([seconds(3_000), seconds(2_450)]).toEqual([3, 2.5]);
  });

  it('edits a reply, offers the default back, and saves', async () => {
    const w = mount(ReplyTextsEditor, { props: { 'modelValue': { ...defaultReplies, voice: 'Kirim teks saja.' }, 'defaults': defaultReplies, 'labels': replyLabels, 'errors': { photo: 'Required' }, 'onUpdate:modelValue': (v: unknown) => w.setProps({ modelValue: v as typeof defaultReplies }) } });
    expect(w.findAll('textarea')).toHaveLength(5);
    expect(w.text()).toContain('Approval request');
    expect(w.text()).toContain('{preview} is replaced by what the agent wants to do.');
    expect(w.find('textarea[aria-invalid="true"]').exists()).toBe(true);
    const reset = w.findAll('button').filter(b => b.text() === 'Use the default');
    expect(reset).toHaveLength(1);
    await reset[0]!.trigger('click');
    expect((w.findAll('textarea')[1]!.element as HTMLTextAreaElement).value).toBe(defaultReplies.voice);
    await w.get('form').trigger('submit');
    expect(w.emitted('save')).toHaveLength(1);
  });

  it('shows the replies without save or reset when read-only', () => {
    const w = mount(ReplyTextsEditor, { props: { modelValue: { ...defaultReplies, voice: 'x' }, defaults: defaultReplies, labels: replyLabels, readOnly: true } });
    expect(w.get('fieldset').attributes('disabled')).toBeDefined();
    expect(w.findAll('button')).toHaveLength(0);
  });

  it('reads typed seconds into milliseconds, one decimal at most', () => {
    expect(['3', ' 2.5 ', '0', '10', '1,5'].map(toMs)).toEqual([3_000, 2_500, 0, 10_000, 1_500]);
    expect(['', 'abc', '-1', '2.55', '3s'].map(toMs)).toEqual([null, null, null, null, null]);
  });

  it('keeps the quiet window and its maximum within the server\'s limit, the window no longer than the maximum', () => {
    const limit = 15_000;
    expect(timingErrors(3_000, 10_000, limit)).toEqual({});
    expect(timingErrors(12_000, 15_000, limit), 'the limit comes from the server').toEqual({});
    expect(timingErrors(0, 0, limit)).toEqual({});
    expect(timingErrors(null, 10_000, limit)).toEqual({ quiet: 'channels.error_quiet_range' });
    expect(timingErrors(3_000, 15_500, limit)).toEqual({ quietMax: 'channels.error_quiet_range' });
    expect(timingErrors(6_000, 4_000, limit)).toEqual({ quietMax: 'channels.error_quiet_order' });
    expect(timingErrors(16_000, null, limit)).toEqual({ quiet: 'channels.error_quiet_range', quietMax: 'channels.error_quiet_range' });
    expect(timingErrors(99_000, 99_000, 0), 'no limit from the server: no range check').toEqual({});
    expect(timingErrors(null, 5_000, 0)).toEqual({ quiet: 'channels.error_quiet_number' });
    expect(timingErrors(6_000, 4_000, 0)).toEqual({ quietMax: 'channels.error_quiet_order' });
  });

  it('tells the admin that {preview} stands for the request, in both locales, as literal text vue-i18n won\'t treat as a placeholder', () => {
    const rendered = (message: string) => message.replace(/\{'([^']*)'\}/g, '$1');
    for (const messages of [en, id] as Record<string, string>[]) {
      const hint = messages['channels.reply_approvalPrompt_hint']!;
      expect(rendered(hint)).toContain('{preview}');
      expect(hint.replace(/\{'[^']*'\}/g, '')).not.toMatch(/\{\w+\}/);
      expect(messages['channels.reply_approvalExpired_hint']).toBeTruthy();
    }
  });
});
