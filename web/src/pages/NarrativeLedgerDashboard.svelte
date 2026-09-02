<script lang="ts">
  import type { LedgerDashboard } from '../lib/ledger'

  export let projectId: string

  let dashboard: LedgerDashboard | null = null
  let diagnostics: Array<{code: string; severity: string; entity_key: string; message: string}> = []
  let error = ''
  let loadedProject = ''
  let chapter = 1

  $: if (projectId && projectId !== loadedProject) {
    loadedProject = projectId
    void reload()
  }

  async function reload(): Promise<void> {
    if (!projectId) return
    error = ''
    try {
      const base = `/api/projects/${encodeURIComponent(projectId)}/ledger`
      const [dashboardResponse, diagnosticsResponse] = await Promise.all([
        fetch(`${base}/dashboard?chapter=${chapter}`),
        fetch(`${base}/diagnostics?chapter=${chapter}`),
      ])
      const dashboardPayload = await dashboardResponse.json()
      const diagnosticsPayload = await diagnosticsResponse.json()
      if (!dashboardResponse.ok) throw new Error(dashboardPayload?.error?.message ?? 'Dashboard request failed')
      if (!diagnosticsResponse.ok) throw new Error(diagnosticsPayload?.error?.message ?? 'Diagnostics request failed')
      dashboard = dashboardPayload as LedgerDashboard
      diagnostics = diagnosticsPayload.diagnostics ?? []
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Unable to load Narrative Ledger dashboard'
    }
  }
</script>

<section class="dashboard" aria-labelledby="ledger-dashboard-title">
  <header>
    <div>
      <p>Narrative Ledger</p>
      <h2 id="ledger-dashboard-title">Chapter-N dashboard</h2>
    </div>
    <label>
      Chapter
      <input type="number" min="0" bind:value={chapter} on:change={reload} />
    </label>
  </header>
  {#if error}<p role="alert">{error}</p>{/if}
  {#if dashboard}
    <div class="metrics">
      <article><strong>{dashboard.foreshadows_overdue}</strong><span>Overdue</span></article>
      <article><strong>{dashboard.foreshadows_critical}</strong><span>Critical active</span></article>
      <article><strong>{dashboard.foreshadows_upcoming}</strong><span>Upcoming</span></article>
      <article><strong>{dashboard.secrets_hidden}</strong><span>Private secrets</span></article>
      <article><strong>{dashboard.secrets_public}</strong><span>Public secrets</span></article>
    </div>
  {/if}
  {#if diagnostics.length > 0}
    <ul>
      {#each diagnostics as diagnostic (diagnostic.code + diagnostic.entity_key)}
        <li><code>{diagnostic.code}</code> {diagnostic.entity_key}: {diagnostic.message}</li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .dashboard { margin-top: 1.5rem; border: 1px solid var(--border, #d8d8d8); border-radius: .8rem; padding: 1rem; }
  header { display: flex; justify-content: space-between; align-items: center; gap: 1rem; flex-wrap: wrap; }
  header p, h2 { margin: .2rem 0; }
  .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(8rem, 1fr)); gap: .6rem; margin-top: 1rem; }
  article { display: grid; gap: .25rem; border: 1px solid var(--border, #ddd); border-radius: .6rem; padding: .7rem; }
  article strong { font-size: 1.4rem; }
  input { font: inherit; padding: .4rem; }
  code { font-size: .78rem; }
</style>
