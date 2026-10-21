'use strict';

// ─── Theme ────────────────────────────────────────────────────────────────────
function toggleTheme() {
    const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    try { localStorage.setItem('sm-theme', next); } catch (e) {}
    // Type and age palettes are stepped per theme.
    if (state.current && state.colorMode !== 'classic') renderTreemap();
}

// ─── Auth ─────────────────────────────────────────────────────────────────────
function showLogin() {
    stopScan();
    hideProgress();
    closePopovers();
    $('app').hidden = true;
    $('login-overlay').hidden = false;
    $('login-error').textContent = '';
    setTimeout(() => $('login-password').focus(), 30);
}

function showApp() {
    $('login-overlay').hidden = true;
    $('app').hidden = false;
    $('btn-logout').hidden = !state.authRequired;
}

async function doLogin(ev) {
    ev.preventDefault();
    const input = $('login-password'), errEl = $('login-error'), btn = $('login-btn');
    errEl.textContent = '';
    btn.disabled = true;
    try {
        const resp = await fetch('/api/auth', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ password: input.value }),
        });
        if (resp.ok) {
            input.value = '';
            showApp();
            await loadConfig();
            await restoreLastScan();
            return;
        }
        const body = await resp.json().catch(() => ({}));
        errEl.textContent = resp.status === 429
            ? `Too many attempts — try again in ${body.retryAfter || 60}s.`
            : 'Incorrect password.';
        input.select();
    } catch (e) {
        errEl.textContent = 'Connection error.';
    } finally {
        btn.disabled = false;
    }
}

async function doLogout() {
    stopScan();
    await fetch('/api/logout', { method: 'POST' }).catch(() => {});
    clearScan();
    showLogin();
}

async function loadConfig() {
    try {
        const cfg = await apiJSON('/api/config');
        state.config = cfg;
        if (cfg.version) $('about-version').textContent = cfg.version;
    } catch (e) { /* defaults stay */ }
}

// ─── Keyboard ─────────────────────────────────────────────────────────────────
document.addEventListener('keydown', e => {
    const tag = document.activeElement && document.activeElement.tagName;
    const typing = tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT';

    if (e.key === 'Escape') {
        if (anyPopoverOpen()) { closePopovers(); return; }
        if (anyModalOpen()) { document.querySelectorAll('.modal').forEach(m => { m.hidden = true; }); return; }
        if (typing && document.activeElement.id === 'search-input') {
            if (state.search) clearSearch(); else document.activeElement.blur();
            return;
        }
        if (!$('progress-overlay').hidden) return;
        selectNode(null, '');
        return;
    }
    if (typing) return;
    if (anyModalOpen() || anyPopoverOpen() || !$('progress-overlay').hidden || $('app').hidden) return;

    switch (e.key) {
        case 'Backspace': case 'ArrowLeft': e.preventDefault(); zoomOut(); break;
        case 'Enter':     zoomIntoSelected(); break;
        case 'Home':      zoomFull(); break;
        case 'Delete':    if (!$('btn-delete').hidden) openDeleteModal(); break;
        case '/':         e.preventDefault(); if (!$('search-input').disabled) $('search-input').focus(); break;
        case 'r': case 'R':
            if (!e.ctrlKey && !e.metaKey && state.scan) startScan(state.scan.path);
            break;
        case 'o': case 'O':
            if (!e.ctrlKey && !e.metaKey) openDriveDialog();
            break;
    }
});

// ─── Resize ───────────────────────────────────────────────────────────────────
window.addEventListener('resize', debounce(() => { if (state.current) renderTreemap(); }, 150));

// ─── Init ─────────────────────────────────────────────────────────────────────
function bindEvents() {
    $('login-form').addEventListener('submit', doLogin);
    $('btn-open').addEventListener('click', openDriveDialog);
    $('btn-open-empty').addEventListener('click', openDriveDialog);
    $('btn-reload').addEventListener('click', () => { if (state.scan) startScan(state.scan.path); });
    $('btn-zoom-full').addEventListener('click', zoomFull);
    $('btn-zoom-in').addEventListener('click', zoomIntoSelected);
    $('btn-zoom-out').addEventListener('click', zoomOut);
    $('btn-free-space').addEventListener('click', () => {
        state.freeSpace = !state.freeSpace;
        $('btn-free-space').classList.toggle('active', state.freeSpace);
        if (state.current) renderTreemap();
    });
    $('btn-theme').addEventListener('click', toggleTheme);
    $('color-mode').addEventListener('change', e => setColorMode(e.target.value));
    $('btn-panel').addEventListener('click', togglePanel);
    $('btn-about').addEventListener('click', () => { closePopovers(); $('about-modal').hidden = false; });
    $('btn-logout').addEventListener('click', doLogout);
    $('btn-cancel-scan').addEventListener('click', () => { stopScan(); hideProgress(); toast('Scan cancelled'); });
    $('info-bar-close').addEventListener('click', () => { $('info-bar').hidden = true; });

    // Search & filter
    $('search-input').addEventListener('input', onSearchInput);
    $('btn-filter').addEventListener('click', () => {
        if (!$('filter-popover').hidden) closePopovers(); else openFilterPopover();
    });
    $('filter-apply').addEventListener('click', applyFilterFromPopover);
    $('filter-clear').addEventListener('click', clearFilter);
    $('filter-popover').addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); applyFilterFromPopover(); } });

    // Scans / compare
    $('btn-scans').addEventListener('click', () => {
        if (!$('scans-popover').hidden) closePopovers(); else openScansPopover();
    });
    document.querySelectorAll('.seg-btn').forEach(b => b.addEventListener('click', () => {
        state.compareMode = b.dataset.cmp;
        renderCompare();
    }));

    // Panel
    document.querySelectorAll('.tab').forEach(t => t.addEventListener('click', () => setPanelTab(t.dataset.tab)));
    $('btn-copy-path').addEventListener('click', () => { const it = focusItem(); if (it) copyText(absOf(it.rel)); });
    $('btn-export-csv').addEventListener('click', () => exportFocus('csv'));
    $('btn-export-json').addEventListener('click', () => exportFocus('json'));
    $('btn-delete').addEventListener('click', openDeleteModal);
    $('delete-confirm-check').addEventListener('change', e => { $('delete-confirm-btn').disabled = !e.target.checked; });
    $('delete-confirm-btn').addEventListener('click', confirmDelete);

    // Modals & popovers
    document.querySelectorAll('[data-close]').forEach(el =>
        el.addEventListener('click', () => closeModal(el.dataset.close)));
    document.querySelectorAll('.modal').forEach(el =>
        el.addEventListener('click', e => { if (e.target === el) el.hidden = true; }));
    document.addEventListener('mousedown', e => {
        if (!anyPopoverOpen()) return;
        if (e.target.closest('.popover') || e.target.closest('#btn-filter') || e.target.closest('#btn-scans')) return;
        closePopovers();
    });
}

async function init() {
    initTooltip();
    bindEvents();
    loadColorMode();

    try {
        if (localStorage.getItem('sm-panel') === '0') {
            $('side-panel').hidden = true;
            $('btn-panel').classList.remove('active');
        }
    } catch (e) {}

    fetch('/api/version').then(r => r.json())
        .then(d => { if (d.version) $('about-version').textContent = d.version; })
        .catch(() => {});

    let status;
    try {
        status = await (await fetch('/api/auth')).json();
    } catch (e) {
        status = { authRequired: false, authenticated: true };
        toast('Could not reach the server.', 'error');
    }
    state.authRequired = !!status.authRequired;
    if (status.authRequired && !status.authenticated) { showLogin(); return; }
    showApp();
    await loadConfig();
    await restoreLastScan();
}

document.addEventListener('DOMContentLoaded', init);
