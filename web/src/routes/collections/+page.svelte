<script>
	import { listCollections, createCollection, deleteCollection } from '$lib/api/client.js';
	import { onMount } from 'svelte';

	let collections = $state([]);
	let loading = $state(true);
	let error = $state(null);
	let showCreate = $state(false);
	let newName = $state('');
	let newDescription = $state('');
	let newType = $state('manual');
	let message = $state('');

	onMount(loadCollections);

	async function loadCollections() {
		loading = true;
		try {
			const result = await listCollections();
			collections = result.data || [];
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	async function handleCreate() {
		if (!newName.trim()) return;
		try {
			await createCollection({
				name: newName.trim(),
				description: newDescription.trim(),
				collection_type: newType
			});
			newName = '';
			newDescription = '';
			showCreate = false;
			message = 'Collection created';
			await loadCollections();
			setTimeout(() => message = '', 3000);
		} catch (e) {
			message = 'Error: ' + e.message;
		}
	}

	async function handleDelete(id, name) {
		if (!confirm(`Delete collection "${name}"?`)) return;
		try {
			await deleteCollection(id);
			message = 'Collection deleted';
			await loadCollections();
			setTimeout(() => message = '', 3000);
		} catch (e) {
			message = 'Error: ' + e.message;
		}
	}
</script>

<svelte:head>
	<title>Collections - Codex</title>
</svelte:head>

<div class="header">
	<h2>Collections</h2>
	<button onclick={() => showCreate = !showCreate}>
		{showCreate ? 'Cancel' : 'New Collection'}
	</button>
</div>

{#if message}
	<div class="message">{message}</div>
{/if}

{#if showCreate}
	<div class="create-form">
		<input type="text" placeholder="Collection name" bind:value={newName} />
		<input type="text" placeholder="Description (optional)" bind:value={newDescription} />
		<select bind:value={newType}>
			<option value="manual">Manual</option>
			<option value="smart">Smart</option>
		</select>
		<button onclick={handleCreate}>Create</button>
	</div>
{/if}

{#if error}
	<div class="error">{error}</div>
{:else if loading}
	<p class="loading">Loading...</p>
{:else if collections.length === 0}
	<p class="text-muted">No collections yet. Create one to get started.</p>
{:else}
	<div class="collection-list">
		{#each collections as coll}
			<div class="collection-card">
				<div class="collection-info">
					<h3><a href="/collections/{coll.ID}">{coll.Name}</a></h3>
					{#if coll.Description}
						<p class="text-muted">{coll.Description}</p>
					{/if}
					<div class="meta">
						<span class="badge">{coll.CollectionType}</span>
						{#if coll.IsPublic}<span class="badge success">Public</span>{/if}
					</div>
				</div>
				<button class="btn-danger" onclick={() => handleDelete(coll.ID, coll.Name)}>Delete</button>
			</div>
		{/each}
	</div>
{/if}

<style>
	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
	}

	.create-form {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 1.5rem;
		flex-wrap: wrap;
	}

	input, select {
		background: var(--bg-surface);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius);
		font-size: 0.9rem;
	}

	input { flex: 1; min-width: 150px; }

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

	.btn-danger {
		background: transparent;
		border: 1px solid var(--danger);
		color: var(--danger);
		padding: 0.3rem 0.75rem;
		font-size: 0.8rem;
	}

	.btn-danger:hover { background: rgba(239, 68, 68, 0.1); }

	.collection-list { display: flex; flex-direction: column; gap: 0.75rem; }

	.collection-card {
		display: flex;
		justify-content: space-between;
		align-items: center;
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 1rem;
	}

	.collection-info h3 { margin-bottom: 0.25rem; }

	.meta { display: flex; gap: 0.5rem; margin-top: 0.35rem; }

	.badge {
		padding: 0.1rem 0.5rem;
		border-radius: 999px;
		font-size: 0.75rem;
		background: var(--bg-hover);
		color: var(--text-muted);
	}

	.badge.success { background: rgba(34, 197, 94, 0.15); color: var(--success); }

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
