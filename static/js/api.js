'use strict';

// fetch wrapper: a 401 while auth is on means the session expired.
async function apiFetch(url, opts) {
    const resp = await fetch(url, opts);
    if (resp.status === 401 && state.authRequired) {
        showLogin();
        throw new Error('Session expired — please sign in again.');
    }
    return resp;
}

async function apiJSON(url, opts) {
    const resp = await apiFetch(url, opts);
    const body = await resp.json().catch(() => ({}));
    if (!resp.ok) throw new Error(body.error || `HTTP ${resp.status}`);
    return body;
}

function apiPost(url, data) {
    return apiJSON(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data || {}),
    });
}

function apiDelete(url) {
    return apiJSON(url, { method: 'DELETE' });
}
