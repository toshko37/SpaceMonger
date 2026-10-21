'use strict';

// ─── Scans popover ────────────────────────────────────────────────────────────
async function openScansPopover() {
    showPopover('scans-popover', $('btn-scans'));
    const list = $('scans-list');
    list.innerHTML = '<div class="panel-empty">Loading…</div>';
    $('scans-hint').textContent = `(last ${state.config.historySize} kept in memory)`;
    let scans;
    try {
        scans = await apiJSON('/api/scans');
    } catch (e) {
        list.innerHTML = `<div class="panel-empty">${esc(e.message)}</div>`;
        return;
    }
    if (!scans.length) {
        list.innerHTML = '<div class="panel-empty">No scans yet.</div>';
        return;
    }
    const curId = state.scan ? state.scan.id : null;
    const curPath = state.scan ? state.scan.path : null;
    list.innerHTML = scans.map(s => {
        const isCur = s.id === curId;
        const canCompare = curId && !isCur && s.path === curPath;
        return `<div class="scan-item${isCur ? ' current' : ''}" data-id="${esc(s.id)}">` +
            `<div class="mono">${esc(s.path)}</div>` +
            `<div class="sub">${esc(new Date(s.scannedAt * 1000).toLocaleString())} · ${formatSize(s.size)} · ${s.files.toLocaleString()} files${isCur ? ' · current' : ''}</div>` +
            `<div class="acts">` +
            (isCur ? '' : `<button class="btn btn-small" data-act="load" title="Show this scan">Load</button>`) +
            (canCompare ? `<button class="btn btn-small" data-act="compare" title="Compare with the current scan"><svg class="icon"><use href="#i-diff"/></svg></button>` : '') +
            `<button class="btn btn-icon btn-ghost btn-small" data-act="remove" title="Forget this scan"><svg class="icon"><use href="#i-x"/></svg></button>` +
            `</div></div>`;
    }).join('');

    list.querySelectorAll('[data-act]').forEach(btn => {
        const id = btn.closest('.scan-item').dataset.id;
        btn.addEventListener('click', async ev => {
            ev.stopPropagation();
            const act = btn.dataset.act;
            if (act === 'load') { closePopovers(); loadScanData(id); }
            if (act === 'compare') { closePopovers(); openCompare(id, curId); }
            if (act === 'remove') {
                try { await apiDelete('/api/scans?id=' + encodeURIComponent(id)); } catch (e) { toast(e.message, 'error'); }
                if (id === curId) clearScan();
                openScansPopover();
            }
        });
    });
}

// ─── Compare modal ────────────────────────────────────────────────────────────
// a = older scan, b = newer (current). Rows navigate within the current scan.
async function openCompare(aId, bId) {
    showLoading('Comparing…');
    try {
        const res = await apiJSON(`/api/compare?a=${encodeURIComponent(aId)}&b=${encodeURIComponent(bId)}&limit=500`);
        // Make sure "older" really is the older one for the labels.
        if (res.a.scannedAt > res.b.scannedAt) {
            const swapped = await apiJSON(`/api/compare?a=${encodeURIComponent(bId)}&b=${encodeURIComponent(aId)}&limit=500`);
            state.compare = swapped;
        } else {
            state.compare = res;
        }
    } catch (e) {
        hideProgress();
        toast('Compare failed: ' + e.message, 'error', 6000);
        return;
    }
    hideProgress();
    state.compareMode = 'all';
    renderCompare();
    $('compare-modal').hidden = false;
}

function renderCompare() {
    const c = state.compare;
    if (!c) return;
    const s = c.summary;
    const when = m => new Date(m.scannedAt * 1000).toLocaleString();
    $('compare-summary').innerHTML =
        `<div><b>${formatSize(s.before)}</b>${esc(when(c.a))}</div>` +
        `<div><b>${formatSize(s.after)}</b>${esc(when(c.b))}</div>` +
        `<div><b class="${s.delta >= 0 ? 'pos' : 'neg'}">${formatDelta(s.delta)}</b>change</div>` +
        `<div><b>${s.added.toLocaleString()}</b>added</div>` +
        `<div><b>${s.removed.toLocaleString()}</b>removed</div>` +
        `<div><b>${s.changed.toLocaleString()}</b>changed</div>`;

    document.querySelectorAll('.seg-btn').forEach(b => b.classList.toggle('active', b.dataset.cmp === state.compareMode));
    let rows = c.changes.filter(ch => ch.path !== '');
    if (state.compareMode === 'grew')   rows = rows.filter(ch => ch.delta > 0);
    if (state.compareMode === 'shrunk') rows = rows.filter(ch => ch.delta < 0);
    $('compare-count').textContent = `${rows.length} entries`;

    const maxAbs = rows.reduce((m, ch) => Math.max(m, Math.abs(ch.delta)), 1);
    const list = $('compare-list');
    if (!rows.length) { list.innerHTML = '<div class="panel-empty">No differences.</div>'; return; }
    list.innerHTML = rows.map(ch => {
        const cls = ch.delta > 0 ? 'pos' : 'neg';
        const name = ch.path.split('/').pop();
        const dirPart = ch.path.slice(0, -name.length).replace(/\/$/, '');
        const status = ch.added ? 'added' : ch.gone ? 'removed' : `${formatSize(ch.before)} → ${formatSize(ch.after)}`;
        return `<button class="row crow has-bar${ch.dir ? ' dir' : ''}" data-rel="${esc(ch.path)}" data-gone="${ch.gone ? 1 : 0}">` +
            `<svg class="icon"><use href="#${ch.dir ? 'i-folder' : 'i-file'}"/></svg>` +
            `<span class="row-name">${esc(name)}</span>` +
            `<span class="row-size ${cls}">${formatDelta(ch.delta)}</span>` +
            `<span class="row-sub">${esc(dirPart)}</span>` +
            `<span class="row-pct">${esc(status)}</span>` +
            `<span class="delta-bar ${cls}"><span style="width:${(Math.abs(ch.delta) / maxAbs * 100).toFixed(1)}%"></span></span>` +
            `</button>`;
    }).join('');

    list.querySelectorAll('.row').forEach(el => el.addEventListener('click', async () => {
        if (el.dataset.gone === '1') { toast('This item no longer exists in the newer scan.'); return; }
        closeModal('compare-modal');
        if (!state.scan || state.scan.id !== c.b.id) await loadScanDataAndThen(c.b.id, () => revealRel(el.dataset.rel));
        else revealRel(el.dataset.rel);
    }));
}

// Load a scan, then run fn once it is rendered.
async function loadScanDataAndThen(id, fn) {
    showLoading('Loading tree…');
    try {
        const msg = await apiJSON('/api/data?id=' + encodeURIComponent(id));
        if (!msg.root) throw new Error('empty result');
        applyScanResult(msg);
        clearSearch();
        hideProgress();
        fn();
    } catch (e) {
        hideProgress();
        toast('Could not load scan: ' + e.message, 'error');
    }
}
