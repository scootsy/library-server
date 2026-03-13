<script>
	import { getSettings, updateSettings, triggerScan, getScanStatus } from '$lib/api/client.js';
	import { onMount } from 'svelte';

	let settings = $state({});
	let scanStatus = $state(null);
	let loading = $state(true);
	let error = $state(null);
	let message = $state('');
	let newKey = $state('');
	let newValue = $state('');

	onMount(async () => {
		await loadSettings();
		await loadScanStatus();
	});

	async function loadSettings() {
		loading = true;
		try {
			const result = await getSettings();
			settings = result.data || {};
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	async function loadScanStatus() {
		try {
			scanStatus = await getScanStatus();
		} catch (e) {
			// Non-critical.
		}
	}

	async function handleSave(key, value) {
		try {
			await updateSettings({ [key]: value });
			message = `Setting "${key}" updated`;
			setTimeout(() => message = '', 3000);
		} catch (e) {
			message = 'Error: ' + e.message;
		}
	}

	async function handleAdd() {
		if (!newKey.trim()) return;
		try {
			await updateSettings({ [newKey.trim()]: newValue });
			settings[newKey.trim()] = newValue;
			newKey = '';
			newValue = '';
			message = 'Setting added';
			await loadSettings();
			setTimeout(() => message = '', 3000);
		} catch (e) {
			message = 'Error: ' + e.message;
		}
	}

	async function handleScan() {
		try {
			await triggerScan();
			message = 'Scan started';
			setTimeout(() => { message = ''; loadScanStatus(); }, 3000);
		} catch (e) {
			message = 'Error: ' + e.message;
		}
	}
</script>

<svelte:head>
	<title>Settings - Codex</title>
</svelte:head>

<h2>Settings</h2>

{#if message}
	<div class="message">{message}</div>
{/if}

<section class="section">
	<h3>Library Scan</h3>
	<div class="scan-controls">
		<button onclick={handleScan}>Trigger Full Scan</button>
		{#if scanStatus}
			<div class="scan-info">
				<span>Status: <strong>{scanStatus.status}</strong></span>
				{#if scanStatus.last_run}
					<span>Last run: {new Date(scanStatus.last_run).toLocaleString()}</span>
				{/if}
				{#if scanStatus.last_error}
					<span class="error-text">Error: {scanStatus.last_error}</span>
				{/if}
			</div>
		{/if}
	</div>
</section>

<section class="section">
	<h3>Application Settings</h3>

	{#if error}
		<div class="error">{error}</div>
	{:else if loading}
		<p class="loading">Loading...</p>
	{:else}
		{#if Object.keys(settings).length}
			<table class="table">
				<thead><tr><th>Key</th><th>Value</th><th>Actions</th></tr></thead>
				<tbody>
					{#each Object.entries(settings) as [key, value]}
						<tr>
							<td class="mono">{key}</td>
							<td>
								<input
									type="text"
									value={value}
									onchange={(e) => handleSave(key, e.target.value)}
								/>
							</td>
							<td>
								<button class="btn-small" onclick={() => handleSave(key, settings[key])}>Save</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{:else}
			<p class="text-muted">No settings configured.</p>
		{/if}

		<div class="add-setting">
			<input type="text" placeholder="Key" bind:value={newKey} />
			<input type="text" placeholder="Value" bind:value={newValue} />
			<button onclick={handleAdd}>Add Setting</button>
		</div>
	{/if}
</section>

<style>
	h2 { margin-bottom: 1.5rem; }
	h3 { margin-bottom: 0.75rem; font-size: 1rem; color: var(--text-muted); }

	.section { margin-bottom: 2rem; }

	.scan-controls {
		display: flex;
		align-items: center;
		gap: 1.5rem;
	}

	.scan-info {
		display: flex;
		gap: 1rem;
		font-size: 0.85rem;
		color: var(--text-muted);
	}

	.error-text { color: var(--danger); }

	.table {
		width: 100%;
		border-collapse: collapse;
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		margin-bottom: 1rem;
	}

	.table th, .table td {
		padding: 0.5rem 0.75rem;
		text-align: left;
		border-bottom: 1px solid var(--border);
	}

	.table th { font-size: 0.8rem; color: var(--text-muted); text-transform: uppercase; }

	input {
		background: var(--bg);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.35rem 0.5rem;
		border-radius: var(--radius);
		font-size: 0.85rem;
		width: 100%;
	}

	.add-setting {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	.add-setting input { width: auto; flex: 1; }

	button {
		background: var(--primary);
		color: white;
		border: none;
		padding: 0.5rem 1rem;
		border-radius: var(--radius);
		cursor: pointer;
		font-size: 0.9rem;
		white-space: nowrap;
	}

	button:hover { background: var(--primary-hover); }

	.btn-small { font-size: 0.75rem; padding: 0.25rem 0.5rem; }

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
