<script>
	import { getDashboard } from '$lib/api/client.js';
	import { onMount } from 'svelte';

	let dashboard = $state(null);
	let error = $state(null);

	onMount(async () => {
		try {
			dashboard = await getDashboard();
		} catch (e) {
			error = e.message;
		}
	});
</script>

<svelte:head>
	<title>Dashboard - Codex</title>
</svelte:head>

<h2>Dashboard</h2>

{#if error}
	<div class="error">Failed to load dashboard: {error}</div>
{:else if !dashboard}
	<p class="loading">Loading...</p>
{:else}
	<div class="stats">
		<div class="stat-card">
			<span class="stat-value">{dashboard.total_works}</span>
			<span class="stat-label">Total Works</span>
		</div>
		<div class="stat-card review">
			<span class="stat-value">{dashboard.needs_review}</span>
			<span class="stat-label">Needs Review</span>
		</div>
		<div class="stat-card">
			<span class="stat-value">{dashboard.pending_tasks}</span>
			<span class="stat-label">Pending Tasks</span>
		</div>
		<div class="stat-card">
			<span class="stat-value">{dashboard.total_collections}</span>
			<span class="stat-label">Collections</span>
		</div>
	</div>

	<section class="section">
		<h3>Media Roots</h3>
		{#if dashboard.media_roots?.length}
			<table class="table">
				<thead><tr><th>Name</th><th>Path</th></tr></thead>
				<tbody>
					{#each dashboard.media_roots as root}
						<tr><td>{root.Name}</td><td class="mono">{root.RootPath}</td></tr>
					{/each}
				</tbody>
			</table>
		{:else}
			<p class="text-muted">No media roots configured.</p>
		{/if}
	</section>

	<section class="section">
		<h3>Recently Added</h3>
		{#if dashboard.recent_works?.length}
			<table class="table">
				<thead><tr><th>Title</th><th>Language</th><th>Added</th><th>Review</th></tr></thead>
				<tbody>
					{#each dashboard.recent_works as work}
						<tr>
							<td><a href="/works/{work.ID}">{work.Title}</a></td>
							<td>{work.Language || '—'}</td>
							<td>{new Date(work.AddedAt).toLocaleDateString()}</td>
							<td>{work.NeedsReview ? 'Yes' : 'No'}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{:else}
			<p class="text-muted">No works yet. Run a scan to get started.</p>
		{/if}
	</section>
{/if}

<style>
	h2 { margin-bottom: 1.5rem; }
	h3 { margin-bottom: 0.75rem; font-size: 1rem; color: var(--text-muted); }

	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
		gap: 1rem;
		margin-bottom: 2rem;
	}

	.stat-card {
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 1.25rem;
		text-align: center;
	}

	.stat-card.review .stat-value {
		color: var(--warning);
	}

	.stat-value {
		display: block;
		font-size: 2rem;
		font-weight: 700;
		color: var(--primary);
	}

	.stat-label {
		color: var(--text-muted);
		font-size: 0.85rem;
	}

	.section { margin-bottom: 2rem; }

	.table {
		width: 100%;
		border-collapse: collapse;
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
	}

	.table th, .table td {
		padding: 0.5rem 0.75rem;
		text-align: left;
		border-bottom: 1px solid var(--border);
	}

	.table th {
		font-size: 0.8rem;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.mono { font-family: monospace; font-size: 0.85rem; }

	.error {
		background: rgba(239, 68, 68, 0.1);
		border: 1px solid var(--danger);
		color: var(--danger);
		padding: 0.75rem;
		border-radius: var(--radius);
	}

	.loading, .text-muted { color: var(--text-muted); }
</style>
