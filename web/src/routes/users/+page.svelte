<script>
	import { listUsers, createUser, updateUser, deleteUser } from '$lib/api/client.js';

	let users = $state([]);
	let loading = $state(true);
	let error = $state('');

	let showCreate = $state(false);
	let editingUser = $state(null);
	let formData = $state({ username: '', display_name: '', email: '', password: '', role: 'user' });
	let formError = $state('');
	let formLoading = $state(false);

	async function loadUsers() {
		try {
			const result = await listUsers();
			users = result.data || [];
		} catch (err) {
			error = err.message;
		} finally {
			loading = false;
		}
	}

	$effect(() => { loadUsers(); });

	function openCreate() {
		formData = { username: '', display_name: '', email: '', password: '', role: 'user' };
		formError = '';
		editingUser = null;
		showCreate = true;
	}

	function openEdit(user) {
		formData = {
			username: user.username,
			display_name: user.display_name || '',
			email: user.email || '',
			password: '',
			role: user.role
		};
		formError = '';
		editingUser = user;
		showCreate = true;
	}

	function closeForm() {
		showCreate = false;
		editingUser = null;
	}

	async function handleSubmit(e) {
		e.preventDefault();
		formError = '';
		formLoading = true;

		try {
			if (editingUser) {
				const data = { ...formData };
				if (!data.password) delete data.password;
				await updateUser(editingUser.id, data);
			} else {
				if (!formData.password) {
					formError = 'Password is required for new users';
					formLoading = false;
					return;
				}
				await createUser(formData);
			}
			closeForm();
			await loadUsers();
		} catch (err) {
			formError = err.message;
		} finally {
			formLoading = false;
		}
	}

	async function handleDelete(user) {
		if (!confirm(`Delete user "${user.username}"? This cannot be undone.`)) return;
		try {
			await deleteUser(user.id);
			await loadUsers();
		} catch (err) {
			error = err.message;
		}
	}
</script>

<svelte:head>
	<title>Codex - Users</title>
</svelte:head>

<div class="page">
	<div class="page-header">
		<h2>User Management</h2>
		<button class="btn-primary" onclick={openCreate}>Add User</button>
	</div>

	{#if error}
		<div class="error-msg">{error}</div>
	{/if}

	{#if loading}
		<p class="muted">Loading users...</p>
	{:else if users.length === 0}
		<p class="muted">No users found.</p>
	{:else}
		<table class="users-table">
			<thead>
				<tr>
					<th>Username</th>
					<th>Display Name</th>
					<th>Email</th>
					<th>Role</th>
					<th>Status</th>
					<th>Last Login</th>
					<th>Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each users as user}
					<tr>
						<td>{user.username}</td>
						<td>{user.display_name || '—'}</td>
						<td>{user.email || '—'}</td>
						<td><span class="role-badge role-{user.role}">{user.role}</span></td>
						<td>
							<span class="status-dot" class:active={user.is_active} class:inactive={!user.is_active}></span>
							{user.is_active ? 'Active' : 'Disabled'}
						</td>
						<td class="muted">{user.last_login_at ? new Date(user.last_login_at).toLocaleDateString() : 'Never'}</td>
						<td>
							<button class="btn-sm" onclick={() => openEdit(user)}>Edit</button>
							<button class="btn-sm btn-danger" onclick={() => handleDelete(user)}>Delete</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}

	{#if showCreate}
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="modal-overlay" onclick={closeForm} role="presentation">
			<div class="modal" onclick={(e) => e.stopPropagation()} role="dialog">
				<h3>{editingUser ? 'Edit User' : 'Create User'}</h3>

				{#if formError}
					<div class="error-msg">{formError}</div>
				{/if}

				<form onsubmit={handleSubmit}>
					<div class="field">
						<label for="f-username">Username</label>
						<input id="f-username" type="text" bind:value={formData.username} required disabled={formLoading} />
					</div>
					<div class="field">
						<label for="f-display">Display Name</label>
						<input id="f-display" type="text" bind:value={formData.display_name} disabled={formLoading} />
					</div>
					<div class="field">
						<label for="f-email">Email</label>
						<input id="f-email" type="email" bind:value={formData.email} disabled={formLoading} />
					</div>
					<div class="field">
						<label for="f-password">{editingUser ? 'New Password (leave blank to keep)' : 'Password'}</label>
						<input id="f-password" type="password" bind:value={formData.password} disabled={formLoading}
							required={!editingUser} autocomplete="new-password" />
					</div>
					<div class="field">
						<label for="f-role">Role</label>
						<select id="f-role" bind:value={formData.role} disabled={formLoading}>
							<option value="admin">Admin</option>
							<option value="user">User</option>
							<option value="guest">Guest</option>
						</select>
					</div>
					<div class="form-actions">
						<button type="button" class="btn-secondary" onclick={closeForm} disabled={formLoading}>Cancel</button>
						<button type="submit" class="btn-primary" disabled={formLoading}>
							{formLoading ? 'Saving...' : (editingUser ? 'Update' : 'Create')}
						</button>
					</div>
				</form>
			</div>
		</div>
	{/if}
</div>

<style>
	.page { max-width: 960px; }

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.5rem;
	}

	.page-header h2 { font-size: 1.25rem; }

	.muted { color: var(--text-muted); font-size: 0.9rem; }

	.users-table {
		width: 100%;
		border-collapse: collapse;
	}

	.users-table th,
	.users-table td {
		padding: 0.6rem 0.75rem;
		text-align: left;
		border-bottom: 1px solid var(--border);
		font-size: 0.85rem;
	}

	.users-table th {
		color: var(--text-muted);
		font-weight: 500;
		font-size: 0.8rem;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.role-badge {
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
		font-size: 0.75rem;
		text-transform: capitalize;
	}

	.role-admin { background: rgba(108, 140, 255, 0.15); color: var(--primary); }
	.role-user { background: rgba(34, 197, 94, 0.15); color: var(--success); }
	.role-guest { background: rgba(245, 158, 11, 0.15); color: var(--warning); }

	.status-dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		margin-right: 0.3rem;
	}

	.status-dot.active { background: var(--success); }
	.status-dot.inactive { background: var(--danger); }

	.btn-primary {
		padding: 0.45rem 1rem;
		background: var(--primary);
		color: #fff;
		border: none;
		border-radius: var(--radius);
		cursor: pointer;
		font-size: 0.85rem;
	}

	.btn-primary:hover:not(:disabled) { background: var(--primary-hover); }
	.btn-primary:disabled { opacity: 0.6; cursor: not-allowed; }

	.btn-secondary {
		padding: 0.45rem 1rem;
		background: transparent;
		color: var(--text-muted);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		cursor: pointer;
		font-size: 0.85rem;
	}

	.btn-secondary:hover { background: var(--bg-hover); }

	.btn-sm {
		padding: 0.25rem 0.6rem;
		background: transparent;
		color: var(--text-muted);
		border: 1px solid var(--border);
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.78rem;
		margin-right: 0.25rem;
	}

	.btn-sm:hover { background: var(--bg-hover); color: var(--text); }

	.btn-danger { border-color: var(--danger); color: var(--danger); }
	.btn-danger:hover { background: rgba(239, 68, 68, 0.1); color: var(--danger); }

	.error-msg {
		background: rgba(239, 68, 68, 0.1);
		border: 1px solid var(--danger);
		color: var(--danger);
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius);
		margin-bottom: 1rem;
		font-size: 0.85rem;
	}

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.6);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 100;
	}

	.modal {
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 1.5rem;
		width: 100%;
		max-width: 420px;
	}

	.modal h3 { margin-bottom: 1rem; font-size: 1.1rem; }

	.field {
		margin-bottom: 0.85rem;
	}

	.field label {
		display: block;
		margin-bottom: 0.3rem;
		color: var(--text-muted);
		font-size: 0.8rem;
	}

	.field input,
	.field select {
		width: 100%;
		padding: 0.5rem 0.65rem;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		color: var(--text);
		font-size: 0.9rem;
	}

	.field input:focus,
	.field select:focus {
		outline: none;
		border-color: var(--primary);
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 1rem;
	}
</style>
