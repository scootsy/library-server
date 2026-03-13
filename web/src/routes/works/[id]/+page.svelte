<script>
	import { page } from '$app/state';
	import {
		deleteWork,
		getMetadataTasks,
		getWork,
		getWorkCoverUrl,
		getWorkFileDownloadUrl,
		refreshMetadata,
		selectCover,
		updateWork
	} from '$lib/api/client.js';
	import DOMPurify from 'dompurify';
	import { onMount } from 'svelte';

	const FILE_FORMAT_PRIORITY = {
		m4b: 0,
		epub: 1,
		pdf: 2,
		mp3: 3,
		m4a: 4,
		flac: 5,
		mobi: 6,
		azw3: 7
	};

	let work = $state(null);
	let tasks = $state([]);
	let loading = $state(true);
	let error = $state(null);
	let saving = $state(false);
	let message = $state('');
	let activeTab = $state('files');
	let menuOpen = $state(false);
	let editorOpen = $state(false);
	let coverLoadFailed = $state(false);

	let editor = $state({
		title: '',
		subtitle: '',
		language: '',
		publisher: '',
		publishDate: '',
		description: ''
	});

	const workId = $derived(page.params.id);

	const sortedContributors = $derived.by(() =>
		[...(work?.contributors || [])].sort((a, b) => {
			const priorityDiff = getContributorPriority(a) - getContributorPriority(b);
			if (priorityDiff !== 0) return priorityDiff;
			if ((a.position || 0) !== (b.position || 0)) return (a.position || 0) - (b.position || 0);
			return (a.name || '').localeCompare(b.name || '');
		})
	);

	const narrators = $derived.by(() =>
		sortedContributors.filter((contributor) => isRole(contributor.role, ['narrator']))
	);

	const primarySeries = $derived.by(() => work?.series?.[0] || null);

	const selectedCover = $derived.by(
		() => work?.covers?.find((cover) => cover.IsSelected) || work?.covers?.[0] || null
	);

	const coverUrl = $derived.by(() => {
		if (!selectedCover) return '';
		const version = encodeURIComponent(selectedCover.Filename || selectedCover.Source || 'cover');
		return `${getWorkCoverUrl(workId)}?v=${version}`;
	});

	const primaryFile = $derived.by(() => choosePrimaryFile(work?.files || []));

	const alternateFiles = $derived.by(() =>
		(work?.files || []).filter((file) => file.ID !== primaryFile?.ID)
	);

	const formatBadges = $derived.by(() => buildFormatBadges(work?.files || [], primaryFile));

	const totalFileSize = $derived.by(() =>
		(work?.files || []).reduce((total, file) => total + (file.SizeBytes || 0), 0)
	);

	const computedDuration = $derived.by(() => {
		if (work?.DurationSeconds) return work.DurationSeconds;
		return (work?.files || []).reduce((total, file) => total + (file.DurationSeconds || 0), 0);
	});

	const isbnValue = $derived.by(() => pickIdentifier(work?.identifiers || {}));

	const metadataItems = $derived.by(() => [
		{ label: 'Publisher', value: work?.Publisher || '--' },
		{ label: 'Published', value: formatDate(work?.PublishDate) },
		{ label: 'Language', value: work?.Language || '--' },
		{ label: 'Page Count', value: work?.PageCount ? String(work.PageCount) : '--' },
		{ label: 'ISBN', value: isbnValue || '--' },
		{ label: 'File Size', value: totalFileSize ? formatBytes(totalFileSize) : '--' },
		{ label: 'Narrator', value: narrators.length ? narrators.map((person) => person.name).join(', ') : '--' },
		{ label: 'Duration', value: formatDuration(computedDuration) }
	]);

	const supportingFacts = $derived.by(() => {
		const facts = [];

		if (work?.NeedsReview) facts.push({ label: 'Needs Review', tone: 'warning' });
		if (work?.IsAbridged) facts.push({ label: 'Abridged', tone: 'neutral' });
		if (work?.HasMediaOverlay) facts.push({ label: 'Media Overlay', tone: 'neutral' });
		if (work?.PrimarySource) facts.push({ label: `Source: ${formatSourceName(work.PrimarySource)}`, tone: 'neutral' });
		if (work?.MatchConfidence > 0) {
			facts.push({
				label: `Match: ${(work.MatchConfidence * 100).toFixed(0)}%`,
				tone: work.MatchConfidence >= 0.85 ? 'success' : 'warning'
			});
		}

		return facts;
	});

	const primaryRating = $derived.by(() => choosePrimaryRating(work?.ratings || []));
	const filledStars = $derived.by(() => Math.round(primaryRating ? (primaryRating.Score / primaryRating.MaxScore) * 5 : 0));

	const downloadSummary = $derived.by(() => {
		if (!primaryFile) return 'No downloadable file';
		return `${formatLabel(primaryFile.Format)} / ${formatBytes(primaryFile.SizeBytes)}`;
	});

	onMount(async () => {
		await loadWork();
		await loadTasks();
	});

	async function loadWork() {
		loading = true;
		error = null;
		coverLoadFailed = false;

		try {
			work = await getWork(workId);
			syncEditor();
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
		} catch {
			tasks = [];
		}
	}

	function syncEditor() {
		if (!work) return;
		editor.title = work.Title || '';
		editor.subtitle = work.Subtitle || '';
		editor.language = work.Language || '';
		editor.publisher = work.Publisher || '';
		editor.publishDate = work.PublishDate || '';
		editor.description = work.Description || '';
	}

	function setMessage(text, timeout = 3000) {
		message = text;
		setTimeout(() => {
			if (message === text) message = '';
		}, timeout);
	}

	async function handleFetchMetadata() {
		menuOpen = false;

		try {
			await refreshMetadata(workId);
			await loadTasks();
			setMessage('Metadata fetch queued');
		} catch (e) {
			setMessage(`Error: ${e.message}`, 5000);
		}
	}

	async function handleSelectCover(source) {
		try {
			await selectCover(workId, source);
			coverLoadFailed = false;
			await loadWork();
			setMessage('Cover updated', 2000);
		} catch (e) {
			setMessage(`Error: ${e.message}`, 5000);
		}
	}

	function openEditor() {
		menuOpen = false;
		editorOpen = true;
		syncEditor();
	}

	function closeEditor() {
		editorOpen = false;
		syncEditor();
	}

	async function saveEditor() {
		if (!work) return;

		const patch = {};

		if (editor.title.trim() !== (work.Title || '')) patch.title = editor.title.trim();
		if (editor.subtitle.trim() !== (work.Subtitle || '')) patch.subtitle = editor.subtitle.trim();
		if (editor.language.trim() !== (work.Language || '')) patch.language = editor.language.trim();
		if (editor.publisher.trim() !== (work.Publisher || '')) patch.publisher = editor.publisher.trim();
		if (editor.publishDate.trim() !== (work.PublishDate || '')) patch.publish_date = editor.publishDate.trim();

		if (editor.description.trim() !== (work.Description || '')) {
			patch.description = editor.description.trim();
			patch.description_format = 'plain';
		}

		if (!Object.keys(patch).length) {
			editorOpen = false;
			setMessage('No metadata changes to save', 2000);
			return;
		}

		saving = true;
		try {
			await updateWork(workId, patch);
			await loadWork();
			editorOpen = false;
			setMessage('Metadata updated', 2000);
		} catch (e) {
			setMessage(`Error: ${e.message}`, 5000);
		} finally {
			saving = false;
		}
	}

	async function handleDeleteWork() {
		menuOpen = false;

		if (!confirm(`Delete "${work?.Title || 'this work'}"? This cannot be undone.`)) {
			return;
		}

		try {
			await deleteWork(workId);
			window.location.href = '/browse';
		} catch (e) {
			setMessage(`Error: ${e.message}`, 5000);
		}
	}

	function handleCoverError() {
		coverLoadFailed = true;
	}

	function buildBrowseLink(query) {
		return `/browse?q=${encodeURIComponent(query)}`;
	}

	function isRole(role, expected) {
		return expected.includes((role || '').trim().toLowerCase());
	}

	function getContributorPriority(contributor) {
		const role = (contributor.role || '').trim().toLowerCase();
		if (role === 'author') return 0;
		if (role === 'writer') return 1;
		if (role === 'narrator') return 2;
		return 3;
	}

	function choosePrimaryFile(files) {
		if (!files.length) return null;

		return [...files].sort((a, b) => {
			const formatDiff = getFormatPriority(a.Format) - getFormatPriority(b.Format);
			if (formatDiff !== 0) return formatDiff;
			if ((b.SizeBytes || 0) !== (a.SizeBytes || 0)) return (b.SizeBytes || 0) - (a.SizeBytes || 0);
			return (a.Filename || '').localeCompare(b.Filename || '');
		})[0];
	}

	function buildFormatBadges(files, mainFile) {
		const seen = new Set();
		const badges = [];

		for (const file of files) {
			const format = formatLabel(file.Format);
			const key = format.toLowerCase();
			if (seen.has(key)) continue;
			seen.add(key);
			badges.push({
				label: format,
				isPrimary: mainFile ? formatLabel(mainFile.Format).toLowerCase() === key : false
			});
		}

		return badges;
	}

	function choosePrimaryRating(ratings) {
		if (!ratings.length) return null;

		return [...ratings].sort((a, b) => {
			const countDiff = (b.Count || 0) - (a.Count || 0);
			if (countDiff !== 0) return countDiff;
			return (b.Score || 0) - (a.Score || 0);
		})[0];
	}

	function pickIdentifier(identifiers) {
		return identifiers.isbn_13 || identifiers.isbn || identifiers.isbn13 || identifiers.isbn_10 || identifiers.isbn10 || '';
	}

	function getFormatPriority(format) {
		const normalized = (format || '').trim().toLowerCase();
		return normalized in FILE_FORMAT_PRIORITY ? FILE_FORMAT_PRIORITY[normalized] : 999;
	}

	function formatLabel(value) {
		return (value || 'file').toUpperCase();
	}

	function formatDate(value) {
		if (!value) return '--';

		const parsed = new Date(value);
		if (Number.isNaN(parsed.getTime())) return value;

		if (value.length <= 4) {
			return parsed.toLocaleDateString(undefined, { year: 'numeric' });
		}

		return parsed.toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}

	function formatDuration(seconds) {
		if (!seconds) return '--';

		const totalSeconds = Math.round(seconds);
		const hours = Math.floor(totalSeconds / 3600);
		const minutes = Math.floor((totalSeconds % 3600) / 60);
		const remainingSeconds = totalSeconds % 60;

		if (hours > 0) return `${hours}h ${String(minutes).padStart(2, '0')}m`;
		if (minutes > 0) return `${minutes}m`;
		return `${remainingSeconds}s`;
	}

	function formatBytes(bytes) {
		if (bytes == null) return '--';
		if (bytes === 0) return '0 B';

		const units = ['B', 'KB', 'MB', 'GB', 'TB'];
		let size = bytes;
		let unitIndex = 0;

		while (size >= 1024 && unitIndex < units.length - 1) {
			size /= 1024;
			unitIndex += 1;
		}

		return `${size.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
	}

	function formatSourceName(source) {
		return (source || '')
			.split(/[_-]/g)
			.filter(Boolean)
			.map((part) => part[0].toUpperCase() + part.slice(1))
			.join(' ');
	}

	function getSourceInitials(source) {
		const parts = formatSourceName(source).split(/\s+/).filter(Boolean);
		return parts.slice(0, 2).map((part) => part[0]).join('').toUpperCase() || 'NA';
	}

	function formatSeriesPosition(position) {
		if (position == null) return '';
		return Number.isInteger(position) ? String(position) : String(position).replace(/\.0$/, '');
	}
</script>

<svelte:head>
	<title>{work?.Title || 'Work'} - Codex</title>
</svelte:head>

{#if error}
	<div class="error">{error}</div>
{:else if loading}
	<p class="loading">Loading work details...</p>
{:else if work}
	<div class="work-detail-page">
		<div class="page-topline">
			<a href="/browse" class="back-link">Back to browse</a>
			{#if work.DirectoryPath}
				<span class="directory-path">{work.DirectoryPath}</span>
			{/if}
		</div>

		{#if message}
			<div class="message">{message}</div>
		{/if}

		<section class="hero-card">
			<div class="cover-column">
				<div class="cover-frame">
					{#if selectedCover && !coverLoadFailed}
						<img src={coverUrl} alt={`Cover for ${work.Title}`} class="cover-image" onerror={handleCoverError} />
					{:else}
						<div class="cover-placeholder">
							<span>{work.Title}</span>
						</div>
					{/if}
				</div>

				{#if formatBadges.length}
					<div class="cover-format-stack">
						{#each formatBadges as badge}
							<span class="format-pill" class:primary={badge.isPrimary}>
								{badge.label}
								{#if badge.isPrimary}
									<strong>PRIMARY</strong>
								{/if}
							</span>
						{/each}
					</div>
				{/if}

				{#if work.covers?.length > 1}
					<div class="cover-source-group">
						<span class="group-label">Cover sources</span>
						<div class="cover-source-list">
							{#each work.covers as cover}
								<button
									type="button"
									class="cover-source-button"
									class:selected={cover.IsSelected}
									onclick={() => handleSelectCover(cover.Source)}
								>
									{formatSourceName(cover.Source)}
								</button>
							{/each}
						</div>
					</div>
				{/if}
			</div>

			<div class="hero-content">
				<div class="title-block">
					{#if primarySeries}
						<a href={buildBrowseLink(primarySeries.Name)} class="series-link">
							{primarySeries.Name}
							{#if primarySeries.Position != null}
								<span>#{formatSeriesPosition(primarySeries.Position)}</span>
							{/if}
						</a>
					{/if}

					<h1>{work.Title}</h1>

					{#if work.Subtitle}
						<p class="subtitle">{work.Subtitle}</p>
					{/if}

					{#if sortedContributors.length}
						<div class="contributors-row">
							{#each sortedContributors as contributor}
								<a href={buildBrowseLink(contributor.name)} class="contributor-chip">
									<span>{contributor.name}</span>
									{#if !isRole(contributor.role, ['author', 'writer'])}
										<small>{contributor.role}</small>
									{/if}
								</a>
							{/each}
						</div>
					{:else}
						<p class="muted">No contributors listed yet.</p>
					{/if}
				</div>

				<div class="status-row">
					{#each supportingFacts as fact}
						<span class="status-pill {fact.tone}">{fact.label}</span>
					{/each}
				</div>

				{#if primaryRating}
					<div class="rating-row">
						<div class="stars">
							{#each Array(5) as _, index}
								<span class="star" class:filled={index < filledStars}>★</span>
							{/each}
							<strong>{primaryRating.Score.toFixed(1)}</strong>
							<span class="muted">from {formatSourceName(primaryRating.Source)}</span>
						</div>

						<div class="rating-sources">
							{#each work.ratings as rating}
								<div class="rating-source">
									<span class="source-icon">{getSourceInitials(rating.Source)}</span>
									<div>
										<strong>{rating.Score.toFixed(1)}/{rating.MaxScore}</strong>
										<small>{formatSourceName(rating.Source)}</small>
									</div>
								</div>
							{/each}
						</div>
					</div>
				{/if}

				{#if work.tags?.length}
					<div class="tag-row">
						{#each work.tags as tag}
							<a href={buildBrowseLink(tag.Name)} class="tag-pill">{tag.Name}</a>
						{/each}
					</div>
				{/if}

				<div class="metadata-grid">
					{#each metadataItems as item}
						<div class="meta-cell">
							<span class="meta-label">{item.label}</span>
							<span class="meta-value">{item.value}</span>
						</div>
					{/each}
				</div>

				<div class="action-row">
					<button type="button" class="btn-primary" onclick={handleFetchMetadata}>Fetch Metadata</button>

					{#if primaryFile}
						<a
							class="btn-download"
							href={getWorkFileDownloadUrl(workId, primaryFile.ID)}
						>
							Download
							<span>{downloadSummary}</span>
						</a>
					{:else}
						<button type="button" class="btn-download" disabled>
							Download
							<span>No file available</span>
						</button>
					{/if}

					<div class="menu-wrap">
						<button type="button" class="btn-menu" onclick={() => menuOpen = !menuOpen}>More</button>

						{#if menuOpen}
							<div class="menu-panel">
								<button type="button" onclick={handleFetchMetadata}>Refresh Metadata</button>
								<button type="button" onclick={openEditor}>Edit Metadata</button>
								<button type="button" class="danger-text" onclick={handleDeleteWork}>Delete Work</button>
							</div>
						{/if}
					</div>
				</div>
			</div>
		</section>

		<section class="panel description-panel">
			<div class="panel-header">
				<div>
					<h2>Description</h2>
					<p class="panel-subtitle">Book summary and imported metadata</p>
				</div>
			</div>

			{#if work.Description}
				<div class="description-body">
					{#if work.DescriptionFormat === 'html'}
						{@html DOMPurify.sanitize(work.Description)}
					{:else}
						<p>{work.Description}</p>
					{/if}
				</div>
			{:else}
				<p class="empty-state">No description available. Use Fetch Metadata to pull one in.</p>
			{/if}
		</section>

		{#if editorOpen}
			<section class="panel editor-panel">
				<div class="panel-header">
					<div>
						<h2>Edit Metadata</h2>
						<p class="panel-subtitle">Manual corrections for the fields this page already supports.</p>
					</div>
				</div>

				<div class="editor-grid">
					<label>
						<span>Title</span>
						<input bind:value={editor.title} type="text" />
					</label>

					<label>
						<span>Subtitle</span>
						<input bind:value={editor.subtitle} type="text" />
					</label>

					<label>
						<span>Language</span>
						<input bind:value={editor.language} type="text" />
					</label>

					<label>
						<span>Publisher</span>
						<input bind:value={editor.publisher} type="text" />
					</label>

					<label class="wide">
						<span>Published</span>
						<input bind:value={editor.publishDate} type="text" placeholder="YYYY-MM-DD" />
					</label>

					<label class="wide">
						<span>Description</span>
						<textarea bind:value={editor.description} rows="7"></textarea>
					</label>
				</div>

				<div class="editor-actions">
					<button type="button" class="btn-secondary" onclick={closeEditor} disabled={saving}>Cancel</button>
					<button type="button" class="btn-primary" onclick={saveEditor} disabled={saving}>
						{saving ? 'Saving...' : 'Save Changes'}
					</button>
				</div>
			</section>
		{/if}

		<section class="panel tab-panel">
			<div class="tab-strip">
				<button type="button" class:active={activeTab === 'files'} onclick={() => activeTab = 'files'}>Files</button>
				<button type="button" class:active={activeTab === 'notes'} onclick={() => activeTab = 'notes'}>Notes</button>
				<button type="button" class:active={activeTab === 'similar'} onclick={() => activeTab = 'similar'}>Similar</button>
			</div>

			{#if activeTab === 'files'}
				<div class="file-sections">
					<div class="file-block">
						<div class="block-header">
							<h2>Primary File</h2>
							{#if primaryFile}
								<span class="block-meta">{formatBytes(primaryFile.SizeBytes)}</span>
							{/if}
						</div>

						{#if primaryFile}
							<div class="file-row primary">
								<div class="file-main">
									<span class="file-format primary">{formatLabel(primaryFile.Format)}</span>
									<div>
										<strong>{primaryFile.Filename}</strong>
										<div class="file-details">
											<span>{formatBytes(primaryFile.SizeBytes)}</span>
											{#if primaryFile.DurationSeconds}
												<span>{formatDuration(primaryFile.DurationSeconds)}</span>
											{/if}
											{#if primaryFile.Codec}
												<span>{primaryFile.Codec}</span>
											{/if}
										</div>
									</div>
								</div>

								<div class="file-actions">
									<a href={getWorkFileDownloadUrl(workId, primaryFile.ID)} class="file-action-link">Download</a>
									<button type="button" class="file-action-button" disabled title="File deletion is not available yet">
										Delete
									</button>
								</div>
							</div>
						{:else}
							<p class="empty-state">No files are attached to this work yet.</p>
						{/if}
					</div>

					<div class="file-block">
						<div class="block-header">
							<h2>Alternative Formats</h2>
							<span class="block-meta">{alternateFiles.length} file{alternateFiles.length === 1 ? '' : 's'}</span>
						</div>

						{#if alternateFiles.length}
							<div class="file-list">
								{#each alternateFiles as file}
									<div class="file-row">
										<div class="file-main">
											<span class="file-format">{formatLabel(file.Format)}</span>
											<div>
												<strong>{file.Filename}</strong>
												<div class="file-details">
													<span>{formatBytes(file.SizeBytes)}</span>
													{#if file.DurationSeconds}
														<span>{formatDuration(file.DurationSeconds)}</span>
													{/if}
													{#if file.Codec}
														<span>{file.Codec}</span>
													{/if}
												</div>
											</div>
										</div>

										<div class="file-actions">
											<a href={getWorkFileDownloadUrl(workId, file.ID)} class="file-action-link">Download</a>
											<button type="button" class="file-action-button" disabled title="File deletion is not available yet">
												Delete
											</button>
										</div>
									</div>
								{/each}
							</div>
						{:else}
							<p class="empty-state">No alternate formats yet.</p>
						{/if}
					</div>

					{#if work.chapters?.length}
						<div class="file-block">
							<div class="block-header">
								<h2>Chapters</h2>
								<span class="block-meta">{work.chapters.length}</span>
							</div>

							<div class="chapter-table">
								<div class="chapter-head">
									<span>#</span>
									<span>Title</span>
									<span>Start</span>
									<span>End</span>
								</div>

								{#each work.chapters as chapter}
									<div class="chapter-row">
										<span>{chapter.IndexPosition + 1}</span>
										<span>{chapter.Title}</span>
										<span>{formatDuration(chapter.StartSeconds)}</span>
										<span>{formatDuration(chapter.EndSeconds)}</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			{:else if activeTab === 'notes'}
				<p class="empty-state">Notes are planned for a later pass.</p>
			{:else}
				<p class="empty-state">Similar works are planned for a later pass.</p>
			{/if}
		</section>

		{#if tasks.length}
			<details class="panel tasks-panel">
				<summary>
					<div>
						<h2>Metadata Tasks</h2>
						<p class="panel-subtitle">Background jobs and review history</p>
					</div>
					<span>{tasks.length}</span>
				</summary>

				<div class="task-table">
					<div class="task-head">
						<span>Type</span>
						<span>Status</span>
						<span>Created</span>
					</div>

					{#each tasks as task}
						<div class="task-row">
							<span>{task.TaskType}</span>
							<span class="task-status {task.Status}">{task.Status}</span>
							<span>{new Date(task.CreatedAt).toLocaleString()}</span>
						</div>
					{/each}
				</div>
			</details>
		{/if}
	</div>
{/if}

<style>
	.work-detail-page {
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
	}

	.page-topline {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 1rem;
		flex-wrap: wrap;
	}

	.back-link {
		font-size: 0.9rem;
		color: var(--text-muted);
	}

	.directory-path {
		font-size: 0.8rem;
		color: var(--text-muted);
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid var(--border);
		border-radius: 999px;
		padding: 0.35rem 0.75rem;
	}

	.hero-card,
	.panel {
		background:
			linear-gradient(180deg, rgba(108, 140, 255, 0.08), rgba(108, 140, 255, 0) 140px),
			var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: 18px;
		box-shadow: 0 18px 40px rgba(0, 0, 0, 0.28);
	}

	.hero-card {
		display: grid;
		grid-template-columns: minmax(220px, 260px) minmax(0, 1fr);
		gap: 1.5rem;
		padding: 1.5rem;
	}

	.cover-column {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.cover-frame {
		background: #11151f;
		border: 1px solid rgba(255, 255, 255, 0.06);
		border-radius: 20px;
		padding: 0.85rem;
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
	}

	.cover-image,
	.cover-placeholder {
		display: block;
		width: 100%;
		aspect-ratio: 2 / 3;
		border-radius: 14px;
	}

	.cover-image {
		object-fit: cover;
		box-shadow: 0 18px 36px rgba(0, 0, 0, 0.35);
	}

	.cover-placeholder {
		display: grid;
		place-items: center;
		padding: 1rem;
		text-align: center;
		font-size: 1.2rem;
		font-weight: 700;
		line-height: 1.25;
		color: var(--text);
		background:
			radial-gradient(circle at top, rgba(87, 199, 190, 0.35), transparent 45%),
			linear-gradient(160deg, rgba(108, 140, 255, 0.35), rgba(26, 31, 46, 0.92));
	}

	.cover-format-stack,
	.cover-source-list,
	.status-row,
	.tag-row,
	.rating-sources {
		display: flex;
		flex-wrap: wrap;
		gap: 0.55rem;
	}

	.format-pill,
	.status-pill,
	.tag-pill,
	.cover-source-button {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		border-radius: 999px;
		padding: 0.4rem 0.75rem;
		font-size: 0.78rem;
		font-weight: 700;
		letter-spacing: 0.03em;
	}

	.format-pill {
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(255, 255, 255, 0.07);
		color: var(--text);
	}

	.format-pill.primary {
		background: rgba(108, 140, 255, 0.18);
		border-color: rgba(108, 140, 255, 0.4);
	}

	.format-pill strong {
		font-size: 0.67rem;
		color: #c7d2ff;
	}

	.group-label,
	.meta-label,
	.panel-subtitle,
	.muted,
	.file-details,
	.directory-path,
	.chapter-head,
	.task-head {
		color: var(--text-muted);
	}

	.cover-source-group {
		display: flex;
		flex-direction: column;
		gap: 0.45rem;
	}

	.cover-source-button {
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid var(--border);
		color: var(--text-muted);
		cursor: pointer;
	}

	.cover-source-button.selected {
		background: rgba(34, 197, 94, 0.14);
		border-color: rgba(34, 197, 94, 0.35);
		color: var(--text);
	}

	.hero-content {
		display: flex;
		flex-direction: column;
		gap: 1.1rem;
		min-width: 0;
	}

	.title-block {
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
	}

	.series-link {
		font-size: 0.88rem;
		font-weight: 600;
		color: #87d7d2;
	}

	h1 {
		font-size: clamp(2rem, 3.5vw, 3rem);
		line-height: 1.05;
	}

	.subtitle {
		font-size: 1rem;
		color: var(--text-muted);
		max-width: 65ch;
	}

	.contributors-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.55rem;
	}

	.contributor-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		padding: 0.45rem 0.8rem;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(255, 255, 255, 0.07);
		color: var(--text);
	}

	.contributor-chip small {
		color: #9bb2ff;
		text-transform: uppercase;
		font-size: 0.65rem;
		letter-spacing: 0.04em;
	}

	.status-pill {
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(255, 255, 255, 0.06);
		color: var(--text-muted);
	}

	.status-pill.warning {
		background: rgba(245, 158, 11, 0.16);
		border-color: rgba(245, 158, 11, 0.32);
		color: #ffd08a;
	}

	.status-pill.success {
		background: rgba(34, 197, 94, 0.16);
		border-color: rgba(34, 197, 94, 0.32);
		color: #8ee3b2;
	}

	.rating-row {
		display: grid;
		gap: 0.9rem;
		padding: 1rem 1.1rem;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid rgba(255, 255, 255, 0.05);
		border-radius: 16px;
	}

	.stars {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		flex-wrap: wrap;
	}

	.star {
		color: rgba(255, 255, 255, 0.18);
		font-size: 1rem;
	}

	.star.filled {
		color: #f6c65b;
	}

	.rating-source {
		display: inline-flex;
		align-items: center;
		gap: 0.55rem;
		padding: 0.5rem 0.7rem;
		border-radius: 14px;
		background: rgba(15, 18, 25, 0.6);
		border: 1px solid rgba(255, 255, 255, 0.05);
	}

	.rating-source small {
		display: block;
		color: var(--text-muted);
		font-size: 0.72rem;
	}

	.source-icon {
		display: inline-grid;
		place-items: center;
		width: 2rem;
		height: 2rem;
		border-radius: 50%;
		background: rgba(108, 140, 255, 0.22);
		color: #d7e0ff;
		font-size: 0.72rem;
		font-weight: 800;
	}

	.tag-pill {
		background: rgba(87, 199, 190, 0.14);
		border: 1px solid rgba(87, 199, 190, 0.28);
		color: #a7ece6;
	}

	.metadata-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
		gap: 0.8rem;
	}

	.meta-cell {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		padding: 0.9rem 1rem;
		border-radius: 14px;
		background: rgba(15, 18, 25, 0.58);
		border: 1px solid rgba(255, 255, 255, 0.05);
		min-height: 82px;
	}

	.meta-label {
		font-size: 0.72rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	.meta-value {
		font-size: 0.96rem;
		font-weight: 600;
		line-height: 1.35;
		word-break: break-word;
	}

	.action-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		align-items: center;
	}

	button,
	.btn-download,
	.file-action-link {
		border: none;
		border-radius: 999px;
		cursor: pointer;
		font-weight: 700;
		font-size: 0.9rem;
		text-decoration: none;
	}

	.btn-primary,
	.btn-download,
	.btn-secondary,
	.btn-menu {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.45rem;
		padding: 0.8rem 1.1rem;
	}

	.btn-primary {
		background: linear-gradient(135deg, #6c8cff, #89a4ff);
		color: white;
		box-shadow: 0 10px 20px rgba(108, 140, 255, 0.24);
	}

	.btn-primary:hover:not(:disabled) {
		filter: brightness(1.05);
	}

	.btn-download {
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(255, 255, 255, 0.08);
		color: var(--text);
	}

	.btn-download span {
		color: var(--text-muted);
		font-size: 0.78rem;
	}

	.btn-download:disabled,
	button:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}

	.btn-secondary,
	.btn-menu,
	.file-action-button {
		background: transparent;
		border: 1px solid var(--border);
		color: var(--text);
	}

	.btn-secondary:hover:not(:disabled),
	.btn-menu:hover:not(:disabled),
	.file-action-button:hover:not(:disabled) {
		border-color: rgba(108, 140, 255, 0.5);
	}

	.menu-wrap {
		position: relative;
	}

	.menu-panel {
		position: absolute;
		right: 0;
		top: calc(100% + 0.5rem);
		min-width: 190px;
		display: flex;
		flex-direction: column;
		padding: 0.4rem;
		background: #121724;
		border: 1px solid var(--border);
		border-radius: 14px;
		box-shadow: 0 16px 28px rgba(0, 0, 0, 0.32);
		z-index: 10;
	}

	.menu-panel button {
		background: transparent;
		color: var(--text);
		text-align: left;
		padding: 0.75rem 0.85rem;
		border-radius: 10px;
	}

	.menu-panel button:hover {
		background: rgba(255, 255, 255, 0.05);
	}

	.menu-panel .danger-text {
		color: #ff9f9f;
	}

	.panel {
		padding: 1.3rem 1.4rem;
	}

	.panel-header,
	.block-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.panel h2,
	.block-header h2 {
		font-size: 1.05rem;
		margin-bottom: 0.15rem;
	}

	.description-body {
		color: var(--text);
		line-height: 1.7;
	}

	.description-body :global(p + p) {
		margin-top: 1rem;
	}

	.empty-state {
		color: var(--text-muted);
	}

	.editor-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.9rem;
	}

	.editor-grid label {
		display: flex;
		flex-direction: column;
		gap: 0.45rem;
	}

	.editor-grid label.wide {
		grid-column: 1 / -1;
	}

	input,
	textarea {
		width: 100%;
		background: rgba(15, 18, 25, 0.7);
		border: 1px solid var(--border);
		border-radius: 12px;
		color: var(--text);
		padding: 0.75rem 0.9rem;
		font: inherit;
	}

	input:focus,
	textarea:focus {
		outline: none;
		border-color: rgba(108, 140, 255, 0.65);
		box-shadow: 0 0 0 3px rgba(108, 140, 255, 0.14);
	}

	textarea {
		resize: vertical;
	}

	.editor-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.65rem;
		margin-top: 1rem;
	}

	.tab-strip {
		display: flex;
		gap: 0.6rem;
		margin-bottom: 1.1rem;
		flex-wrap: wrap;
	}

	.tab-strip button {
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid var(--border);
		color: var(--text-muted);
		padding: 0.55rem 0.95rem;
	}

	.tab-strip button.active {
		background: rgba(108, 140, 255, 0.16);
		border-color: rgba(108, 140, 255, 0.42);
		color: var(--text);
	}

	.file-sections {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.file-block {
		padding: 1rem;
		border-radius: 16px;
		background: rgba(15, 18, 25, 0.56);
		border: 1px solid rgba(255, 255, 255, 0.05);
	}

	.block-meta {
		color: var(--text-muted);
		font-size: 0.82rem;
	}

	.file-list {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.file-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 1rem;
		padding: 0.9rem 1rem;
		border-radius: 14px;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid rgba(255, 255, 255, 0.05);
	}

	.file-row.primary {
		background: rgba(108, 140, 255, 0.09);
		border-color: rgba(108, 140, 255, 0.22);
	}

	.file-main {
		display: flex;
		align-items: center;
		gap: 0.95rem;
		min-width: 0;
	}

	.file-main strong {
		display: block;
		word-break: break-word;
	}

	.file-format {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 4.1rem;
		padding: 0.55rem 0.75rem;
		border-radius: 12px;
		background: rgba(255, 255, 255, 0.06);
		font-size: 0.78rem;
		font-weight: 800;
	}

	.file-format.primary {
		background: rgba(108, 140, 255, 0.2);
		color: #dce4ff;
	}

	.file-details {
		display: flex;
		flex-wrap: wrap;
		gap: 0.7rem;
		margin-top: 0.3rem;
		font-size: 0.82rem;
	}

	.file-actions {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		flex-shrink: 0;
	}

	.file-action-link,
	.file-action-button {
		padding: 0.6rem 0.9rem;
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(255, 255, 255, 0.08);
		color: var(--text);
	}

	.chapter-table,
	.task-table {
		display: flex;
		flex-direction: column;
		border-radius: 14px;
		overflow: hidden;
		border: 1px solid rgba(255, 255, 255, 0.05);
	}

	.chapter-head,
	.chapter-row,
	.task-head,
	.task-row {
		display: grid;
		grid-template-columns: 64px minmax(0, 1fr) 120px 120px;
		gap: 0.75rem;
		padding: 0.8rem 0.95rem;
		align-items: center;
	}

	.chapter-head,
	.task-head {
		background: rgba(255, 255, 255, 0.04);
		text-transform: uppercase;
		font-size: 0.72rem;
		letter-spacing: 0.08em;
	}

	.chapter-row,
	.task-row {
		background: rgba(15, 18, 25, 0.4);
		border-top: 1px solid rgba(255, 255, 255, 0.04);
	}

	.tasks-panel summary {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 1rem;
		cursor: pointer;
		list-style: none;
	}

	.tasks-panel summary::-webkit-details-marker {
		display: none;
	}

	.tasks-panel summary span {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 2rem;
		height: 2rem;
		padding: 0 0.75rem;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.05);
		color: var(--text-muted);
	}

	.tasks-panel[open] summary {
		margin-bottom: 1rem;
	}

	.task-head,
	.task-row {
		grid-template-columns: 1fr 160px 220px;
	}

	.task-status {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: fit-content;
		padding: 0.35rem 0.7rem;
		border-radius: 999px;
		text-transform: capitalize;
		font-size: 0.78rem;
		font-weight: 700;
	}

	.task-status.completed {
		background: rgba(34, 197, 94, 0.16);
		color: #8ee3b2;
	}

	.task-status.review {
		background: rgba(245, 158, 11, 0.16);
		color: #ffd08a;
	}

	.task-status.failed {
		background: rgba(239, 68, 68, 0.16);
		color: #ffb2b2;
	}

	.message,
	.error {
		padding: 0.85rem 1rem;
		border-radius: 14px;
		font-size: 0.9rem;
	}

	.message {
		background: rgba(108, 140, 255, 0.12);
		border: 1px solid rgba(108, 140, 255, 0.32);
	}

	.error {
		background: rgba(239, 68, 68, 0.12);
		border: 1px solid rgba(239, 68, 68, 0.34);
		color: #ffb2b2;
	}

	.loading {
		color: var(--text-muted);
	}

	@media (max-width: 1100px) {
		.hero-card {
			grid-template-columns: 1fr;
		}

		.cover-column {
			max-width: 320px;
		}
	}

	@media (max-width: 860px) {
		.editor-grid,
		.metadata-grid {
			grid-template-columns: 1fr;
		}

		.file-row,
		.chapter-head,
		.chapter-row,
		.task-head,
		.task-row {
			grid-template-columns: 1fr;
		}

		.file-row {
			align-items: flex-start;
		}

		.file-actions {
			width: 100%;
			justify-content: flex-start;
		}
	}

	@media (max-width: 640px) {
		.hero-card,
		.panel {
			padding: 1rem;
		}

		.action-row,
		.editor-actions {
			flex-direction: column;
			align-items: stretch;
		}

		.btn-primary,
		.btn-download,
		.btn-secondary,
		.btn-menu {
			width: 100%;
		}

		.menu-wrap {
			width: 100%;
		}

		.menu-panel {
			left: 0;
			right: auto;
			width: 100%;
		}
	}
</style>
