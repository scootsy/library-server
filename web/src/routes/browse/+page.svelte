<script>
	import { listWorks, searchWorks } from '$lib/api/client.js';
	import { onMount } from 'svelte';

	let works = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let error = $state(null);
	let searchQuery = $state('');
	let sortBy = $state('sort_title');
	let sortOrder = $state('asc');
	let page = $state(0);
	let needsReview = $state('');
	const limit = 50;

	async function loadWorks() {
		loading = true;
		error = null;
		try {
			let result;
			if (searchQuery.trim()) {
				result = await searchWorks(searchQuery, limit, page * limit);
			} else {
				result = await listWorks({
					limit,
					offset: page * limit,
					sort: sortBy,
					order: sortOrder,
					needs_review: needsReview || undefined
				});
			}
			works = result.data || [];
			total = result.total || 0;
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	onMount(loadWorks);

	function handleSearch() {
		page = 0;
		loadWorks();
	}

	function handleSort(col) {
		if (sortBy === col) {
			sortOrder = sortOrder === 'asc' ? 'desc' : 'asc';
		} else {
			sortBy = col;
			sortOrder = 'asc';
		}
		loadWorks();
	}

	function prevPage() { if (page > 0) { page--; loadWorks(); } }
	function nextPage() { if ((page + 1) * limit < total) { page++; loadWorks(); } }
</script>

<svelte:head>
	<title>Browse - Codex</title>
</svelte:head>

<h2>Browse Library</h2>

<div class="controls">
	<div class="search-bar">
		<input
			type="text"
			placeholder="Search works..."
			bind:value={searchQuery}
			onkeydown={(e) => e.key === 'Enter' && handleSearch()}
		/>
		<button onclick={handleSearch}>Search</button>
	</div>
	<div class="filters">
		<select bind:value={needsReview} onchange={handleSearch}>
			<option value="">All</option>
			<option value="true">Needs Review</option>
			<option value="false">Reviewed</option>
		</select>
	</div>
</div>

{#if error}
	<div class="error">{error}</div>
{:else if loading}
	<p class="loading">Loading...</p>
{:else}
	<div class="results-info">
		Showing {Math.min(page * limit + 1, total)}–{Math.min((page + 1) * limit, total)} of {total} works
	</div>

	<table class="table">
		<thead>
			<tr>
				<th class="sortable" onclick={() => handleSort('sort_title')}>
					Title {sortBy === 'sort_title' ? (sortOrder === 'asc' ? '↑' : '↓') : ''}
				</th>
				<th>Language</th>
				<th class="sortable" onclick={() => handleSort('publish_date')}>
					Published {sortBy === 'publish_date' ? (sortOrder === 'asc' ? '↑' : '↓') : ''}
				</th>
				<th class="sortable" onclick={() => handleSort('added_at')}>
					Added {sortBy === 'added_at' ? (sortOrder === 'asc' ? '↑' : '↓') : ''}
				</th>
				<th>Confidence</th>
				<th>Review</th>
			</tr>
		</thead>
		<tbody>
			{#each works as work}
				<tr>
					<td>
						<a href="/works/{work.ID}">
							<strong>{work.Title}</strong>
							{#if work.Subtitle}<br/><span class="subtitle">{work.Subtitle}</span>{/if}
						</a>
					</td>
					<td>{work.Language || '—'}</td>
					<td>{work.PublishDate || '—'}</td>
					<td>{new Date(work.AddedAt).toLocaleDateString()}</td>
					<td>
						{#if work.MatchConfidence > 0}
							<span class="confidence" class:high={work.MatchConfidence >= 0.85} class:medium={work.MatchConfidence >= 0.5 && work.MatchConfidence < 0.85} class:low={work.MatchConfidence < 0.5}>
								{(work.MatchConfidence * 100).toFixed(0)}%
							</span>
						{:else}
							—
						{/if}
					</td>
					<td>{work.NeedsReview ? '⚠' : '✓'}</td>
				</tr>
			{:else}
				<tr><td colspan="6" class="text-muted">No works found.</td></tr>
			{/each}
		</tbody>
	</table>

	<div class="pagination">
		<button onclick={prevPage} disabled={page === 0}>Previous</button>
		<span>Page {page + 1} of {Math.max(1, Math.ceil(total / limit))}</span>
		<button onclick={nextPage} disabled={(page + 1) * limit >= total}>Next</button>
	</div>
{/if}

<style>
	h2 { margin-bottom: 1rem; }

	.controls {
		display: flex;
		gap: 1rem;
		margin-bottom: 1rem;
		flex-wrap: wrap;
	}

	.search-bar {
		display: flex;
		gap: 0.5rem;
		flex: 1;
		min-width: 200px;
	}

	input, select {
		background: var(--bg-surface);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius);
		font-size: 0.9rem;
	}

	input { flex: 1; }

	button {
		background: var(--primary);
		color: white;
		border: none;
		padding: 0.5rem 1rem;
		border-radius: var(--radius);
		cursor: pointer;
		font-size: 0.9rem;
	}

	button:hover { background: var(--primary-hover); }
	button:disabled { opacity: 0.5; cursor: not-allowed; }

	.results-info {
		color: var(--text-muted);
		font-size: 0.85rem;
		margin-bottom: 0.5rem;
	}

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

	.sortable { cursor: pointer; }
	.sortable:hover { color: var(--primary); }

	.subtitle { color: var(--text-muted); font-size: 0.85rem; }

	.confidence { font-weight: 600; font-size: 0.85rem; }
	.confidence.high { color: var(--success); }
	.confidence.medium { color: var(--warning); }
	.confidence.low { color: var(--danger); }

	.pagination {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 1rem;
		margin-top: 1rem;
	}

	.error {
		background: rgba(239, 68, 68, 0.1);
		border: 1px solid var(--danger);
		color: var(--danger);
		padding: 0.75rem;
		border-radius: var(--radius);
	}

	.loading, .text-muted { color: var(--text-muted); }
</style>
