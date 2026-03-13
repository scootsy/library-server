<script>
	import { login } from '$lib/api/client.js';

	let username = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleSubmit(e) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			const result = await login(username, password);
			// Store user info
			window.__codex_user = result.user;
			// Navigate to dashboard
			window.location.href = '/';
		} catch (err) {
			error = err.message || 'Login failed';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Codex - Login</title>
</svelte:head>

<div class="login-page">
	<div class="login-card">
		<h1 class="logo">Codex</h1>
		<p class="subtitle">Library Server</p>

		<form onsubmit={handleSubmit}>
			{#if error}
				<div class="error-msg">{error}</div>
			{/if}

			<div class="field">
				<label for="username">Username</label>
				<input
					id="username"
					type="text"
					bind:value={username}
					autocomplete="username"
					required
					disabled={loading}
				/>
			</div>

			<div class="field">
				<label for="password">Password</label>
				<input
					id="password"
					type="password"
					bind:value={password}
					autocomplete="current-password"
					required
					disabled={loading}
				/>
			</div>

			<button type="submit" class="btn-login" disabled={loading}>
				{loading ? 'Signing in...' : 'Sign in'}
			</button>
		</form>
	</div>
</div>

<style>
	.login-page {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
		background: var(--bg);
	}

	.login-card {
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 2.5rem;
		width: 100%;
		max-width: 380px;
	}

	.logo {
		font-size: 1.75rem;
		color: var(--primary);
		text-align: center;
		margin-bottom: 0.25rem;
	}

	.subtitle {
		text-align: center;
		color: var(--text-muted);
		margin-bottom: 2rem;
		font-size: 0.9rem;
	}

	.field {
		margin-bottom: 1rem;
	}

	.field label {
		display: block;
		margin-bottom: 0.35rem;
		color: var(--text-muted);
		font-size: 0.85rem;
	}

	.field input {
		width: 100%;
		padding: 0.6rem 0.75rem;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		color: var(--text);
		font-size: 0.95rem;
	}

	.field input:focus {
		outline: none;
		border-color: var(--primary);
	}

	.btn-login {
		width: 100%;
		padding: 0.65rem;
		margin-top: 0.5rem;
		background: var(--primary);
		color: #fff;
		border: none;
		border-radius: var(--radius);
		font-size: 0.95rem;
		cursor: pointer;
		transition: background 0.15s;
	}

	.btn-login:hover:not(:disabled) {
		background: var(--primary-hover);
	}

	.btn-login:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.error-msg {
		background: rgba(239, 68, 68, 0.1);
		border: 1px solid var(--danger);
		color: var(--danger);
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius);
		margin-bottom: 1rem;
		font-size: 0.85rem;
	}
</style>
