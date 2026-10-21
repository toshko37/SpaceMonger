'use strict';

// ─── State ────────────────────────────────────────────────────────────────────
const state = {
    scan:        null,   // { path, root, files, dirs, totalDisk, freeDisk, apparentSize, scannedAt, durationMs }
    navStack:    [],     // zoom history: FileNode[]
    current:     null,   // node currently displayed
    selected:    null,   // d3 node selected by single click
    freeSpace:   false,  // free-space overlay toggle
    authRequired:false,
    evtSource:   null,   // active EventSource during a scan
};

let clickTimer = null;
let tooltipEl  = null;

const $ = id => document.getElementById(id);

// ─── Formatting helpers ───────────────────────────────────────────────────────
function formatSize(bytes) {
    if (!bytes) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.min(Math.floor(Math.log(Math.abs(bytes)) / Math.log(1024)), units.length - 1);
    const v = bytes / Math.pow(1024, i);
    return (i === 0 ? v.toFixed(0) : v.toFixed(v >= 100 ? 0 : 1)) + ' ' + units[i];
}

function formatDate(unixSec) {
    return unixSec ? new Date(unixSec * 1000).toLocaleString() : '—';
}

function formatPct(part, total) {
    return total > 0 ? (part / total * 100).toFixed(1) + '%' : '—';
}

function esc(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;')
                    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function countNodes(node) {
    let n = 0;
    (function walk(x) { n++; if (x.c) x.c.forEach(walk); })(node);
    return n - 1;
}

// ─── Theme ────────────────────────────────────────────────────────────────────
function toggleTheme() {
    const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    try { localStorage.setItem('sm-theme', next); } catch (e) {}
}

// ─── Depth palette ────────────────────────────────────────────────────────────
// Colour is a function of absolute depth in the scanned tree, so a folder
// keeps its colour when you zoom in and out.
const DEPTH_COLORS = [
    '#f87171', '#fb923c', '#facc15', '#4ade80', '#2dd4bf', '#38bdf8',
    '#818cf8', '#c084fc', '#f472b6', '#a3e635', '#fbbf24', '#34d399',
];

function cellColor(d) {
    const abs = (state.navStack.length - 1) + d.depth; // depth 1 == child of scan root
    return DEPTH_COLORS[(abs - 1) % DEPTH_COLORS.length];
}

// ─── Toasts ───────────────────────────────────────────────────────────────────
function toast(msg, kind = 'info', ms = 4000) {
    const el = document.createElement('div');
    el.className = 'toast' + (kind === 'error' ? ' error' : '');
    el.textContent = msg;
    $('toasts').appendChild(el);
    setTimeout(() => el.remove(), ms);
}

// ─── Tooltip ──────────────────────────────────────────────────────────────────
function initTooltip() {
    tooltipEl = document.createElement('div');
    tooltipEl.className = 'tooltip';
    tooltipEl.hidden = true;
    document.body.appendChild(tooltipEl);
}

function showTooltip(ev, d, viewTotal) {
    const n = d.data;
    const extra = n.d
        ? `${(n.c ? n.c.length : 0).toLocaleString()} items`
        : 'File';
    tooltipEl.innerHTML =
        `<strong>${esc(n.n)}</strong>` +
        `<div>${formatSize(d.value)} <span class="muted">· ${formatPct(d.value, viewTotal)} of view</span></div>` +
        `<div class="muted">${extra} · modified ${esc(formatDate(n.m))}</div>`;
    tooltipEl.hidden = false;
    moveTooltip(ev);
}

function moveTooltip(ev) {
    const pad = 14, tw = tooltipEl.offsetWidth || 200, th = tooltipEl.offsetHeight || 60;
    let x = ev.clientX + pad, y = ev.clientY + pad;
    if (x + tw > window.innerWidth - 8)  x = ev.clientX - tw - pad;
    if (y + th > window.innerHeight - 8) y = ev.clientY - th - pad;
    tooltipEl.style.left = x + 'px';
    tooltipEl.style.top  = y + 'px';
}

function hideTooltip() { tooltipEl.hidden = true; }

// ─── SVG text truncation ─────────────────────────────────────────────────────
function fitText(el, maxWidth) {
    if (maxWidth <= 8) { el.textContent = ''; return; }
    if (el.getComputedTextLength() <= maxWidth) return;
    let s = el.textContent;
    // Binary search the longest prefix that fits with an ellipsis.
    let lo = 0, hi = s.length;
    while (lo < hi) {
        const mid = (lo + hi + 1) >> 1;
        el.textContent = s.slice(0, mid) + '…';
        if (el.getComputedTextLength() <= maxWidth) lo = mid; else hi = mid - 1;
    }
    el.textContent = lo > 0 ? s.slice(0, lo) + '…' : '';
}

// ─── Treemap rendering ───────────────────────────────────────────────────────
const MAX_NODES = 3000;
const HEADER_H  = 20;

function renderTreemap() {
    const node = state.current;
    const container = $('treemap-container');
    container.querySelectorAll(':scope > svg, :scope > .free-space-panel').forEach(e => e.remove());
    $('empty-state').hidden = !!node;
    setSelected(null);
    if (!node) return;

    const W = container.clientWidth, H = container.clientHeight;
    if (W <= 0 || H <= 0) return;

    let svgX = 0, svgW = W;
    if (state.freeSpace && state.scan.totalDisk > 0) {
        const used = state.scan.root.s, total = state.scan.totalDisk;
        const usedRatio = Math.max(0.08, Math.min(used / total, 0.97));
        svgW = Math.floor(W * usedRatio);
        svgX = W - svgW;
        const panel = document.createElement('div');
        panel.className = 'free-space-panel';
        panel.style.cssText = `width:${svgX}px;height:${H}px;`;
        panel.innerHTML =
            `<div><b>${formatPct(state.scan.freeDisk, total)}</b>free` +
            `<div>${formatSize(state.scan.freeDisk)}</div></div>`;
        container.appendChild(panel);
    }

    const svgEl = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svgEl.setAttribute('width', svgW);
    svgEl.setAttribute('height', H);
    svgEl.style.cssText = `position:absolute;left:${svgX}px;top:0;`;
    container.appendChild(svgEl);

    const root = d3.hierarchy(node, d => d.c)
        .sum(d => (!d.c || d.c.length === 0) ? Math.max(d.s, 1) : 0)
        .sort((a, b) => b.value - a.value);

    d3.treemap()
        .tile(d3.treemapSquarify.ratio(1.2))
        .size([svgW, H])
        .paddingOuter(3)
        .paddingTop(HEADER_H)
        .paddingInner(2)
        .round(true)(root);

    // Raise the visibility threshold until the node count is manageable.
    let threshold = 3, nodes;
    do {
        nodes = root.descendants().filter(d =>
            d.depth > 0 && (d.x1 - d.x0) >= threshold && (d.y1 - d.y0) >= threshold);
        threshold += 2;
    } while (nodes.length > MAX_NODES && threshold < 60);

    const viewTotal = root.value;
    const cell = d3.select(svgEl).selectAll('g.cell')
        .data(nodes)
        .join('g')
        .attr('class', d => 'cell' + (d.data.d && d.data.c && d.data.c.length ? ' dir' : ''))
        .attr('transform', d => `translate(${d.x0},${d.y0})`);

    cell.append('rect')
        .attr('width',  d => Math.max(0, d.x1 - d.x0))
        .attr('height', d => Math.max(0, d.y1 - d.y0))
        .attr('rx', 3)
        .attr('fill', d => cellColor(d))
        .style('fill-opacity', d => d.children ? 'var(--dir-alpha)' : 1)
        .on('click', (ev, d) => {
            ev.stopPropagation();
            clearTimeout(clickTimer);
            clickTimer = setTimeout(() => { clickTimer = null; setSelected(d === state.selected ? null : d); }, 200);
        })
        .on('dblclick', (ev, d) => {
            ev.stopPropagation();
            clearTimeout(clickTimer); clickTimer = null;
            zoomIntoNode(d);
        })
        .on('mouseover', (ev, d) => showTooltip(ev, d, viewTotal))
        .on('mousemove', moveTooltip)
        .on('mouseout', hideTooltip);

    cell.filter(d => (d.x1 - d.x0) >= 24 && (d.y1 - d.y0) >= 15)
        .append('text')
        .attr('class', 'name')
        .attr('x', 5).attr('y', 14)
        .text(d => d.data.n)
        .each(function (d) { fitText(this, d.x1 - d.x0 - 10); });

    cell.filter(d => !d.children && (d.x1 - d.x0) >= 48 && (d.y1 - d.y0) >= 30)
        .append('text')
        .attr('class', 'size')
        .attr('x', 5).attr('y', 27)
        .text(d => formatSize(d.value))
        .each(function (d) { fitText(this, d.x1 - d.x0 - 10); });

    // Click on empty space deselects.
    svgEl.addEventListener('click', () => setSelected(null));
}

// Show the "building" overlay, let the browser paint, then render.
function renderWithOverlay() {
    $('progress-title').textContent = 'Building treemap…';
    $('progress-counts').hidden = true;
    $('progress-current').textContent = '';
    $('btn-cancel-scan').hidden = true;
    $('progress-overlay').hidden = false;
    requestAnimationFrame(() => requestAnimationFrame(() => {
        renderTreemap();
        $('progress-overlay').hidden = true;
    }));
}

// ─── Selection ────────────────────────────────────────────────────────────────
function setSelected(d) {
    state.selected = d;
    d3.selectAll('.cell rect')
        .attr('stroke', n => n === d ? 'var(--text)' : null)
        .attr('stroke-width', n => n === d ? 2 : null);
    const el = $('status-selected');
    if (d) {
        const n = d.data;
        el.textContent = `${n.n} — ${formatSize(d.value)}` + (n.d ? ` · ${(n.c || []).length} items` : '');
    } else {
        el.textContent = '';
    }
    updateButtons();
}

function canZoomInto(n) { return !!(n && n.d && n.c && n.c.length); }

function zoomIntoNode(d) {
    if (!d || !canZoomInto(d.data)) return;
    state.navStack.push(d.data);
    state.current = d.data;
    refreshView();
}

// ─── Navigation ───────────────────────────────────────────────────────────────
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
    renderTreemap();
    updateBreadcrumb();
    updateButtons();
    updateTitle();
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
    $('btn-zoom-out').disabled   = atRoot;
    $('btn-zoom-full').disabled  = atRoot;
    $('btn-zoom-in').disabled    = !(state.selected && canZoomInto(state.selected.data));
}

function updateTitle() {
    document.title = state.current ? `${state.current.n} — SpaceMonger` : 'SpaceMonger';
}

// ─── Status & info bars ──────────────────────────────────────────────────────
function updateStatusBar() {
    const s = state.scan;
    if (!s) return;
    $('status-files').textContent = s.files.toLocaleString();
    $('status-dirs').textContent  = s.dirs.toLocaleString();

    const space = $('status-space');
    if (s.totalDisk > 0) {
        const used = s.totalDisk - s.freeDisk;
        const pct  = used / s.totalDisk * 100;
        const fill = $('usage-fill');
        fill.style.width = pct.toFixed(1) + '%';
        fill.className = 'usage-fill' + (pct >= 95 ? ' crit' : pct >= 85 ? ' warn' : '');
        $('usage-pct').textContent    = pct.toFixed(0) + '%';
        $('status-free').textContent  = formatSize(s.freeDisk);
        $('status-total').textContent = formatSize(s.totalDisk);
        space.hidden = false;
    } else {
        space.hidden = true;
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

// ─── Scan result handling ────────────────────────────────────────────────────
function applyScanResult(msg) {
    state.scan = {
        path: msg.path, root: msg.root,
        files: msg.files || 0, dirs: msg.dirs || 0,
        totalDisk: msg.totalDisk || 0, freeDisk: msg.freeDisk || 0,
        apparentSize: msg.apparentSize || 0,
        scannedAt: msg.scannedAt || 0, durationMs: msg.durationMs || 0,
    };
    state.navStack = [state.scan.root];
    state.current  = state.scan.root;
    updateStatusBar();
    updateInfoBar();
    updateBreadcrumb();
    updateButtons();
    updateTitle();
}

// ─── Scanning (SSE) ───────────────────────────────────────────────────────────
function startScan(path) {
    stopScan();
    showProgress(path);

    const es = new EventSource(`/api/scan?path=${encodeURIComponent(path)}`);
    state.evtSource = es;
    let finished = false;

    es.onmessage = e => {
        const msg = JSON.parse(e.data);
        if (msg.status === 'scanning') { updateProgress(msg); return; }
        if (msg.status === 'building') {
            $('progress-title').textContent = 'Building treemap…';
            $('progress-counts').hidden = true;
            $('progress-current').textContent = '';
            $('btn-cancel-scan').hidden = true;
            return;
        }
        if (msg.status === 'done') {
            finished = true;
            es.close();
            state.evtSource = null;
            if (msg.error || !msg.root) {
                hideProgress();
                toast('Scan failed: ' + (msg.error || 'no data'), 'error', 6000);
                return;
            }
            applyScanResult(msg);
            renderWithOverlay();
        }
    };

    es.onerror = () => {
        if (finished) return;
        es.close();
        state.evtSource = null;
        hideProgress();
        if (state.authRequired) {
            // A 401 on the stream means the session expired.
            fetch('/api/auth').then(r => r.json()).then(st => {
                if (!st.authenticated) { showLogin(); return; }
                toast('Scan failed — connection lost.', 'error');
            }).catch(() => toast('Scan failed — connection lost.', 'error'));
        } else {
            toast('Scan failed — connection lost.', 'error');
        }
    };
}

// Closing the EventSource disconnects the request; the server cancels the scan.
function stopScan() {
    if (state.evtSource) { state.evtSource.close(); state.evtSource = null; }
}

function showProgress(path) {
    $('progress-title').textContent = 'Scanning…';
    $('progress-counts').hidden = false;
    $('btn-cancel-scan').hidden = false;
    updateProgress({ files: 0, dirs: 0, current: path });
    $('progress-overlay').hidden = false;
}

function hideProgress() { $('progress-overlay').hidden = true; }

function updateProgress(msg) {
    $('progress-files').textContent = (msg.files || 0).toLocaleString();
    $('progress-dirs').textContent  = (msg.dirs  || 0).toLocaleString();
    if (msg.current) $('progress-current').textContent = msg.current;
}

// ─── Open drive dialog ────────────────────────────────────────────────────────
async function openDriveDialog() {
    let mounts;
    try {
        const resp = await fetch('/api/mounts');
        if (resp.status === 401) { showLogin(); return; }
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        mounts = await resp.json();
    } catch (e) {
        toast('Could not load drives: ' + e.message, 'error');
        return;
    }

    const list = $('mounts-list');
    list.innerHTML = '';
    if (!mounts.length) {
        list.innerHTML = '<p class="muted" style="padding:14px">No mounted filesystems found.</p>';
    }
    mounts.forEach(m => {
        const used = m.total - m.free;
        const pct  = m.total > 0 ? used / m.total * 100 : 0;
        const cls  = pct >= 95 ? ' crit' : pct >= 85 ? ' warn' : '';
        const b = document.createElement('button');
        b.className = 'mount-item';
        b.innerHTML =
            `<svg class="icon"><use href="#i-drive"/></svg>` +
            `<div class="mount-path">${esc(m.path)}</div>` +
            `<div class="mount-pct">${pct.toFixed(0)}%</div>` +
            `<span class="usage"><span class="usage-fill${cls}" style="width:${pct.toFixed(1)}%"></span></span>` +
            `<div class="mount-meta">${esc(m.device)} · ${esc(m.fstype)} · ` +
            `${formatSize(used)} used of ${formatSize(m.total)} · ${formatSize(m.free)} free</div>`;
        b.addEventListener('click', () => { closeModal('open-modal'); startScan(m.path); });
        list.appendChild(b);
    });
    $('open-modal').hidden = false;
    const first = list.querySelector('.mount-item');
    if (first) first.focus();
}

function closeModal(id) { $(id).hidden = true; }
function anyModalOpen() { return !$('open-modal').hidden || !$('about-modal').hidden; }

// ─── Free space toggle ────────────────────────────────────────────────────────
function toggleFreeSpace() {
    state.freeSpace = !state.freeSpace;
    $('btn-free-space').classList.toggle('active', state.freeSpace);
    if (state.current) renderTreemap();
}

// ─── Auth ─────────────────────────────────────────────────────────────────────
function showLogin() {
    stopScan();
    hideProgress();
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
            await restoreLastScan();
            return;
        }
        const body = await resp.json().catch(() => ({}));
        if (resp.status === 429) {
            errEl.textContent = `Too many attempts — try again in ${body.retryAfter || 60}s.`;
        } else {
            errEl.textContent = 'Incorrect password.';
        }
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
    state.scan = null; state.current = null; state.navStack = [];
    renderTreemap();
    updateBreadcrumb();
    $('status-space').hidden = true;
    showLogin();
}

// ─── Restore previous scan after page load ───────────────────────────────────
async function restoreLastScan() {
    try {
        const resp = await fetch('/api/data');
        if (resp.status === 401) { showLogin(); return; }
        if (!resp.ok) { refreshView(); return; }
        const msg = await resp.json();
        if (msg.root) { applyScanResult(msg); renderWithOverlay(); } else { refreshView(); }
    } catch (e) {
        refreshView();
    }
}

// ─── Keyboard ─────────────────────────────────────────────────────────────────
document.addEventListener('keydown', e => {
    const tag = document.activeElement && document.activeElement.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA') return;
    if (e.key === 'Escape') {
        if (anyModalOpen()) { closeModal('open-modal'); closeModal('about-modal'); }
        else if (!$('progress-overlay').hidden) { /* ignore */ }
        else setSelected(null);
        return;
    }
    if (anyModalOpen() || !$('progress-overlay').hidden || $('app').hidden) return;
    switch (e.key) {
        case 'Backspace': case 'ArrowLeft': e.preventDefault(); zoomOut(); break;
        case 'Enter':     zoomIntoNode(state.selected); break;
        case 'Home':      zoomFull(); break;
        case 'r': case 'R':
            if (!e.ctrlKey && !e.metaKey && state.scan) startScan(state.scan.path);
            break;
        case 'o': case 'O':
            if (!e.ctrlKey && !e.metaKey) openDriveDialog();
            break;
    }
});

// ─── Resize ───────────────────────────────────────────────────────────────────
let resizeTimer = null;
window.addEventListener('resize', () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => { if (state.current) renderTreemap(); }, 150);
});

// ─── Init ─────────────────────────────────────────────────────────────────────
function bindEvents() {
    $('login-form').addEventListener('submit', doLogin);
    $('btn-open').addEventListener('click', openDriveDialog);
    $('btn-open-empty').addEventListener('click', openDriveDialog);
    $('btn-reload').addEventListener('click', () => { if (state.scan) startScan(state.scan.path); });
    $('btn-zoom-full').addEventListener('click', zoomFull);
    $('btn-zoom-in').addEventListener('click', () => zoomIntoNode(state.selected));
    $('btn-zoom-out').addEventListener('click', zoomOut);
    $('btn-free-space').addEventListener('click', toggleFreeSpace);
    $('btn-theme').addEventListener('click', toggleTheme);
    $('btn-about').addEventListener('click', () => { $('about-modal').hidden = false; });
    $('btn-logout').addEventListener('click', doLogout);
    $('btn-cancel-scan').addEventListener('click', () => { stopScan(); hideProgress(); toast('Scan cancelled'); });
    $('info-bar-close').addEventListener('click', () => { $('info-bar').hidden = true; });

    document.querySelectorAll('[data-close]').forEach(el =>
        el.addEventListener('click', () => closeModal(el.dataset.close)));
    document.querySelectorAll('.modal').forEach(el =>
        el.addEventListener('click', e => { if (e.target === el) el.hidden = true; }));
}

async function init() {
    initTooltip();
    bindEvents();

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
    await restoreLastScan();
}

document.addEventListener('DOMContentLoaded', init);
