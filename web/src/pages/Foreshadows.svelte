<script lang="ts">
  import {
    compareForeshadows,
    newIdempotencyKey,
    type Foreshadow,
    type Page,
  } from '../lib/ledger'

  export let projectId: string

  let items: Foreshadow[] = []
  let loading = false
  let saving = false
  let error = ''
  let loadedProject = ''
  let chapter = 1
  let key = ''
  let title = ''
  let description = ''
  let priority: 'critical' | 'high' | 'normal' | 'low' = 'normal'
  let dueChapter = 1

  $: if (projectId && projectId !== loadedProject) {
    loadedProject = projectId
    void reload()
  }

  async function api<T>(path: string, init?: RequestInit): Promise<T> {
    const response = await fetch(path, init)
    const payload = await response.json().catch(() => ({}))
    if (!response.ok) {
      throw new Error(payload?.error?.message ?? `Request failed (${response.status})`)
    }
    return payload as T
  }

  async function reload(): Promise<void> {
    if (!projectId) return
    loading = true
    error = ''
    try {
      const page = await api<Page<Foreshadow>>(
        `/api/projects/${encodeURIComponent(projectId)}/foreshadows?chapter=${chapter}&limit=100`,
      )
      items = [...page.items].sort(compareForeshadows)
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Unable to load foreshadows'
    } finally {
      loading = false
    }
  }

  async function createForeshadow(): Promise<void> {
    if (!key.trim() || !title.trim()) {
      error = 'Key and title are required.'
      return
    }
    saving = true
    error = ''
    try {
      await api(`/api/projects/${encodeURIComponent(projectId)}/foreshadows`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': newIdempotencyKey('foreshadow-create'),
        },
        body: JSON.stringify({
          chapter,
          action: 'create',
          key,
          title,
          description,
          priority,
          due_chapter: dueChapter,
        }),
      })
      key = ''
      title = ''
      description = ''
      await reload()
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Unable to create foreshadow'
    } finally {
      saving = false
    }
  }

  async function transition(item: Foreshadow, action: 'plant' | 'reinforce' | 'reveal' | 'abandon'): Promise<void> {
    saving = true
    error = ''
    try {
      await api(
        `/api/projects/${encodeURIComponent(projectId)}/foreshadows/${encodeURIComponent(item.key)}`,
        {
          method: 'PATCH',
          headers: {
            'Content-Type': 'application/json',
            'Idempotency-Key': newIdempotencyKey(`foreshadow-${action}`),
          },
          body: JSON.stringify({chapter, action}),
        },
      )
      await reload()
    } catch (cause) {
      error = cause instanceof Error ? cause.message : `Unable to ${action} foreshadow`
    } finally {
      saving = false
    }
  }
</script>

<section class="ledger-panel" aria-labelledby="foreshadow-title">
  <header>
    <div>
      <p class="eyebrow">Narrative Ledger</p>
      <h2 id="foreshadow-title">Foreshadows</h2>
    </div>
    <label>
      Chapter N
      <input type="number" min="0" bind:value={chapter} on:change={reload} />
    </label>
  </header>

  <form on:submit|preventDefault={createForeshadow}>
    <input aria-label="Foreshadow key" placeholder="stable-key" bind:value={key} />
    <input aria-label="Foreshadow title" placeholder="Foreshadow title" bind:value={title} />
    <textarea aria-label="Foreshadow description" placeholder="What must be paid off?" bind:value={description}></textarea>
    <select aria-label="Foreshadow priority" bind:value={priority}>
      <option value="critical">Critical</option>
      <option value="high">High</option>
      <option value="normal">Normal</option>
      <option value="low">Low</option>
    </select>
    <label>
      Due chapter
      <input type="number" min="0" bind:value={dueChapter} />
    </label>
    <button type="submit" disabled={saving}>Add foreshadow</button>
  </form>

  {#if error}<p class="error" role="alert">{error}</p>{/if}
  {#if loading}
    <p>Loading authoritative ledger…</p>
  {:else if items.length === 0}
    <p class="empty">No foreshadows at Chapter {chapter}.</p>
  {:else}
    <div class="cards">
      {#each items as item (item.id)}
        <article class:overdue={item.effective_status === 'overdue'}>
          <div class="badges">
            <strong>{item.effective_status.toUpperCase()}</strong>
            <span>{item.priority}</span>
            {#if item.due_chapter !== null && item.due_chapter !== undefined}
              <span>due {item.due_chapter}</span>
            {/if}
          </div>
          <h3>{item.title}</h3>
          <code>{item.key}</code>
          {#if item.description}<p>{item.description}</p>{/if}
          {#if item.status !== 'revealed' && item.status !== 'abandoned'}
            <div class="actions">
              {#if item.status === 'planned'}<button disabled={saving} on:click={() => transition(item, 'plant')}>Plant</button>{/if}
              {#if item.status === 'planted' || item.status === 'reinforced'}
                <button disabled={saving} on:click={() => transition(item, 'reinforce')}>Reinforce</button>
                <button disabled={saving} on:click={() => transition(item, 'reveal')}>Reveal</button>
              {/if}
              <button disabled={saving} on:click={() => transition(item, 'abandon')}>Abandon</button>
            </div>
          {/if}
        </article>
      {/each}
    </div>
  {/if}
</section>

<style>
  .ledger-panel { margin-top: 1.5rem; border: 1px solid var(--border, #d8d8d8); border-radius: .8rem; padding: 1rem; }
  header, .badges, .actions { display: flex; align-items: center; justify-content: space-between; gap: .6rem; flex-wrap: wrap; }
  .eyebrow { margin: 0; font-size: .75rem; text-transform: uppercase; letter-spacing: .12em; opacity: .65; }
  h2, h3 { margin: .2rem 0; }
  form { display: grid; grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr)); gap: .6rem; margin: 1rem 0; }
  textarea { min-height: 2.6rem; }
  input, textarea, select, button { font: inherit; padding: .55rem; }
  .cards { display: grid; gap: .75rem; }
  article { border: 1px solid var(--border, #ddd); border-radius: .65rem; padding: .8rem; }
  article.overdue { border-width: 2px; }
  .badges span, .badges strong { font-size: .75rem; border: 1px solid currentColor; border-radius: 999px; padding: .15rem .45rem; }
  .error { font-weight: 600; }
  .empty { opacity: .7; }
  code { font-size: .8rem; }
</style>
