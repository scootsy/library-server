<script>
	let { children } = $props();
</script>

<svelte:head>
	<style>
		:root {
			--bg: #0f1117;
			--bg-surface: #1a1d27;
			--bg-hover: #232736;
			--border: #2a2e3d;
			--text: #e4e5e9;
			--text-muted: #8b8fa3;
			--primary: #6c8cff;
			--primary-hover: #8ba3ff;
			--danger: #ef4444;
			--success: #22c55e;
			--warning: #f59e0b;
			--radius: 6px;
		}

		* { box-sizing: border-box; margin: 0; padding: 0; }

		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
			background: var(--bg);
			color: var(--text);
			line-height: 1.5;
		}

		a { color: var(--primary); text-decoration: none; }
		a:hover { color: var(--primary-hover); }
	</style>
</svelte:head>

<div class="layout">
	<nav class="sidebar">
		<div class="logo">
			<h1>Codex</h1>
		</div>
		<ul class="nav-links">
			<li><a href="/">Dashboard</a></li>
			<li><a href="/browse">Browse</a></li>
			<li><a href="/review">Review Queue</a></li>
			<li><a href="/collections">Collections</a></li>
			<li><a href="/settings">Settings</a></li>
		</ul>
	</nav>
	<main class="content">
		{@render children()}
	</main>
</div>

<style>
	.layout {
		display: flex;
		min-height: 100vh;
	}

	.sidebar {
		width: 220px;
		background: var(--bg-surface);
		border-right: 1px solid var(--border);
		padding: 1rem;
		flex-shrink: 0;
	}

	.logo h1 {
		font-size: 1.25rem;
		margin-bottom: 1.5rem;
		color: var(--primary);
	}

	.nav-links {
		list-style: none;
	}

	.nav-links li {
		margin-bottom: 0.25rem;
	}

	.nav-links a {
		display: block;
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius);
		color: var(--text-muted);
		transition: all 0.15s;
	}

	.nav-links a:hover {
		background: var(--bg-hover);
		color: var(--text);
	}

	.content {
		flex: 1;
		padding: 1.5rem 2rem;
		overflow-y: auto;
	}
</style>
