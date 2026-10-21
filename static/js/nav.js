'use strict';

// ─── Paths ────────────────────────────────────────────────────────────────────
// Relative path (to the scan root) of the current view.
function currentRel() {
    return state.navStack.slice(1).map(n => n.n).join('/');
}

// Relative path of a d3 node rendered under the current view.
function relOfD3(d) {
    const names = d.ancestors().reverse().slice(1).map(x => x.data.n);
    return relJoin(currentRel(), names.join('/'));
}

function absOf(rel) {
    if (!state.scan) return '';
    const root = state.scan.path.replace(/\/+$/, '');
    return rel ? (root + '/' + rel) : (root || '/');
}

// findByRel walks from the scan root; returns { node, ancestors } or null.
function findByRel(rel) {
    if (!state.scan) return null;
    let node = state.scan.root;
    const ancestors = [];
    if (rel) {
        for (const seg of rel.split('/')) {
            if (!seg) continue;
            const next = (node.c || []).find(c => c.n === seg);
            if (!next) return null;
            ancestors.push(node);
            node = next;
        }
    }
    return { node, ancestors };
}

// revealRel navigates to the parent folder of rel and selects the item.
function revealRel(rel) {
    const found = findByRel(rel);
    if (!found) { toast('Path not found in this scan', 'error'); return; }
    state.navStack = [state.scan.root, ...found.ancestors.slice(1)];
    state.current  = state.navStack[state.navStack.length - 1];
    if (!found.ancestors.length) { // the root itself
        refreshView();
        return;
    }
    refreshView();
    selectNode(found.node, rel);
}

// removeLocal drops a node from the in-memory tree after a deletion.
function removeLocal(rel) {
    const found = findByRel(rel);
    if (!found || !found.ancestors.length) return false;
    const parent = found.ancestors[found.ancestors.length - 1];
    parent.c = parent.c.filter(c => c !== found.node);
    found.ancestors.forEach(a => { a.s -= found.node.s; });
    const counts = countTree(found.node);
    state.scan.files -= counts.files;
    state.scan.dirs  -= counts.dirs;
    // Pop the view back if we deleted something on the navigation path.
    const idx = state.navStack.indexOf(found.node);
    if (idx > 0) {
        state.navStack = state.navStack.slice(0, idx);
        state.current  = state.navStack[state.navStack.length - 1];
    }
    return true;
}

function countTree(n) {
    if (!isDirNode(n)) return { files: 1, dirs: 0 };
    const r = { files: 0, dirs: 1 };
    (n.c || []).forEach(c => { const x = countTree(c); r.files += x.files; r.dirs += x.dirs; });
    return r;
}

// ─── Navigation ───────────────────────────────────────────────────────────────
function zoomIntoNode(node) {
    if (!hasChildren(node)) return;
    state.navStack.push(node);
    state.current = node;
    refreshView();
}

function zoomIntoSelected() {
    if (!state.selected || !hasChildren(state.selected)) return;
    const found = findByRel(state.selectedRel);
    if (!found) return;
    state.navStack = [state.scan.root, ...found.ancestors.slice(1), found.node];
    state.current  = found.node;
    refreshView();
}

function zoomOut() {
    if (state.navStack.length <= 1) return;
    state.navStack.pop();
    state.current = state.navStack[state.navStack.length - 1];
    refreshView();
}

function zoomFull() {
    if (!state.scan) return;
    state.navStack = [state.scan.root];
    state.current  = state.scan.root;
    refreshView();
}

function jumpTo(depth) {
    state.navStack = state.navStack.slice(0, depth + 1);
    state.current  = state.navStack[state.navStack.length - 1];
    refreshView();
}

function refreshView() {
    state.selected = null;
    state.selectedRel = '';
    $('status-selected').textContent = '';
    renderTreemap();
    updateBreadcrumb();
    updateButtons();
    updateTitle();
    renderPanel();
}

function updateBreadcrumb() {
    const bc = $('breadcrumb');
    bc.innerHTML = '';
    if (!state.scan) return;
    state.navStack.forEach((node, i) => {
        const last = i === state.navStack.length - 1;
        if (i > 0) {
            const sep = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
            sep.setAttribute('class', 'icon bc-sep');
            sep.innerHTML = '<use href="#i-chevron"/>';
            bc.appendChild(sep);
        }
        const b = document.createElement('button');
        b.className = 'bc-item' + (last ? ' current' : '');
        b.textContent = i === 0 ? state.scan.path : node.n;
        if (!last) b.addEventListener('click', () => jumpTo(i));
        bc.appendChild(b);
    });
    const size = document.createElement('span');
    size.className = 'bc-size';
    size.textContent = formatSize(state.current.s);
    bc.appendChild(size);
    bc.scrollLeft = bc.scrollWidth;
}

function updateButtons() {
    const atRoot = state.navStack.length <= 1;
    const has = !!state.scan;
    $('btn-reload').disabled     = !has;
    $('btn-free-space').disabled = !has;
    $('btn-filter').disabled     = !has;
    $('color-mode').disabled     = !has;
    $('search-input').disabled   = !has;
    $('btn-zoom-out').disabled   = atRoot;
    $('btn-zoom-full').disabled  = atRoot;
    $('btn-zoom-in').disabled    = !hasChildren(state.selected);
}

function updateTitle() {
    document.title = state.current ? `${state.current.n} — SpaceMonger` : 'SpaceMonger';
}

// ─── Status & info bars ──────────────────────────────────────────────────────
function updateStatusBar() {
    const s = state.scan;
    if (!s) { $('status-space').hidden = true; $('status-scan').hidden = true; return; }
    $('status-files').textContent = s.files.toLocaleString();
    $('status-dirs').textContent  = s.dirs.toLocaleString();

    if (s.totalDisk > 0) {
        const used = s.totalDisk - s.freeDisk;
        const pct  = used / s.totalDisk * 100;
        const fill = $('usage-fill');
        fill.style.width = pct.toFixed(1) + '%';
        fill.className = 'usage-fill' + (pct >= 95 ? ' crit' : pct >= 85 ? ' warn' : '');
        $('usage-pct').textContent    = pct.toFixed(0) + '%';
        $('status-free').textContent  = formatSize(s.freeDisk);
        $('status-total').textContent = formatSize(s.totalDisk);
        $('status-space').hidden = false;
    } else {
        $('status-space').hidden = true;
    }

    const scanEl = $('status-scan');
    if (s.scannedAt) {
        const secs = (s.durationMs / 1000).toFixed(s.durationMs < 10000 ? 1 : 0);
        scanEl.textContent = `scanned ${new Date(s.scannedAt * 1000).toLocaleTimeString()} in ${secs}s`;
        scanEl.hidden = false;
    } else {
        scanEl.hidden = true;
    }
}

function updateInfoBar() {
    const bar = $('info-bar');
    const s = state.scan;
    if (!s) { bar.hidden = true; return; }
    const apparent = s.apparentSize || 0, actual = s.root.s, saved = apparent - actual;
    const pct = apparent > 0 ? saved / apparent * 100 : 0;
    if (apparent > 0 && pct >= 2) {
        const kind = pct >= 30 ? 'Compression' : 'Sparse files / compression';
        $('info-bar-text').textContent =
            `${kind} detected — apparent ${formatSize(apparent)}, on disk ${formatSize(actual)}, ` +
            `saved ${formatSize(saved)} (${pct.toFixed(1)}%)`;
        bar.hidden = false;
    } else {
        bar.hidden = true;
    }
}

// applyScanResult installs a scan (metadata + tree) as the current one.
function applyScanResult(msg) {
    state.scan = {
        id: msg.id, path: msg.path, root: msg.root,
        files: msg.files || 0, dirs: msg.dirs || 0,
        totalDisk: msg.totalDisk || 0, freeDisk: msg.freeDisk || 0,
        apparentSize: msg.apparentSize || 0,
        scannedAt: msg.scannedAt || 0, durationMs: msg.durationMs || 0,
    };
    state.navStack = [state.scan.root];
    state.current  = state.scan.root;
    updateStatusBar();
    updateInfoBar();
}

function clearScan() {
    state.scan = null; state.current = null; state.navStack = [];
    state.selected = null; state.selectedRel = '';
    renderTreemap();
    updateBreadcrumb();
    updateButtons();
    updateStatusBar();
    updateInfoBar();
    renderPanel();
}
