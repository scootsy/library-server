/**
 * API client for the Codex REST API.
 * All functions return promises that resolve to JSON data.
 */

const BASE = '/api';

async function request(path, options = {}) {
	const url = `${BASE}${path}`;
	const res = await fetch(url, {
		headers: { 'Content-Type': 'application/json', ...options.headers },
		...options
	});

	if (!res.ok) {
		const body = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error(body.error || `HTTP ${res.status}`);
	}

	return res.json();
}

// ── Auth ────────────────────────────────────────────────────────────────────
export function login(username, password) {
	return request('/auth/login', {
		method: 'POST',
		body: JSON.stringify({ username, password })
	});
}

export function logout() {
	return request('/auth/logout', { method: 'POST' });
}

export function getCurrentUser() {
	return request('/auth/me');
}

export function changePassword(currentPassword, newPassword) {
	return request('/auth/password', {
		method: 'PUT',
		body: JSON.stringify({ current_password: currentPassword, new_password: newPassword })
	});
}

// ── Admin: Users ────────────────────────────────────────────────────────────
export function listUsers() {
	return request('/admin/users');
}

export function getUser(id) {
	return request(`/admin/users/${id}`);
}

export function createUser(data) {
	return request('/admin/users', {
		method: 'POST',
		body: JSON.stringify(data)
	});
}

export function updateUser(id, data) {
	return request(`/admin/users/${id}`, {
		method: 'PUT',
		body: JSON.stringify(data)
	});
}

export function deleteUser(id) {
	return request(`/admin/users/${id}`, { method: 'DELETE' });
}

// ── Dashboard ───────────────────────────────────────────────────────────────
export function getDashboard() {
	return request('/dashboard');
}

// ── Works ───────────────────────────────────────────────────────────────────
export function listWorks(params = {}) {
	const qs = new URLSearchParams();
	if (params.limit) qs.set('limit', params.limit);
	if (params.offset) qs.set('offset', params.offset);
	if (params.sort) qs.set('sort', params.sort);
	if (params.order) qs.set('order', params.order);
	if (params.needs_review !== undefined) qs.set('needs_review', params.needs_review);
	if (params.language) qs.set('language', params.language);
	if (params.format) qs.set('format', params.format);
	return request(`/works?${qs}`);
}

export function searchWorks(q, limit = 50, offset = 0) {
	return request(`/works/search?q=${encodeURIComponent(q)}&limit=${limit}&offset=${offset}`);
}

export function getWork(id) {
	return request(`/works/${id}`);
}

export function updateWork(id, fields) {
	return request(`/works/${id}`, {
		method: 'PUT',
		body: JSON.stringify(fields)
	});
}

export function deleteWork(id) {
	return request(`/works/${id}`, { method: 'DELETE' });
}

// ── Contributors ────────────────────────────────────────────────────────────
export function listContributors(limit = 50, offset = 0) {
	return request(`/contributors?limit=${limit}&offset=${offset}`);
}

export function getContributor(id, limit = 50, offset = 0) {
	return request(`/contributors/${id}?limit=${limit}&offset=${offset}`);
}

// ── Series ──────────────────────────────────────────────────────────────────
export function listSeries(limit = 50, offset = 0) {
	return request(`/series?limit=${limit}&offset=${offset}`);
}

export function getSeries(id, limit = 50, offset = 0) {
	return request(`/series/${id}?limit=${limit}&offset=${offset}`);
}

// ── Tags ────────────────────────────────────────────────────────────────────
export function listTags() {
	return request('/tags');
}

export function getTag(id, limit = 50, offset = 0) {
	return request(`/tags/${id}?limit=${limit}&offset=${offset}`);
}

// ── Collections ─────────────────────────────────────────────────────────────
export function listCollections() {
	return request('/collections');
}

export function getCollection(id, limit = 50, offset = 0) {
	return request(`/collections/${id}?limit=${limit}&offset=${offset}`);
}

export function createCollection(data) {
	return request('/collections', {
		method: 'POST',
		body: JSON.stringify(data)
	});
}

export function updateCollection(id, data) {
	return request(`/collections/${id}`, {
		method: 'PUT',
		body: JSON.stringify(data)
	});
}

export function deleteCollection(id) {
	return request(`/collections/${id}`, { method: 'DELETE' });
}

export function addWorkToCollection(collectionId, workId, position = 0) {
	return request(`/collections/${collectionId}/works`, {
		method: 'POST',
		body: JSON.stringify({ work_id: workId, position })
	});
}

export function removeWorkFromCollection(collectionId, workId) {
	return request(`/collections/${collectionId}/works/${workId}`, { method: 'DELETE' });
}

// ── Metadata ────────────────────────────────────────────────────────────────
export function refreshMetadata(workId) {
	return request(`/metadata/refresh/${workId}`, { method: 'POST' });
}

export function getMetadataTasks(workId) {
	return request(`/metadata/tasks/${workId}`);
}

export function applyCandidate(taskId, candidateIndex) {
	return request(`/metadata/apply/${taskId}`, {
		method: 'POST',
		body: JSON.stringify({ candidate_index: candidateIndex })
	});
}

export function getReviewQueue(limit = 50, offset = 0) {
	return request(`/metadata/review?limit=${limit}&offset=${offset}`);
}

// ── Scan ────────────────────────────────────────────────────────────────────
export function triggerScan() {
	return request('/scan', { method: 'POST' });
}

export function getScanStatus() {
	return request('/scan/status');
}

// ── Covers ──────────────────────────────────────────────────────────────────
export function getWorkCovers(workId) {
	return request(`/works/${workId}/covers`);
}

export function selectCover(workId, source) {
	return request(`/works/${workId}/covers/select`, {
		method: 'PUT',
		body: JSON.stringify({ source })
	});
}

// ── Settings ────────────────────────────────────────────────────────────────
export function getSettings() {
	return request('/settings');
}

export function updateSettings(settings) {
	return request('/settings', {
		method: 'PUT',
		body: JSON.stringify(settings)
	});
}
