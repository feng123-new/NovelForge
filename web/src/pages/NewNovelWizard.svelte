<script lang="ts">
  import { t, text, label as stateLabel, locale, message as uiMessage, errorMessage } from '../lib/i18n';
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import type { ModelEntry } from '../lib/types';
  import { buildWizardRequests, initialWizardState, validateWizardStep, type WizardState } from '../lib/wizard';

  const steps = [uiMessage("ui_15a81b9294c7"), uiMessage("ui_e9e6ee1c77a8"), uiMessage("ui_baf17c21dcda"), uiMessage("ui_8cbcf741e727"), uiMessage("ui_829a35554fa1"), uiMessage("ui_3804625bf391")];
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
      errors = [cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_91f378ea3006")];
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
      result = uiMessage("ui_8053ac480230", {p0: project.title, p1: foundation.id});
      step = 6;
    } catch (cause) {
      const message = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_86b4be6ce9d3");
      errors = createdID ? [uiMessage("ui_b5f2c06f7f4d"), message] : [message];
    } finally {
      pending = false;
    }
  }
</script>

<div class="space-y-5">
  <div>
    <h1 class="workspace-title">{$t("ui_18f5aa4a51c6")}</h1>
    <p class="muted mt-2">{$t("ui_92f42c383941")}</p>
  </div>

  <ol class="workspace-card grid gap-2 p-4 sm:grid-cols-3 xl:grid-cols-6" aria-label={$t("ui_c1ec25110e83")}>
    {#each steps as label, index}
      <li class:badge-primary={step === index + 1} class:badge-ghost={step !== index + 1} class="badge h-auto justify-start gap-2 px-3 py-2">
        <span>{index + 1}</span><span>{$text(label)}</span>
      </li>
    {/each}
  </ol>

  {#if errors.length}
    <div class="alert alert-error" role="alert"><div>{#each errors as error}<p>{$text(error)}</p>{/each}</div></div>
  {/if}
  {#if result}
    <div class="alert alert-success" role="status"><div><p>{$text(result)}</p><a class="link mt-2 inline-block" href={`#/chapters?project=${createdID}`}>{$t("ui_2a7d71e65ede")}</a><a class="link ml-4" href={`#/autopilot?project=${createdID}`}>{$t("ui_e9a1e145018c")}</a></div></div>
  {/if}

  <form class="workspace-card p-6" on:submit|preventDefault={submit}>
    {#if step === 1}
      <div class="grid gap-4 md:grid-cols-2">
        <label class="form-control md:col-span-2"><span class="label-text mb-2">{$t("ui_c3405f8c7d9d")}</span><input class="input input-bordered" bind:value={state.title} maxlength="200" /></label>
        <label class="form-control"><span class="label-text mb-2">{$t("ui_ecf11c156b87")}</span><input class="input input-bordered" bind:value={state.genre} placeholder={$t("ui_526ae3e93bc1")} /></label>
        <label class="form-control"><span class="label-text mb-2">{$t("ui_9f6fee1aba17")}</span><select class="select select-bordered" bind:value={state.language}><option value="zh-CN">{$t("ui_9937e365aaf3")}</option><option value="zh-TW">{$t("ui_7bf7a60617de")}</option><option value="en">{$t("ui_ba118bf7fc9c")}</option></select></label>
        <label class="form-control"><span class="label-text mb-2">{$t("ui_090d73ec7285")}</span><input class="input input-bordered" type="number" min="1000" bind:value={state.targetWords} /></label>
        <label class="form-control"><span class="label-text mb-2">{$t("ui_fa1915720634")}</span><input class="input input-bordered" type="number" min="1" bind:value={state.targetChapters} /></label>
        <label class="form-control md:col-span-2"><span class="label-text mb-2">{$t("ui_8515676f077d")}</span><input class="input input-bordered" type="number" min="100" bind:value={state.wordsPerChapter} /></label>
      </div>
    {:else if step === 2}
      <label class="form-control"><span class="label-text mb-2">{$t("ui_e9e6ee1c77a8")}</span><textarea class="textarea textarea-bordered min-h-56" bind:value={state.idea} maxlength="10000" placeholder={$t("ui_9a2235861d45")}></textarea></label>
    {:else if step === 3}
      <label class="form-control"><span class="label-text mb-2">{$t("ui_373fa2c8afd2")}</span><textarea class="textarea textarea-bordered min-h-56" bind:value={state.style} maxlength="4000" placeholder={$t("ui_4783608415b1")}></textarea></label>
    {:else if step === 4}
      <div class="grid gap-4 md:grid-cols-2">
        <label class="form-control"><span class="label-text mb-2">{$t("ui_861eea32ae99")}</span><select class="select select-bordered" bind:value={state.architectModel}><option value="">{$t("ui_d450aae04ed0")}</option>{#each models as model}<option value={`${model.provider}/${model.id}`}>{model.name} · {model.provider}</option>{/each}</select></label>
        <label class="form-control"><span class="label-text mb-2">{$t("ui_0e3c5fe00f66")}</span><select class="select select-bordered" bind:value={state.writerModel}><option value="">{$t("ui_d450aae04ed0")}</option>{#each models as model}<option value={`${model.provider}/${model.id}`}>{model.name} · {model.provider}</option>{/each}</select></label>
      </div>
      {#if models.length === 0}<p class="muted mt-4">{$t("ui_5e4424ad3cd6")}</p>{/if}
    {:else if step === 5}
      <div class="grid gap-5 md:grid-cols-2">
        <fieldset class="space-y-3"><legend class="font-medium">{$t("ui_4321cd73905d")}</legend><label class="label cursor-pointer justify-start gap-3"><input class="radio radio-primary" type="radio" value="copilot" bind:group={state.automationMode} /><span>{$t("ui_49f5b0e488cf")}</span></label><label class="label cursor-pointer justify-start gap-3"><input class="radio radio-primary" type="radio" value="autopilot" bind:group={state.automationMode} /><span>{$t("ui_a32e7637ecae")}</span></label></fieldset>
        <fieldset class="space-y-3"><legend class="font-medium">{$t("ui_5fe2b9906892")}</legend><label class="label cursor-pointer justify-start gap-3"><input class="radio radio-secondary" type="radio" value="every_chapter" bind:group={state.reviewPolicy} /><span>{$t("ui_998db3ba6174")}</span></label><label class="label cursor-pointer justify-start gap-3"><input class="radio radio-secondary" type="radio" value="every_n" bind:group={state.reviewPolicy} /><span>{$t("ui_d2e57e40cc9b")}</span></label><label class="label cursor-pointer justify-start gap-3"><input class="radio radio-secondary" type="radio" value="full_automatic" bind:group={state.reviewPolicy} /><span>{$t("ui_7f41e6707b2d")}</span></label>{#if state.reviewPolicy === 'every_n'}<input class="input input-bordered w-36" type="number" min="1" max="100" bind:value={state.reviewEveryN} aria-label={$t("ui_1e60a555cb8a")} />{/if}</fieldset>
      </div>
    {:else}
      <div class="space-y-4">
        <h2 class="text-xl font-semibold">{$t("ui_af09c9abe186")}</h2>
        <div class="grid gap-3 text-sm md:grid-cols-2"><div><span class="muted">{$t("ui_c3405f8c7d9d")}</span><p>{(state.title || '—')}</p></div><div><span class="muted">{$t("ui_3dbf2e1f70fe")}</span><p>{$t("ui_a2691b9e417e", {p0: state.targetChapters, p1: state.targetWords.toLocaleString($locale)})}</p></div><div><span class="muted">{$t("ui_47a270081ab2")}</span><p>{$stateLabel(state.automationMode)}</p></div><div><span class="muted">{$t("ui_8deff6662fea")}</span><p>{$stateLabel(state.reviewPolicy)}</p></div></div>
        <div class="alert alert-info"><span>{$t("ui_95a768171ebf")}</span></div>
      </div>
    {/if}

    <div class="mt-8 flex flex-wrap justify-between gap-3 border-t border-base-300 pt-5">
      <button class="btn" type="button" on:click={previous} disabled={step === 1 || pending}>{$t("ui_da336fdc0dbd")}</button>
      {#if step < 6}<button class="btn btn-primary" type="button" on:click={next}>{$t("ui_acfc4e74a650")}</button>{:else}<button class="btn btn-primary" type="submit" disabled={pending || Boolean(result)}>{(pending ? $t("ui_18931020ad52") : $t("ui_bf4d7bee3206"))}</button>{/if}
    </div>
  </form>
</div>
