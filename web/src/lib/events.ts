import { writable } from 'svelte/store';
import type { WorkspaceEvent } from './types';

export type ConnectionState = 'connecting' | 'connected' | 'reconnecting' | 'unavailable';

export const connectionState = writable<ConnectionState>('connecting');
export const recentEvents = writable<WorkspaceEvent[]>([]);

const eventTypes = [
  'connected',
  'heartbeat',
  'server.ready',
  'project.created',
  'project.updated',
  'project.archived',
  'project.unarchived',
  'project.duplicated',
  'audit.project.deleted',
  'foundation.requested',
  'autopilot.changed',
  'replay.truncated',
  'stream.error'
];

let source: EventSource | undefined;
let listeners = 0;

export function startEventStream(project = ''): () => void {
  listeners += 1;
  if (!source && typeof EventSource !== 'undefined') {
    const params = new URLSearchParams();
    if (project) params.set('project', project);
    const query = params.toString();
    source = new EventSource(`/api/events${query ? `?${query}` : ''}`);
    connectionState.set('connecting');
    source.onopen = () => connectionState.set('connected');
    source.onerror = () => connectionState.set('reconnecting');
    for (const type of eventTypes) {
      source.addEventListener(type, receiveEvent);
    }
  } else if (!source) {
    connectionState.set('unavailable');
  }
  let stopped = false;
  return () => {
    if (stopped) return;
    stopped = true;
    listeners -= 1;
    if (listeners <= 0 && source) {
      source.close();
      source = undefined;
      listeners = 0;
      connectionState.set('connecting');
    }
  };
}

function receiveEvent(message: MessageEvent<string>): void {
  try {
    const parsed = JSON.parse(message.data) as WorkspaceEvent;
    recentEvents.update((events) => [parsed, ...events].slice(0, 100));
  } catch {
    recentEvents.update((events) => [
      {
        type: 'client.decode_error',
        time: new Date().toISOString(),
        data: { retryable: false }
      },
      ...events
    ].slice(0, 100));
  }
}

export function resetEventStateForTest(): void {
  source?.close();
  source = undefined;
  listeners = 0;
  recentEvents.set([]);
  connectionState.set('connecting');
}
