<script>
	import { getCurrentUser, logout } from '$lib/api/client.js';
	import { onMount } from 'svelte';

	let { children } = $props();

	let user = $state(null);
	let authChecked = $state(false);
	let isLoginPage = $state(false);
	let authError = $state('');
	let redirectedToLogin = $state(false);

	onMount(() => {
		isLoginPage = window.location.pathname === '/login';

		if (isLoginPage) {
			authChecked = true;
			return;
		}

		getCurrentUser()
			.then((u) => {
				user = u;
				authChecked = true;
			})
			.catch((err) => {
				authError = err?.message || 'authentication required';
				authChecked = true;
				redirectedToLogin = true;
				window.location.href = '/login';
			});
	});

	async function handleLogout() {
		try {
			await logout();
		} catch {
			// ignore errors during logout
		}
		window.location.href = '/login';
	}
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

{#if !authChecked}
	<div class="loading-screen">
		<div class="loading-spinner"></div>
	</div>
{:else if isLoginPage}
	{@render children()}
{:else if user}
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
				{#if user.role === 'admin'}
					<li><a href="/users">Users</a></li>
				{/if}
				<li><a href="/settings">Settings</a></li>
			</ul>
			<div class="sidebar-footer">
				<div class="user-info">
					<span class="user-name">{user.display_name || user.username}</span>
					<span class="user-role">{user.role}</span>
				</div>
				<button class="btn-logout" onclick={handleLogout}>Sign out</button>
			</div>
		</nav>
		<main class="content">
			{@render children()}
		</main>
	</div>
{:else}
	<div class="unauthenticated-screen">
		<p>Authentication required. Redirecting to login…</p>
		{#if authError}
			<p class="auth-error">{authError}</p>
		{/if}
		{#if redirectedToLogin}
			<p><a href="/login">Go to login</a></p>
		{/if}
	</div>
{/if}

<style>
	.loading-screen {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
	}

	.loading-spinner {
		width: 32px;
		height: 32px;
		border: 3px solid var(--border);
		border-top-color: var(--primary);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
	}

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
		display: flex;
		flex-direction: column;
	}

	.logo h1 {
		font-size: 1.25rem;
		margin-bottom: 1.5rem;
		color: var(--primary);
	}

	.nav-links {
		list-style: none;
		flex: 1;
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

	.sidebar-footer {
		border-top: 1px solid var(--border);
		padding-top: 0.75rem;
		margin-top: 0.75rem;
	}

	.user-info {
		display: flex;
		flex-direction: column;
		margin-bottom: 0.5rem;
	}

	.user-name {
		font-size: 0.85rem;
		color: var(--text);
	}

	.user-role {
		font-size: 0.75rem;
		color: var(--text-muted);
		text-transform: capitalize;
	}

	.btn-logout {
		width: 100%;
		padding: 0.4rem;
		background: transparent;
		border: 1px solid var(--border);
		border-radius: var(--radius);
		color: var(--text-muted);
		font-size: 0.8rem;
		cursor: pointer;
		transition: all 0.15s;
	}

	.btn-logout:hover {
		background: var(--bg-hover);
		color: var(--text);
		border-color: var(--text-muted);
	}

	.content {
		flex: 1;
		padding: 1.5rem 2rem;
		overflow-y: auto;
	}

	.unauthenticated-screen {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
		gap: 0.5rem;
		color: var(--text-muted);
	}

	.auth-error {
		color: var(--danger);
	}
</style>
