<script>
	import { getWork, updateWork, refreshMetadata, getMetadataTasks, selectCover } from '$lib/api/client.js';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import DOMPurify from 'dompurify';

	let work = $state(null);
	let tasks = $state([]);
	let loading = $state(true);
	let error = $state(null);
	let editing = $state(null);
	let editValue = $state('');
	let saving = $state(false);
	let message = $state('');

	const workId = $derived(page.params.id);

	onMount(async () => {
		await loadWork();
		await loadTasks();
	});

	async function loadWork() {
		loading = true;
		try {
			work = await getWork(workId);
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	async function loadTasks() {
		try {
			const result = await getMetadataTasks(workId);
			tasks = result.data || [];
		} catch (e) {
			// Non-critical, just log.
		}
	}

	function startEdit(field, value) {
		editing = field;
		editValue = value || '';
	}

	async function saveEdit(field) {
		saving = true;
		try {
			await updateWork(workId, { [field]: editValue });
			await loadWork();
			editing = null;
			message = 'Saved';
			setTimeout(() => message = '', 2000);
		} catch (e) {
			message = 'Error: ' + e.message;
		} finally {
			saving = false;
		}
	}

	function cancelEdit() {
		editing = null;
		editValue = '';
	}

	async function handleRefresh() {
		try {
			await refreshMetadata(workId);
			message = 'Metadata refresh queued';
			setTimeout(() => message = '', 3000);
		} catch (e) {
			message = 'Error: ' + e.message;
		}
	}

	async function handleSelectCover(source) {
		try {
			await selectCover(workId, source);
			await loadWork();
			message = 'Cover updated';
			setTimeout(() => message = '', 2000);
		} catch (e) {
			message = 'Error: ' + e.message;
		}
	}

	function formatDuration(seconds) {
		if (!seconds) return '—';
		const h = Math.floor(seconds / 3600);
		const m = Math.floor((seconds % 3600) / 60);
		return h > 0 ? `${h}h ${m}m` : `${m}m`;
	}

	function formatBytes(bytes) {
		if (!bytes) return '—';
		const units = ['B', 'KB', 'MB', 'GB'];
		let i = 0;
		let size = bytes;
		while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
		return `${size.toFixed(1)} ${units[i]}`;
	}
</script>

<svelte:head>
	<title>{work?.Title || 'Work'} - Codex</title>
</svelte:head>

{#if error}
	<div class="error">{error}</div>
{:else if loading}
	<p class="loading">Loading...</p>
{:else if work}
	<div class="work-header">
		<div>
			<a href="/browse" class="back">← Back to browse</a>
			<h2>{work.Title}</h2>
			{#if work.Subtitle}<p class="subtitle">{work.Subtitle}</p>{/if}
		</div>
		<div class="actions">
			<button class="btn-secondary" onclick={handleRefresh}>Refresh Metadata</button>
			{#if work.NeedsReview}
				<span class="badge warning">Needs Review</span>
			{/if}
		</div>
	</div>

	{#if message}
		<div class="message">{message}</div>
	{/if}

	<div class="work-grid">
		<section class="card">
			<h3>Metadata</h3>
			<dl class="metadata">
				{#each [
					['title', 'Title', work.Title],
					['subtitle', 'Subtitle', work.Subtitle],
					['language', 'Language', work.Language],
					['publisher', 'Publisher', work.Publisher],
					['publish_date', 'Published', work.PublishDate],
				] as [field, label, value]}
					<div class="field">
						<dt>{label}</dt>
						<dd>
							{#if editing === field}
								<div class="edit-inline">
									<input bind:value={editValue} onkeydown={(e) => e.key === 'Enter' && saveEdit(field)} />
									<button onclick={() => saveEdit(field)} disabled={saving}>Save</button>
									<button class="btn-secondary" onclick={cancelEdit}>Cancel</button>
								</div>
							{:else}
								<span class="value" ondblclick={() => startEdit(field, value)}>
									{value || '—'}
								</span>
								<button class="btn-edit" onclick={() => startEdit(field, value)}>Edit</button>
							{/if}
						</dd>
					</div>
				{/each}
			</dl>

			{#if work.Description}
				<div class="field">
					<dt>Description</dt>
					<dd class="description">
						{#if work.DescriptionFormat === 'html'}
							{@html DOMPurify.sanitize(work.Description)}
						{:else}
							{work.Description}
						{/if}
					</dd>
				</div>
			{/if}

			<div class="meta-info">
				{#if work.PageCount}<span>Pages: {work.PageCount}</span>{/if}
				{#if work.DurationSeconds}<span>Duration: {formatDuration(work.DurationSeconds)}</span>{/if}
				{#if work.MatchConfidence > 0}
					<span>Confidence: {(work.MatchConfidence * 100).toFixed(0)}%</span>
				{/if}
				{#if work.PrimarySource}<span>Source: {work.PrimarySource}</span>{/if}
			</div>
		</section>

		<section class="card">
			<h3>Contributors</h3>
			{#if work.contributors?.length}
				<ul class="list">
					{#each work.contributors as c}
						<li><strong>{c.name}</strong> <span class="role">({c.role})</span></li>
					{/each}
				</ul>
			{:else}
				<p class="text-muted">No contributors.</p>
			{/if}
		</section>

		<section class="card">
			<h3>Series</h3>
			{#if work.series?.length}
				<ul class="list">
					{#each work.series as s}
						<li>
							<a href="/browse?series={s.id}">{s.name}</a>
							{#if s.position != null} <span class="text-muted">#{s.position}</span>{/if}
						</li>
					{/each}
				</ul>
			{:else}
				<p class="text-muted">Not in any series.</p>
			{/if}
		</section>

		<section class="card">
			<h3>Tags</h3>
			{#if work.tags?.length}
				<div class="tags">
					{#each work.tags as t}
						<span class="tag">{t.Name}</span>
					{/each}
				</div>
			{:else}
				<p class="text-muted">No tags.</p>
			{/if}
		</section>

		<section class="card">
			<h3>Files</h3>
			{#if work.files?.length}
				<table class="table">
					<thead><tr><th>Filename</th><th>Format</th><th>Size</th></tr></thead>
					<tbody>
						{#each work.files as f}
							<tr>
								<td class="mono">{f.Filename}</td>
								<td>{f.Format}</td>
								<td>{formatBytes(f.SizeBytes)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{:else}
				<p class="text-muted">No files.</p>
			{/if}
		</section>

		<section class="card">
			<h3>Identifiers</h3>
			{#if work.identifiers && Object.keys(work.identifiers).length}
				<dl class="identifiers">
					{#each Object.entries(work.identifiers) as [type, value]}
						<div class="field"><dt>{type}</dt><dd class="mono">{value}</dd></div>
					{/each}
				</dl>
			{:else}
				<p class="text-muted">No identifiers.</p>
			{/if}
		</section>

		<section class="card">
			<h3>Covers</h3>
			{#if work.covers?.length}
				<div class="covers-grid">
					{#each work.covers as cover}
						<div class="cover-item" class:selected={cover.IsSelected}>
							<div class="cover-info">
								<strong>{cover.Source}</strong>
								{#if cover.Width && cover.Height}
									<span class="text-muted">{cover.Width}x{cover.Height}</span>
								{/if}
								<span class="mono">{cover.Filename}</span>
							</div>
							{#if !cover.IsSelected}
								<button class="btn-small" onclick={() => handleSelectCover(cover.Source)}>Select</button>
							{:else}
								<span class="badge success">Selected</span>
							{/if}
						</div>
					{/each}
				</div>
			{:else}
				<p class="text-muted">No covers.</p>
			{/if}
		</section>

		<section class="card">
			<h3>Ratings</h3>
			{#if work.ratings?.length}
				<table class="table">
					<thead><tr><th>Source</th><th>Score</th><th>Votes</th></tr></thead>
					<tbody>
						{#each work.ratings as r}
							<tr>
								<td>{r.Source}</td>
								<td>{r.Score}/{r.MaxScore}</td>
								<td>{r.Count || '—'}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{:else}
				<p class="text-muted">No ratings.</p>
			{/if}
		</section>

		{#if work.chapters?.length}
			<section class="card">
				<h3>Chapters</h3>
				<table class="table">
					<thead><tr><th>#</th><th>Title</th><th>Start</th><th>End</th></tr></thead>
					<tbody>
						{#each work.chapters as ch}
							<tr>
								<td>{ch.IndexPosition + 1}</td>
								<td>{ch.Title}</td>
								<td>{formatDuration(ch.StartSeconds)}</td>
								<td>{formatDuration(ch.EndSeconds)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</section>
		{/if}

		{#if tasks.length}
			<section class="card">
				<h3>Metadata Tasks</h3>
				<table class="table">
					<thead><tr><th>Type</th><th>Status</th><th>Created</th></tr></thead>
					<tbody>
						{#each tasks as task}
							<tr>
								<td>{task.TaskType}</td>
								<td>
									<span class="badge" class:success={task.Status === 'completed'}
										class:warning={task.Status === 'review'}
										class:danger={task.Status === 'failed'}>
										{task.Status}
									</span>
								</td>
								<td>{new Date(task.CreatedAt).toLocaleString()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</section>
		{/if}
	</div>
{/if}

<style>
	.back { font-size: 0.85rem; color: var(--text-muted); margin-bottom: 0.5rem; display: inline-block; }
	h2 { margin-bottom: 0.25rem; }
	.subtitle { color: var(--text-muted); margin-bottom: 1rem; }

	.work-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		margin-bottom: 1.5rem;
	}

	.actions { display: flex; gap: 0.5rem; align-items: center; }

	.work-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
		gap: 1rem;
	}

	.card {
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 1rem;
	}

	.card h3 {
		font-size: 0.85rem;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin-bottom: 0.75rem;
	}

	.field { margin-bottom: 0.5rem; }
	dt { font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.15rem; }
	dd { display: flex; align-items: center; gap: 0.5rem; }

	.value { cursor: pointer; }
	.value:hover { color: var(--primary); }

	.btn-edit {
		background: transparent;
		border: 1px solid var(--border);
		color: var(--text-muted);
		padding: 0.15rem 0.5rem;
		border-radius: var(--radius);
		cursor: pointer;
		font-size: 0.75rem;
	}

	.btn-edit:hover { border-color: var(--primary); color: var(--primary); }

	.edit-inline {
		display: flex;
		gap: 0.25rem;
	}

	.edit-inline input {
		background: var(--bg);
		border: 1px solid var(--primary);
		color: var(--text);
		padding: 0.25rem 0.5rem;
		border-radius: var(--radius);
		flex: 1;
	}

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
	button:disabled { opacity: 0.5; }

	.btn-secondary {
		background: transparent;
		border: 1px solid var(--border);
		color: var(--text);
	}

	.btn-secondary:hover { border-color: var(--primary); }

	.btn-small { font-size: 0.75rem; padding: 0.2rem 0.5rem; }

	.badge {
		padding: 0.15rem 0.5rem;
		border-radius: 999px;
		font-size: 0.75rem;
		font-weight: 600;
	}

	.badge.warning { background: rgba(245, 158, 11, 0.15); color: var(--warning); }
	.badge.success { background: rgba(34, 197, 94, 0.15); color: var(--success); }
	.badge.danger { background: rgba(239, 68, 68, 0.15); color: var(--danger); }

	.list { list-style: none; }
	.list li { padding: 0.25rem 0; border-bottom: 1px solid var(--border); }
	.list li:last-child { border-bottom: none; }
	.role { color: var(--text-muted); font-size: 0.85rem; }

	.tags { display: flex; flex-wrap: wrap; gap: 0.35rem; }
	.tag {
		background: var(--bg-hover);
		padding: 0.2rem 0.6rem;
		border-radius: 999px;
		font-size: 0.8rem;
		color: var(--text-muted);
	}

	.covers-grid { display: flex; flex-direction: column; gap: 0.5rem; }
	.cover-item {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.5rem;
		border: 1px solid var(--border);
		border-radius: var(--radius);
	}
	.cover-item.selected { border-color: var(--success); }
	.cover-info { display: flex; flex-direction: column; gap: 0.15rem; }

	.table {
		width: 100%;
		border-collapse: collapse;
	}
	.table th, .table td {
		padding: 0.35rem 0.5rem;
		text-align: left;
		border-bottom: 1px solid var(--border);
		font-size: 0.85rem;
	}
	.table th { color: var(--text-muted); text-transform: uppercase; font-size: 0.75rem; }

	.description { font-size: 0.9rem; line-height: 1.6; }
	.meta-info {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
		margin-top: 0.75rem;
		padding-top: 0.75rem;
		border-top: 1px solid var(--border);
		font-size: 0.85rem;
		color: var(--text-muted);
	}

	.identifiers .field { display: flex; gap: 0.5rem; align-items: baseline; }
	.identifiers dt { min-width: 80px; }

	.mono { font-family: monospace; font-size: 0.85rem; }
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
