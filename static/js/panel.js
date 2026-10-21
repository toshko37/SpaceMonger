'use strict';

const LARGEST_LIMIT = 100;
const SEARCH_LIMIT  = 300;

// focusItem is what the panel header and actions refer to: the selected
// item if any, otherwise the folder currently shown.
function focusItem() {
    if (!state.scan) return null;
    if (state.selected) return { node: state.selected, rel: state.selectedRel };
    return { node: state.current, rel: currentRel() };
}

function renderPanel() {
    const panel = $('side-panel');
    if (panel.hidden) return;
    const item = focusItem();
    const list = $('panel-list');

    if (!item) {
        $('panel-name').textContent = '—';
        $('panel-path').textContent = '';
        $('panel-meta').textContent = '';
        $('btn-delete').hidden = true;
        list.innerHTML = '<div class="panel-empty">Open a drive to see details here.</div>';
        return;
    }

    const n = item.node;
    $('panel-icon').innerHTML = `<use href="#${isDirNode(n) ? 'i-folder' : 'i-file'}"/>`;
    $('panel-name').textContent = n.n;
    $('panel-path').textContent = absOf(item.rel);
    const parentSize = state.selected ? state.current.s : (state.scan.root.s);
    $('panel-meta').innerHTML =
        `<b>${formatSize(n.s)}</b>` +
        (state.selected ? ` · ${formatPct(n.s, parentSize)} of ${esc(state.current.n)}` : ` · ${formatPct(n.s, state.scan.root.s)} of scan`) +
        (isDirNode(n) ? ` · ${(n.c || []).length.toLocaleString()} items` : '') +
        `<br>modified ${esc(formatDate(n.m))}`;
    $('btn-delete').hidden = !(state.config.allowDelete && item.rel !== '');

    document.querySelectorAll('.tab').forEach(t => t.classList.toggle('active', t.dataset.tab === state.panelTab));

    switch (state.panelTab) {
        case 'largest': renderLargest(list); break;
        case 'search':  renderSearchResults(list); break;
        default:        renderContents(list);
    }
}

function rowHTML(n, rel, opts) {
    const dir = isDirNode(n);
    const pct = opts.total > 0 ? n.s / opts.total * 100 : 0;
    return `<button class="row${dir ? ' dir' : ''}${opts.selected ? ' selected' : ''}${opts.bar ? ' has-bar' : ''}" data-rel="${esc(rel)}">` +
        `<svg class="icon"><use href="#${dir ? 'i-folder' : 'i-file'}"/></svg>` +
        `<span class="row-name">${esc(n.n)}</span>` +
        `<span class="row-size">${formatSize(n.s)}</span>` +
        `<span class="row-sub">${esc(opts.sub || '')}</span>` +
        `<span class="row-pct">${opts.pct !== false ? pct.toFixed(1) + '%' : ''}</span>` +
        (opts.bar ? `<span class="bar"><span style="width:${Math.min(100, pct).toFixed(1)}%"></span></span>` : '') +
        `</button>`;
}

function bindRows(list, resolve) {
    list.querySelectorAll('.row').forEach(el => {
        const rel = el.dataset.rel;
        el.addEventListener('click', () => {
            const node = resolve(rel);
            if (!node) return;
            if (state.selected === node) selectNode(null, ''); else selectNode(node, rel);
        });
        el.addEventListener('dblclick', () => {
            const node = resolve(rel);
            if (hasChildren(node)) revealAndZoom(rel);
        });
    });
}

function revealAndZoom(rel) {
    const found = findByRel(rel);
    if (!found) return;
    state.navStack = [state.scan.root, ...found.ancestors.slice(1), found.node];
    state.current  = found.node;
    refreshView();
}

function renderContents(list) {
    const cur = state.current;
    const base = currentRel();
    const kids = (cur.c || []).slice().sort((a, b) => b.s - a.s);
    if (!kids.length) { list.innerHTML = '<div class="panel-empty">Empty folder.</div>'; return; }
    const sub = n => isDirNode(n) ? `${(n.c || []).length} items` : extOf(n.n).toUpperCase() || 'file';
    list.innerHTML = kids.map(n => rowHTML(n, relJoin(base, n.n), {
        total: cur.s, bar: true, selected: n === state.selected, sub: sub(n),
    })).join('');
    bindRows(list, rel => kids.find(n => relJoin(base, n.n) === rel));
}

// Top files in the current subtree, kept as a bounded list while walking.
function largestFiles(root, baseRel, limit) {
    const top = [];
    let floor = -1;
    const walk = (n, rel) => {
        if (!isDirNode(n)) {
            if (n.s <= floor && top.length >= limit) return;
            top.push({ n, rel });
            if (top.length >= limit * 2) {
                top.sort((a, b) => b.n.s - a.n.s);
                top.length = limit;
                floor = top[top.length - 1].n.s;
            }
            return;
        }
        for (const c of n.c || []) walk(c, relJoin(rel, c.n));
    };
    walk(root, baseRel);
    top.sort((a, b) => b.n.s - a.n.s);
    return top.slice(0, limit);
}

function renderLargest(list) {
    const base = currentRel();
    const items = largestFiles(state.current, base, LARGEST_LIMIT);
    if (!items.length) { list.innerHTML = '<div class="panel-empty">No files in this folder.</div>'; return; }
    list.innerHTML = items.map(it => rowHTML(it.n, it.rel, {
        total: state.current.s, selected: it.n === state.selected,
        sub: it.rel.slice(base ? base.length + 1 : 0).split('/').slice(0, -1).join('/'),
    })).join('');
    const index = new Map(items.map(it => [it.rel, it.n]));
    bindRows(list, rel => index.get(rel));
}

function renderSearchResults(list) {
    const base = currentRel();
    beginMatchPass();
    const hits = [];
    const walk = (n, rel) => {
        if (rel !== base && nodeMatches(n) && (isDirNode(n) ? nameMatches(n) && !isFilterActive() : true)) {
            hits.push({ n, rel });
        }
        if (isDirNode(n)) for (const c of n.c || []) walk(c, relJoin(rel, c.n));
    };
    walk(state.current, base);
    hits.sort((a, b) => b.n.s - a.n.s);
    const shown = hits.slice(0, SEARCH_LIMIT);
    if (!hits.length) { list.innerHTML = '<div class="panel-empty">No matches in this folder.</div>'; return; }
    list.innerHTML =
        `<div class="panel-empty" style="padding:8px 14px;text-align:left">${hits.length.toLocaleString()} match${hits.length === 1 ? '' : 'es'}${hits.length > SEARCH_LIMIT ? `, showing ${SEARCH_LIMIT}` : ''}</div>` +
        shown.map(it => rowHTML(it.n, it.rel, {
            total: state.current.s, selected: it.n === state.selected,
            sub: it.rel.slice(base ? base.length + 1 : 0).split('/').slice(0, -1).join('/'),
        })).join('');
    const index = new Map(shown.map(it => [it.rel, it.n]));
    bindRows(list, rel => index.get(rel));
}

// ─── Actions ──────────────────────────────────────────────────────────────────
function exportFocus(format) {
    const item = focusItem();
    if (!item) return;
    downloadURL(`/api/export?id=${encodeURIComponent(state.scan.id)}&format=${format}&path=${encodeURIComponent(item.rel)}`);
}

function openDeleteModal() {
    const item = focusItem();
    if (!item || !state.config.allowDelete || item.rel === '') return;
    const n = item.node;
    const counts = countTree(n);
    $('delete-path').textContent = absOf(item.rel);
    $('delete-meta').textContent = `${formatSize(n.s)}` +
        (isDirNode(n) ? ` · ${counts.files.toLocaleString()} files, ${(counts.dirs - 1).toLocaleString()} folders` : '');
    $('delete-confirm-check').checked = false;
    $('delete-confirm-btn').disabled = true;
    $('delete-modal').dataset.rel = item.rel;
    $('delete-modal').hidden = false;
}

async function confirmDelete() {
    const rel = $('delete-modal').dataset.rel;
    const btn = $('delete-confirm-btn');
    btn.disabled = true;
    try {
        const res = await apiPost('/api/delete', { id: state.scan.id, path: absOf(rel) });
        closeModal('delete-modal');
        removeLocal(rel);
        // Free space changed; refresh the metadata from the server.
        try {
            const scans = await apiJSON('/api/scans');
            const me = scans.find(s => s.id === state.scan.id);
            if (me) { state.scan.files = me.files; state.scan.dirs = me.dirs; }
        } catch (e) { /* non-fatal */ }
        updateStatusBar();
        refreshView();
        toast(`Deleted — ${formatSize(res.freed)} freed`);
    } catch (e) {
        toast('Delete failed: ' + e.message, 'error', 6000);
        btn.disabled = false;
    }
}

function setPanelTab(tab) {
    state.panelTab = tab;
    renderPanel();
}

function togglePanel() {
    const panel = $('side-panel');
    panel.hidden = !panel.hidden;
    $('btn-panel').classList.toggle('active', !panel.hidden);
    try { localStorage.setItem('sm-panel', panel.hidden ? '0' : '1'); } catch (e) {}
    if (state.current) renderTreemap();
    renderPanel();
}
