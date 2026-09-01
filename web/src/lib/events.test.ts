import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';

class FakeEventSource {
  static latest: FakeEventSource | undefined;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  listeners = new Map<string, (event: MessageEvent<string>) => void>();
  closed = false;

  constructor(readonly url: string) {
    FakeEventSource.latest = this;
  }

  addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    this.listeners.set(type, listener as (event: MessageEvent<string>) => void);
  }

  close() {
    this.closed = true;
  }
}

describe('SSE event hub', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
    FakeEventSource.latest = undefined;
  });

  it('reports reconnecting and accepts named durable events', async () => {
    vi.stubGlobal('EventSource', FakeEventSource);
    const module = await import('./events');
    const stop = module.startEventStream();
    FakeEventSource.latest?.onopen?.();
    expect(get(module.connectionState)).toBe('connected');
    FakeEventSource.latest?.onerror?.();
    expect(get(module.connectionState)).toBe('reconnecting');
    FakeEventSource.latest?.listeners.get('project.created')?.(new MessageEvent('project.created', {
      data: JSON.stringify({ id: 4, type: 'project.created', time: '2026-09-01T00:00:00Z' })
    }));
    expect(get(module.recentEvents)[0].type).toBe('project.created');
    stop();
    expect(FakeEventSource.latest?.closed).toBe(true);
  });
});
