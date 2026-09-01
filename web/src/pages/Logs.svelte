<script lang="ts">
  import { connectionState, recentEvents } from '../lib/events';
</script>

<div class="space-y-5">
  <div class="flex items-end justify-between gap-4"><div><h1 class="workspace-title">Logs</h1><p class="muted mt-2">来自持久化 SSE Event Repository 的实时与重放事件。</p></div><span class:badge-success={$connectionState === 'connected'} class:badge-warning={$connectionState !== 'connected'} class="badge badge-outline">{$connectionState}</span></div>
  {#if $recentEvents.length === 0}<div class="workspace-card p-10 text-center"><p class="font-medium">尚无事件</p><p class="muted mt-1">连接成功后，服务器与项目事件会出现在这里。</p></div>{:else}<div class="space-y-3">{#each $recentEvents as event}<article class="workspace-card p-4"><div class="flex flex-wrap items-center justify-between gap-3"><div class="flex items-center gap-2"><span class="badge badge-primary badge-outline">{event.type}</span>{#if event.project}<span class="muted">project {event.project}</span>{/if}</div><time class="muted">{new Date(event.time).toLocaleString()}</time></div>{#if event.data}<pre class="mt-3 max-h-48 overflow-auto rounded-xl bg-base-200 p-3 text-xs">{JSON.stringify(event.data, null, 2)}</pre>{/if}</article>{/each}</div>{/if}
</div>
