<script lang="ts">
  import { newIdempotencyKey, type Page, type Secret } from '../lib/ledger'

  export let projectId: string

  let items: Secret[] = []
  let loading = false
  let saving = false
  let error = ''
  let loadedProject = ''
  let chapter = 1
  let key = ''
  let title = ''
  let description = ''
  let holder = ''
  let publicFromChapter: number | null = null

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
      const page = await api<Page<Secret>>(
        `/api/projects/${encodeURIComponent(projectId)}/secrets?chapter=${chapter}&limit=100`,
      )
      items = page.items
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Unable to load secrets'
    } finally {
      loading = false
    }
  }

  async function createSecret(): Promise<void> {
    if (!key.trim() || !title.trim()) {
      error = 'Key and title are required.'
      return
    }
    saving = true
    error = ''
    try {
      const knowledge = holder.trim()
        ? [{holder: holder.trim(), known_from_chapter: chapter}]
        : []
      await api(`/api/projects/${encodeURIComponent(projectId)}/secrets`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': newIdempotencyKey('secret-create'),
        },
        body: JSON.stringify({
          chapter,
          action: 'create',
          key,
          title,
          description,
          status: 'hidden',
          public_from_chapter: publicFromChapter,
          knowledge,
        }),
      })
      key = ''
      title = ''
      description = ''
      holder = ''
      publicFromChapter = null
      await reload()
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Unable to create secret'
    } finally {
      saving = false
    }
  }

  async function transition(item: Secret, action: 'hint' | 'reveal' | 'retire'): Promise<void> {
    saving = true
    error = ''
    try {
      await api(`/api/projects/${encodeURIComponent(projectId)}/secrets/${encodeURIComponent(item.key)}`, {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': newIdempotencyKey(`secret-${action}`),
        },
        body: JSON.stringify({chapter, action}),
      })
      await reload()
    } catch (cause) {
      error = cause instanceof Error ? cause.message : `Unable to ${action} secret`
    } finally {
      saving = false
    }
  }
</script>

<section class="ledger-panel" aria-labelledby="secret-title">
  <header>
    <div>
      <p class="eyebrow">Knowledge Boundary</p>
      <h2 id="secret-title">Secrets</h2>
    </div>
    <label>
      Chapter N
      <input type="number" min="0" bind:value={chapter} on:change={reload} />
    </label>
  </header>

  <form on:submit|preventDefault={createSecret}>
    <input aria-label="Secret key" placeholder="stable-key" bind:value={key} />
    <input aria-label="Secret title" placeholder="Secret title" bind:value={title} />
    <textarea aria-label="Secret description" placeholder="Secret content" bind:value={description}></textarea>
    <input aria-label="Initial holder" placeholder="Initial holder (optional)" bind:value={holder} />
    <label>
      Public from chapter
      <input type="number" min="0" bind:value={publicFromChapter} />
    </label>
    <button type="submit" disabled={saving}>Add secret</button>
  </form>

  {#if error}<p class="error" role="alert">{error}</p>{/if}
  {#if loading}
    <p>Loading Chapter-N knowledge boundaries…</p>
  {:else if items.length === 0}
    <p class="empty">No secrets at Chapter {chapter}.</p>
  {:else}
    <div class="cards">
      {#each items as item (item.id)}
        <article>
          <div class="badges">
            <strong>{item.status.toUpperCase()}</strong>
            <span>{item.public ? 'PUBLIC' : 'PRIVATE'}</span>
            <span>{item.holders.length} holder{item.holders.length === 1 ? '' : 's'}</span>
          </div>
          <h3>{item.title}</h3>
          <code>{item.key}</code>
          {#if item.description}<p>{item.description}</p>{/if}
          <p><strong>Known by Chapter {chapter}:</strong> {item.holders.join(', ') || 'nobody recorded'}</p>
          {#if item.public_from_chapter !== null && item.public_from_chapter !== undefined}
            <p>Public from chapter {item.public_from_chapter}.</p>
          {/if}
          {#if item.status !== 'retired'}
            <div class="actions">
              {#if item.status === 'hidden'}<button disabled={saving} on:click={() => transition(item, 'hint')}>Hint</button>{/if}
              {#if item.status === 'hidden' || item.status === 'hinted'}<button disabled={saving} on:click={() => transition(item, 'reveal')}>Reveal</button>{/if}
              <button disabled={saving} on:click={() => transition(item, 'retire')}>Retire</button>
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
  input, textarea, button { font: inherit; padding: .55rem; }
  .cards { display: grid; gap: .75rem; }
  article { border: 1px solid var(--border, #ddd); border-radius: .65rem; padding: .8rem; }
  .badges span, .badges strong { font-size: .75rem; border: 1px solid currentColor; border-radius: 999px; padding: .15rem .45rem; }
  .error { font-weight: 600; }
  .empty { opacity: .7; }
  code { font-size: .8rem; }
</style>
