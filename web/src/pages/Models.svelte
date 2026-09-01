<script lang="ts">
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import type { ModelEntry } from '../lib/types';

  let models: ModelEntry[] = [];
  let query = '';
  let loading = true;
  let error = '';

  async function load() {
    loading = true;
    error = '';
    try {
      models = (await api.listModels(query)).models;
    } catch (cause) {
      error = cause instanceof APIClientError ? cause.message : '模型列表加载失败';
    } finally {
      loading = false;
    }
  }

  onMount(load);
</script>

<div class="space-y-5">
  <div><h1 class="workspace-title">Models</h1><p class="muted mt-2">来自 Go 运行时模型注册表的真实窗口和价格元数据。</p></div>
  <form class="workspace-card flex gap-3 p-4" on:submit|preventDefault={load}><input class="input input-bordered flex-1" bind:value={query} placeholder="provider、model 或名称" aria-label="搜索模型" /><button class="btn" type="submit">筛选</button></form>
  {#if error}<div class="alert alert-error" role="alert"><span>{error}</span></div>{/if}
  {#if loading}<div class="workspace-card flex min-h-48 items-center justify-center"><span class="loading loading-spinner loading-lg"></span></div>{:else if models.length === 0}<div class="workspace-card p-10 text-center">没有匹配模型</div>{:else}<div class="workspace-card overflow-x-auto"><table class="table table-zebra"><thead><tr><th>Provider</th><th>Model</th><th>上下文</th><th>最大输出</th><th>输入 / 1M</th><th>输出 / 1M</th></tr></thead><tbody>{#each models as model}<tr><td><span class="badge badge-outline">{model.provider}</span></td><td><div class="font-medium">{model.name}</div><div class="muted">{model.id}</div></td><td>{model.context_window.toLocaleString()}</td><td>{model.max_tokens.toLocaleString()}</td><td>{model.input_cost_per_1m > 0 ? `$${model.input_cost_per_1m.toFixed(2)}` : 'unknown'}</td><td>{model.output_cost_per_1m > 0 ? `$${model.output_cost_per_1m.toFixed(2)}` : 'unknown'}</td></tr>{/each}</tbody></table></div>{/if}
</div>
