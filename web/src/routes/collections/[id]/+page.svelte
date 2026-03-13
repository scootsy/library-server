<script>
	import { getCollection, removeWorkFromCollection, updateCollection } from '$lib/api/client.js';
	import { onMount } from 'svelte';
	import { page } from '$app/state';

	let collection = $state(null);
	let works = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let error = $state(null);
	let message = $state('');

	const collId = $derived(page.params.id);

	onMount(loadCollection);

	async function loadCollection() {
		loading = true;
		try {
			const result = await getCollection(collId);
			collection = result.collection;
			works = result.works?.data || [];
			total = result.works?.total || 0;
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	async function handleRemoveWork(workId) {
		try {
			await removeWorkFromCollection(collId, workId);
			message = 'Work removed';
			await loadCollection();
			setTimeout(() => message = '', 3000);
		} catch (e) {
			message = 'Error: ' + e.message;
		}
	}
</script>

<svelte:head>
	<title>{collection?.Name || 'Collection'} - Codex</title>
</svelte:head>

{#if error}
	<div class="error">{error}</div>
{:else if loading}
	<p class="loading">Loading...</p>
{:else if collection}
	<a href="/collections" class="back">← Back to collections</a>
	<h2>{collection.Name}</h2>
	{#if collection.Description}
		<p class="description">{collection.Description}</p>
	{/if}
	<p class="meta">Type: {collection.CollectionType} · {total} work{total !== 1 ? 's' : ''}</p>

	{#if message}
		<div class="message">{message}</div>
	{/if}

	{#if works.length}
		<table class="table">
			<thead><tr><th>Title</th><th>Language</th><th>Actions</th></tr></thead>
			<tbody>
				{#each works as work}
					<tr>
						<td><a href="/works/{work.ID}">{work.Title}</a></td>
						<td>{work.Language || '—'}</td>
						<td>
							<button class="btn-small btn-danger" onclick={() => handleRemoveWork(work.ID)}>Remove</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{:else}
		<p class="text-muted">No works in this collection.</p>
	{/if}
{/if}

<style>
	.back { font-size: 0.85rem; color: var(--text-muted); display: inline-block; margin-bottom: 0.5rem; }
	h2 { margin-bottom: 0.25rem; }
	.description { color: var(--text-muted); margin-bottom: 0.5rem; }
	.meta { color: var(--text-muted); font-size: 0.85rem; margin-bottom: 1rem; }

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

	.table th { font-size: 0.8rem; color: var(--text-muted); text-transform: uppercase; }

	button {
		background: var(--primary);
		color: white;
		border: none;
		padding: 0.35rem 0.75rem;
		border-radius: var(--radius);
		cursor: pointer;
		font-size: 0.85rem;
	}

	.btn-small { font-size: 0.75rem; padding: 0.2rem 0.5rem; }

	.btn-danger {
		background: transparent;
		border: 1px solid var(--danger);
		color: var(--danger);
	}

	.btn-danger:hover { background: rgba(239, 68, 68, 0.1); }

	.message {
		background: rgba(108, 140, 255, 0.1);
		border: 1px solid var(--primary);
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius);
		margin-bottom: 1rem;
		font-size: 0.85rem;
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
