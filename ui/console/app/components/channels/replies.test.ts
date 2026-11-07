// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { replyErrors, seconds, timingErrors, toMs } from './replies';
import ReplyTextsEditor from './ReplyTextsEditor.vue';
import { defaultReplies, replyLabels } from './stories';

describe('set replies', () => {
  it('refuses a blank reply and shows timing in seconds', () => {
    expect(replyErrors({ ...defaultReplies, voice: '  ' })).toEqual({ voice: 'channels.error_reply_required' });
    expect(replyErrors(defaultReplies)).toEqual({});
    expect([seconds(3_000), seconds(2_450)]).toEqual([3, 2.5]);
  });

  it('edits a reply, offers the default back, and saves', async () => {
    const w = mount(ReplyTextsEditor, { props: { 'modelValue': { ...defaultReplies, voice: 'Kirim teks saja.' }, 'defaults': defaultReplies, 'labels': replyLabels, 'errors': { photo: 'Required' }, 'onUpdate:modelValue': (v: unknown) => w.setProps({ modelValue: v as typeof defaultReplies }) } });
    expect(w.findAll('textarea')).toHaveLength(3);
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

  it('keeps the quiet window and its maximum within 0-10 s, the window no longer than the maximum', () => {
    expect(timingErrors(3_000, 10_000)).toEqual({});
    expect(timingErrors(0, 0)).toEqual({});
    expect(timingErrors(null, 10_000)).toEqual({ quiet: 'channels.error_quiet_range' });
    expect(timingErrors(3_000, 10_500)).toEqual({ quietMax: 'channels.error_quiet_range' });
    expect(timingErrors(6_000, 4_000)).toEqual({ quietMax: 'channels.error_quiet_order' });
    expect(timingErrors(11_000, null)).toEqual({ quiet: 'channels.error_quiet_range', quietMax: 'channels.error_quiet_range' });
  });
});
