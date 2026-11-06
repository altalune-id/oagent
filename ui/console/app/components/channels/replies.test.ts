// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { replyErrors, seconds } from './replies';
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
});
