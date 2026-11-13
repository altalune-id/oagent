// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { focusWhenShown } from './focus';

describe('focusWhenShown', () => {
  let frames: FrameRequestCallback[];

  beforeEach(() => {
    frames = [];
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => frames.push(cb));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = '';
  });

  function field(focusableAfter: number) {
    const el = document.createElement('input');
    document.body.append(el);
    const real = el.focus.bind(el);
    let calls = 0;
    el.focus = vi.fn((options?: FocusOptions) => {
      calls++;
      if (calls > focusableAfter) {
        real(options);
      }
    });
    return el;
  }

  function nextFrame() {
    const pending = frames;
    frames = [];
    pending.forEach(cb => cb(0));
  }

  it('focuses a shown field at once', () => {
    const el = field(0);
    focusWhenShown(el);
    expect(document.activeElement).toBe(el);
    expect(frames).toHaveLength(0);
  });

  it('retries on each frame until a field that is still being shown takes focus', () => {
    const el = field(2);
    focusWhenShown(el);
    expect(document.activeElement).not.toBe(el);
    nextFrame();
    expect(document.activeElement).not.toBe(el);
    nextFrame();
    expect(document.activeElement).toBe(el);
    expect(frames).toHaveLength(0);
  });

  it('gives up after the frame budget', () => {
    const el = field(100);
    focusWhenShown(el, 3);
    for (let i = 0; i < 5; i++) {
      nextFrame();
    }
    expect(el.focus).toHaveBeenCalledTimes(4);
    expect(document.activeElement).not.toBe(el);
  });
});
