<script lang="ts">
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import type { ModelEntry } from '../lib/types';
  import { buildWizardRequests, initialWizardState, validateWizardStep, type WizardState } from '../lib/wizard';

  const steps = ['基础信息', '核心创意', '写作风格', '模型配置', '自动化', '确认请求'];
  let step = 1;
  let state: WizardState = { ...initialWizardState };
  let models: ModelEntry[] = [];
  let errors: string[] = [];
  let pending = false;
  let result = '';
  let createdID = '';

  onMount(async () => {
    try {
      models = (await api.listModels()).models;

    } catch (cause) {
      errors = [cause instanceof APIClientError ? cause.message : '模型列表加载失败'];
    }
  });

  function next() {
    errors = validateWizardStep(step, state);
    if (!errors.length) step = Math.min(6, step + 1);
  }

  function previous() {
    errors = [];
    step = Math.max(1, step - 1);
  }

  async function submit() {
    errors = [1, 2, 4, 5].flatMap((value) => validateWizardStep(value, state));
    if (errors.length) return;
    pending = true;
    result = '';
    try {
      const requests = buildWizardRequests(state);
      const project = await api.createProject(requests.project);
      createdID = project.id;
      const foundation = await api.requestFoundation(project.id, requests.foundation);
      result = `项目“${project.title}”已创建；Foundation 请求 ${foundation.id} 已安全保存。尚未开始模型调用；请到 Autopilot 页面明确启动任务。`;
      step = 6;
    } catch (cause) {
      const message = cause instanceof APIClientError ? cause.message : '向导提交失败';
      errors = [createdID ? `项目已创建，但 Foundation 请求失败：${message}` : message];
    } finally {
      pending = false;
    }
  }
</script>

<div class="space-y-5">
  <div>
    <h1 class="workspace-title">新建小说</h1>
    <p class="muted mt-2">六步向导创建真实项目并保存非敏感 Foundation 请求，不伪装 Autopilot 已运行。</p>
  </div>

  <ol class="workspace-card grid gap-2 p-4 sm:grid-cols-3 xl:grid-cols-6" aria-label="创建步骤">
    {#each steps as label, index}
      <li class:badge-primary={step === index + 1} class:badge-ghost={step !== index + 1} class="badge h-auto justify-start gap-2 px-3 py-2">
        <span>{index + 1}</span><span>{label}</span>
      </li>
    {/each}
  </ol>

  {#if errors.length}
    <div class="alert alert-error" role="alert"><div>{#each errors as error}<p>{error}</p>{/each}</div></div>
  {/if}
  {#if result}
    <div class="alert alert-success" role="status"><div><p>{result}</p><a class="link mt-2 inline-block" href={`#/chapters?project=${createdID}`}>打开章节页</a><a class="link ml-4" href={`#/autopilot?project=${createdID}`}>打开 Autopilot</a></div></div>
  {/if}

  <form class="workspace-card p-6" on:submit|preventDefault={submit}>
    {#if step === 1}
      <div class="grid gap-4 md:grid-cols-2">
        <label class="form-control md:col-span-2"><span class="label-text mb-2">标题</span><input class="input input-bordered" bind:value={state.title} maxlength="200" /></label>
        <label class="form-control"><span class="label-text mb-2">题材</span><input class="input input-bordered" bind:value={state.genre} placeholder="东方玄幻 / 科幻 / 悬疑" /></label>
        <label class="form-control"><span class="label-text mb-2">语言</span><select class="select select-bordered" bind:value={state.language}><option value="zh-CN">简体中文</option><option value="zh-TW">繁体中文</option><option value="en">English</option></select></label>
        <label class="form-control"><span class="label-text mb-2">目标字数</span><input class="input input-bordered" type="number" min="1000" bind:value={state.targetWords} /></label>
        <label class="form-control"><span class="label-text mb-2">目标章节</span><input class="input input-bordered" type="number" min="1" bind:value={state.targetChapters} /></label>
        <label class="form-control md:col-span-2"><span class="label-text mb-2">每章字数</span><input class="input input-bordered" type="number" min="100" bind:value={state.wordsPerChapter} /></label>
      </div>
    {:else if step === 2}
      <label class="form-control"><span class="label-text mb-2">核心创意</span><textarea class="textarea textarea-bordered min-h-56" bind:value={state.idea} maxlength="10000" placeholder="主角、目标、阻力、独特设定与终局方向"></textarea></label>
    {:else if step === 3}
      <label class="form-control"><span class="label-text mb-2">风格要求</span><textarea class="textarea textarea-bordered min-h-56" bind:value={state.style} maxlength="4000" placeholder="视角、节奏、语言密度、对白比例、禁用模式"></textarea></label>
    {:else if step === 4}
      <div class="grid gap-4 md:grid-cols-2">
        <label class="form-control"><span class="label-text mb-2">Architect</span><select class="select select-bordered" bind:value={state.architectModel}><option value="">使用项目配置</option>{#each models as model}<option value={`${model.provider}/${model.id}`}>{model.name} · {model.provider}</option>{/each}</select></label>
        <label class="form-control"><span class="label-text mb-2">Writer</span><select class="select select-bordered" bind:value={state.writerModel}><option value="">使用项目配置</option>{#each models as model}<option value={`${model.provider}/${model.id}`}>{model.name} · {model.provider}</option>{/each}</select></label>
      </div>
      {#if models.length === 0}<p class="muted mt-4">没有可用模型元数据。请先检查 Models 页面和本地配置。</p>{/if}
    {:else if step === 5}
      <div class="grid gap-5 md:grid-cols-2">
        <fieldset class="space-y-3"><legend class="font-medium">协作模式</legend><label class="label cursor-pointer justify-start gap-3"><input class="radio radio-primary" type="radio" value="copilot" bind:group={state.automationMode} /><span>Copilot</span></label><label class="label cursor-pointer justify-start gap-3"><input class="radio radio-primary" type="radio" value="autopilot" bind:group={state.automationMode} /><span>Autopilot（创建后单独启动）</span></label></fieldset>
        <fieldset class="space-y-3"><legend class="font-medium">审阅策略</legend><label class="label cursor-pointer justify-start gap-3"><input class="radio radio-secondary" type="radio" value="every_chapter" bind:group={state.reviewPolicy} /><span>每章审阅</span></label><label class="label cursor-pointer justify-start gap-3"><input class="radio radio-secondary" type="radio" value="every_n" bind:group={state.reviewPolicy} /><span>每 N 章审阅</span></label><label class="label cursor-pointer justify-start gap-3"><input class="radio radio-secondary" type="radio" value="full_automatic" bind:group={state.reviewPolicy} /><span>全自动请求</span></label>{#if state.reviewPolicy === 'every_n'}<input class="input input-bordered w-36" type="number" min="1" max="100" bind:value={state.reviewEveryN} aria-label="审阅间隔" />{/if}</fieldset>
      </div>
    {:else}
      <div class="space-y-4">
        <h2 class="text-xl font-semibold">确认 Foundation 请求</h2>
        <div class="grid gap-3 text-sm md:grid-cols-2"><div><span class="muted">标题</span><p>{state.title || '—'}</p></div><div><span class="muted">规模</span><p>{state.targetChapters} 章 / {state.targetWords.toLocaleString()} 字</p></div><div><span class="muted">模式</span><p>{state.automationMode}</p></div><div><span class="muted">审阅</span><p>{state.reviewPolicy}</p></div></div>
        <div class="alert alert-info"><span>提交会创建真实项目并保存 Foundation 请求；随后在 Autopilot 页面启动、暂停、停止或继续；不会在提交向导时自动产生模型费用。</span></div>
      </div>
    {/if}

    <div class="mt-8 flex flex-wrap justify-between gap-3 border-t border-base-300 pt-5">
      <button class="btn" type="button" on:click={previous} disabled={step === 1 || pending}>上一步</button>
      {#if step < 6}<button class="btn btn-primary" type="button" on:click={next}>下一步</button>{:else}<button class="btn btn-primary" type="submit" disabled={pending || Boolean(result)}>{pending ? '提交中…' : '创建项目并保存请求'}</button>{/if}
    </div>
  </form>
</div>
