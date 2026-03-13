<script>
	import { getReviewQueue, getMetadataTasks, applyCandidate } from '$lib/api/client.js';
	import { onMount } from 'svelte';

	let works = $state([]);
	let total = $state(0);
	let loading = $state(true);
	let error = $state(null);
	let page = $state(0);
	let selectedWork = $state(null);
	let workTasks = $state([]);
	let message = $state('');
	const limit = 20;

	onMount(loadQueue);

	async function loadQueue() {
		loading = true;
		error = null;
		try {
			const result = await getReviewQueue(limit, page * limit);
			works = result.data || [];
			total = result.total || 0;
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	async function viewCandidates(work) {
		selectedWork = work;
		try {
			const result = await getMetadataTasks(work.ID);
			workTasks = result.data || [];
		} catch (e) {
			message = 'Error loading tasks: ' + e.message;
		}
	}

	async function handleApply(taskId, candidateIndex) {
		try {
			await applyCandidate(taskId, candidateIndex);
			message = 'Candidate applied successfully';
			selectedWork = null;
			await loadQueue();
			setTimeout(() => message = '', 3000);
		} catch (e) {
			message = 'Error: ' + e.message;
		}
	}

	function prevPage() { if (page > 0) { page--; loadQueue(); } }
	function nextPage() { if ((page + 1) * limit < total) { page++; loadQueue(); } }

	function parseCandidates(candidatesJSON) {
		try {
			return JSON.parse(candidatesJSON || '[]');
		} catch {
			return [];
		}
	}
</script>

<svelte:head>
	<title>Review Queue - Codex</title>
</svelte:head>

<h2>Metadata Review Queue</h2>

{#if message}
	<div class="message">{message}</div>
{/if}

{#if error}
	<div class="error">{error}</div>
{:else if loading}
	<p class="loading">Loading...</p>
{:else}
	<p class="info">{total} work{total !== 1 ? 's' : ''} need review</p>

	{#if selectedWork}
		<div class="detail-panel">
			<div class="detail-header">
				<h3>Candidates for: {selectedWork.Title}</h3>
				<button class="btn-secondary" onclick={() => selectedWork = null}>Close</button>
			</div>

			{#each workTasks as task}
				{#if task.Candidates}
					{@const candidates = parseCandidates(task.Candidates)}
					<div class="task-section">
						<p class="text-muted">Task: {task.TaskType} — Status: {task.Status}</p>
						{#if candidates.length}
							<div class="candidates">
								{#each candidates as candidate, i}
									<div class="candidate">
										<div class="candidate-info">
											<strong>{candidate.candidate?.title || 'Unknown'}</strong>
											{#if candidate.candidate?.authors?.length}
												<span class="text-muted">by {candidate.candidate.authors.join(', ')}</span>
											{/if}
											<span class="confidence" class:high={candidate.score?.overall >= 0.85}
												class:medium={candidate.score?.overall >= 0.5 && candidate.score?.overall < 0.85}
												class:low={candidate.score?.overall < 0.5}>
												{((candidate.score?.overall || 0) * 100).toFixed(0)}% confidence
											</span>
											{#if candidate.candidate?.source}
												<span class="text-muted">Source: {candidate.candidate.source}</span>
											{/if}
										</div>
										<button onclick={() => handleApply(task.ID, i)}>Apply</button>
									</div>
								{/each}
							</div>
						{:else}
							<p class="text-muted">No candidates found.</p>
						{/if}
					</div>
				{/if}
			{:else}
				<p class="text-muted">No metadata tasks for this work.</p>
			{/each}
		</div>
	{/if}

	<table class="table">
		<thead>
			<tr>
				<th>Title</th>
				<th>Confidence</th>
				<th>Added</th>
				<th>Actions</th>
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
					<td>
						{#if work.MatchConfidence > 0}
							{(work.MatchConfidence * 100).toFixed(0)}%
						{:else}
							—
						{/if}
					</td>
					<td>{new Date(work.AddedAt).toLocaleDateString()}</td>
					<td>
						<button class="btn-small" onclick={() => viewCandidates(work)}>Review</button>
					</td>
				</tr>
			{:else}
				<tr><td colspan="4" class="text-muted">No works need review.</td></tr>
			{/each}
		</tbody>
	</table>

	{#if total > limit}
		<div class="pagination">
			<button onclick={prevPage} disabled={page === 0}>Previous</button>
			<span>Page {page + 1} of {Math.ceil(total / limit)}</span>
			<button onclick={nextPage} disabled={(page + 1) * limit >= total}>Next</button>
		</div>
	{/if}
{/if}

<style>
	h2 { margin-bottom: 1rem; }
	.info { color: var(--text-muted); margin-bottom: 1rem; }

	.detail-panel {
		background: var(--bg-surface);
		border: 1px solid var(--primary);
		border-radius: var(--radius);
		padding: 1rem;
		margin-bottom: 1.5rem;
	}

	.detail-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
	}

	.detail-header h3 { font-size: 1rem; }

	.task-section { margin-bottom: 1rem; }

	.candidates { display: flex; flex-direction: column; gap: 0.5rem; }

	.candidate {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem;
		border: 1px solid var(--border);
		border-radius: var(--radius);
	}

	.candidate-info { display: flex; flex-direction: column; gap: 0.15rem; }

	.confidence { font-weight: 600; font-size: 0.85rem; }
	.confidence.high { color: var(--success); }
	.confidence.medium { color: var(--warning); }
	.confidence.low { color: var(--danger); }

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
	}

	.subtitle { color: var(--text-muted); font-size: 0.85rem; }

	button {
		background: var(--primary);
		color: white;
		border: none;
		padding: 0.35rem 0.75rem;
		border-radius: var(--radius);
		cursor: pointer;
		font-size: 0.85rem;
	}

	button:hover { background: var(--primary-hover); }
	button:disabled { opacity: 0.5; cursor: not-allowed; }

	.btn-secondary {
		background: transparent;
		border: 1px solid var(--border);
		color: var(--text);
	}

	.btn-small { font-size: 0.75rem; padding: 0.2rem 0.5rem; }

	.pagination {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 1rem;
		margin-top: 1rem;
	}

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
