'use strict';

// ─── State ────────────────────────────────────────────────────────────────────
let scanMeta      = null;   // { root, files, dirs, totalDisk, freeDisk }
let currentNode   = null;   // currently displayed root node
let navStack      = [];     // navigation history: array of FileNode
let freeSpaceMode = false;  // free-space overlay toggle
let lastScanPath  = null;   // path of last scan (for Reload)
let evtSource     = null;   // active EventSource (or null)
let tooltip       = null;   // tooltip DOM element

// ─── Size Formatting ──────────────────────────────────────────────────────────
function formatSize(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(
        Math.floor(Math.log(Math.abs(bytes)) / Math.log(1024)),
        units.length - 1
    );
    const v = bytes / Math.pow(1024, i);
    return (i === 0 ? v.toFixed(0) : v.toFixed(1)) + '\u00a0' + units[i];
}

// ─── HTML Escaping ────────────────────────────────────────────────────────────
function esc(s) {
    return String(s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

// ─── Color Scale (SpaceMonger: green=small → yellow → red=large) ──────────────
function getColor(value, maxValue) {
    if (!maxValue || maxValue === 0) return '#66bb6a';
    const ratio = Math.min(value / maxValue, 1);
    // d3.interpolateRdYlGn: 0=red, 1=green — we reverse: small=green, large=red
    return d3.interpolateRdYlGn(1 - ratio);
}

// ─── Tooltip ──────────────────────────────────────────────────────────────────
function initTooltip() {
    tooltip = document.createElement('div');
    tooltip.className = 'tooltip';
    tooltip.style.display = 'none';
    document.body.appendChild(tooltip);
}

function showTooltip(event, node, value) {
    const mtime = node.mtime
        ? new Date(node.mtime).toLocaleString()
        : '—';
    tooltip.innerHTML =
        `<strong>${esc(node.name)}</strong>` +
        `<div>Size: ${formatSize(value != null ? value : node.size)}</div>` +
        `<div>Modified: ${mtime}</div>` +
        `<div>${node.isDir ? 'Directory' : 'File'}</div>`;
    tooltip.style.display = 'block';
    positionTooltip(event);
}

function moveTooltip(event) { positionTooltip(event); }
function hideTooltip()       { tooltip.style.display = 'none'; }

function positionTooltip(event) {
    const x  = event.clientX + 14;
    const y  = event.clientY - 10;
    const tw = tooltip.offsetWidth  || 180;
    const th = tooltip.offsetHeight || 80;
    tooltip.style.left = Math.min(x, window.innerWidth  - tw - 6) + 'px';
    tooltip.style.top  = Math.min(y, window.innerHeight - th - 6) + 'px';
}

// ─── SVG Text Truncation ─────────────────────────────────────────────────────
function truncateSVGText(el, maxWidth) {
    if (maxWidth <= 0) { el.textContent = ''; return; }
    const full = el.textContent;
    if (el.getComputedTextLength() <= maxWidth) return;
    let s = full;
    while (s.length > 0 && el.getComputedTextLength() > maxWidth) {
        s = s.slice(0, -1);
        el.textContent = s + '\u2026'; // ellipsis
    }
    if (el.getComputedTextLength() > maxWidth) el.textContent = '';
}

// ─── Treemap Rendering ───────────────────────────────────────────────────────
function renderTreemap(node) {
    const container = document.getElementById('treemap-container');
    container.innerHTML = '';
    if (!node) return;

    const W = container.clientWidth;
    const H = container.clientHeight;
    if (W <= 0 || H <= 0) return;

    // ── Free Space panel (left side) ──────────────────────────────────────────
    let svgX = 0;
    let svgW = W;

    if (freeSpaceMode && scanMeta && scanMeta.totalDisk > 0) {
        const usedBytes  = scanMeta.root ? scanMeta.root.size : 0;
        const totalBytes = scanMeta.totalDisk;
        const usedRatio  = Math.max(0.05, Math.min(usedBytes / totalBytes, 0.99));
        svgW = Math.floor(W * usedRatio);
        svgX = W - svgW;

        const freeBytes = scanMeta.freeDisk || 0;
        const freePct   = (freeBytes / totalBytes * 100).toFixed(1);

        const panel = document.createElement('div');
        panel.className = 'free-space-panel';
        panel.style.cssText = `left:0;top:0;width:${svgX}px;height:${H}px;`;
        panel.innerHTML = `
            <div class="free-space-info">
                <div>&lt;Free Space: ${freePct}%&gt;</div>
                <div>${formatSize(freeBytes)} Free</div>
                <div>Files Total: ${(scanMeta.files || 0).toLocaleString()}</div>
                <div>Folders Total: ${(scanMeta.dirs || 0).toLocaleString()}</div>
            </div>`;
        container.appendChild(panel);
    }

    // ── SVG for treemap ───────────────────────────────────────────────────────
    const svgEl = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svgEl.setAttribute('width',  svgW);
    svgEl.setAttribute('height', H);
    svgEl.style.cssText = `position:absolute;left:${svgX}px;top:0;`;
    container.appendChild(svgEl);

    const root = d3.hierarchy(node)
        .sum(d => (!d.children || d.children.length === 0)
            ? Math.max(d.size, 1)
            : 0)
        .sort((a, b) => b.value - a.value);

    d3.treemap()
        .tile(d3.treemapSquarify)
        .size([svgW, H])
        .paddingOuter(2)
        .paddingTop(18)
        .paddingInner(1)
        .round(true)(root);

    const maxValue = root.value;
    const nodes    = root.descendants().filter(d => d.depth > 0);
    const svg      = d3.select(svgEl);

    const cell = svg.selectAll('g.cell')
        .data(nodes)
        .join('g')
        .attr('class', 'cell')
        .attr('transform', d => `translate(${d.x0},${d.y0})`);

    // Background rectangle
    cell.append('rect')
        .attr('width',  d => Math.max(0, d.x1 - d.x0 - 0.5))
        .attr('height', d => Math.max(0, d.y1 - d.y0 - 0.5))
        .attr('fill',   d => getColor(d.value, maxValue))
        .attr('stroke', 'rgba(0,0,0,0.25)')
        .attr('stroke-width', 0.5)
        .style('cursor', d => (d.data.isDir && d.data.children) ? 'pointer' : 'default')
        .on('click',     (ev, d) => { ev.stopPropagation(); handleClick(d); })
        .on('mouseover', (ev, d) => showTooltip(ev, d.data, d.value))
        .on('mousemove', moveTooltip)
        .on('mouseout',  hideTooltip);

    // Name label
    cell.filter(d => (d.x1 - d.x0) >= 18 && (d.y1 - d.y0) >= 13)
        .append('text')
        .attr('x', 3).attr('y', 12)
        .attr('font-size', '11px')
        .attr('font-family', "'Courier New',monospace")
        .attr('fill', '#000')
        .attr('pointer-events', 'none')
        .text(d => d.data.name)
        .each(function(d) { truncateSVGText(this, d.x1 - d.x0 - 6); });

    // Size label (only for larger cells)
    cell.filter(d => (d.x1 - d.x0) >= 45 && (d.y1 - d.y0) >= 27)
        .append('text')
        .attr('x', 3).attr('y', 24)
        .attr('font-size', '10px')
        .attr('font-family', "'Courier New',monospace")
        .attr('fill', '#333')
        .attr('pointer-events', 'none')
        .text(d => formatSize(d.value))
        .each(function(d) { truncateSVGText(this, d.x1 - d.x0 - 6); });
}

function handleClick(d) {
    if (d.data.isDir && d.data.children && d.data.children.length > 0) {
        zoomInto(d.data);
    }
}

// ─── Navigation ───────────────────────────────────────────────────────────────
function zoomInto(node) {
    navStack.push(node);
    currentNode = node;
    renderTreemap(currentNode);
    updateBreadcrumb();
    updateButtons();
}

function zoomOut() {
    if (navStack.length <= 1) return;
    navStack.pop();
    currentNode = navStack[navStack.length - 1];
    renderTreemap(currentNode);
    updateBreadcrumb();
    updateButtons();
}

function zoomFull() {
    if (!scanMeta) return;
    navStack    = [scanMeta.root];
    currentNode = scanMeta.root;
    renderTreemap(currentNode);
    updateBreadcrumb();
    updateButtons();
}

function updateBreadcrumb() {
    const bc = document.getElementById('breadcrumb');
    bc.innerHTML = navStack.map((node, i) => {
        if (i === navStack.length - 1) {
            return `<span class="bc-current">${esc(node.name)}</span>`;
        }
        return `<span class="bc-link" data-depth="${i}">${esc(node.name)}</span>` +
               `<span class="bc-sep"> › </span>`;
    }).join('');

    bc.querySelectorAll('.bc-link[data-depth]').forEach(el => {
        el.addEventListener('click', () => {
            const depth = parseInt(el.dataset.depth, 10);
            navStack    = navStack.slice(0, depth + 1);
            currentNode = navStack[navStack.length - 1];
            renderTreemap(currentNode);
            updateBreadcrumb();
            updateButtons();
        });
    });
}

function updateButtons() {
    const atRoot = navStack.length <= 1;
    document.getElementById('btn-zoom-out').disabled  = atRoot;
    document.getElementById('btn-zoom-full').disabled = atRoot;
}

// ─── Status Bar ───────────────────────────────────────────────────────────────
function updateStatusBar() {
    if (!scanMeta) return;
    document.getElementById('status-files').textContent = (scanMeta.files || 0).toLocaleString();
    document.getElementById('status-dirs').textContent  = (scanMeta.dirs  || 0).toLocaleString();

    if (scanMeta.totalDisk > 0) {
        const usedBytes = scanMeta.totalDisk - scanMeta.freeDisk;
        document.getElementById('status-used').textContent = formatSize(usedBytes);
        document.getElementById('status-free').textContent = formatSize(scanMeta.freeDisk);
        document.getElementById('status-space').style.display = '';
    }
    document.getElementById('titlebar').textContent =
        `${currentNode?.name || '/'} — ` +
        (scanMeta.totalDisk > 0
            ? `${formatSize(scanMeta.totalDisk)} Total — ${formatSize(scanMeta.freeDisk)} Free — `
            : '') +
        'SpaceMonger';
}

// ─── Scanning (SSE) ───────────────────────────────────────────────────────────
function startScan(path) {
    if (evtSource) { evtSource.close(); evtSource = null; }
    lastScanPath = path;
    showProgress();
    updateProgressUI({ files: 0, dirs: 0, current: path });

    const es = new EventSource(`/api/scan?path=${encodeURIComponent(path)}`);
    evtSource = es;

    es.onmessage = function(e) {
        const msg = JSON.parse(e.data);
        updateProgressUI(msg);

        if (msg.status === 'done') {
            es.close();
            evtSource = null;

            scanMeta = {
                root:      msg.root,
                files:     msg.files,
                dirs:      msg.dirs,
                totalDisk: msg.totalDisk || 0,
                freeDisk:  msg.freeDisk  || 0,
            };

            navStack    = [scanMeta.root];
            currentNode = scanMeta.root;

            hideProgress();
            renderTreemap(currentNode);
            updateBreadcrumb();
            updateButtons();
            updateStatusBar();
        }
    };

    es.onerror = function() {
        es.close();
        evtSource = null;
        hideProgress();
        showError('Scan failed — check server logs or permissions.');
    };
}

function showProgress() {
    document.getElementById('progress-overlay').style.display = 'flex';
}
function hideProgress() {
    document.getElementById('progress-overlay').style.display = 'none';
}
function updateProgressUI(msg) {
    document.getElementById('progress-files').textContent = (msg.files || 0).toLocaleString();
    document.getElementById('progress-dirs').textContent  = (msg.dirs  || 0).toLocaleString();
    if (msg.current) {
        const s   = msg.current;
        const max = 58;
        document.getElementById('progress-current').textContent =
            s.length > max ? '…' + s.slice(-(max - 1)) : s;
    }
}

// ─── Open Drive Dialog ────────────────────────────────────────────────────────
async function openDriveDialog() {
    let mounts;
    try {
        const resp = await fetch('/api/mounts');
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        mounts = await resp.json();
    } catch (e) {
        showError('Could not load drives: ' + e.message);
        return;
    }

    const list = document.getElementById('mounts-list');
    if (!mounts || mounts.length === 0) {
        list.innerHTML = '<p style="padding:12px;color:#666;font-size:11px">No mountpoints found.</p>';
    } else {
        list.innerHTML = mounts.map(m => `
            <div class="mount-item" data-path="${esc(m.path)}">
                <div class="mount-path">${esc(m.path)}</div>
                <div class="mount-info">
                    ${esc(m.device)} &nbsp;|&nbsp;
                    ${esc(m.fstype)} &nbsp;|&nbsp;
                    ${formatSize(m.total)} total &nbsp;|&nbsp;
                    ${formatSize(m.free)} free
                </div>
            </div>`).join('');

        list.querySelectorAll('.mount-item').forEach(el => {
            el.addEventListener('click', () => {
                closeModal('open-modal');
                startScan(el.dataset.path);
            });
        });
    }
    document.getElementById('open-modal').style.display = 'flex';
}

function closeModal(id) {
    document.getElementById(id).style.display = 'none';
}

// ─── Free Space Toggle ────────────────────────────────────────────────────────
function toggleFreeSpace() {
    freeSpaceMode = !freeSpaceMode;
    document.getElementById('btn-free-space').classList.toggle('active', freeSpaceMode);
    if (currentNode) renderTreemap(currentNode);
}

// ─── Auth ─────────────────────────────────────────────────────────────────────
function showLoginOverlay() {
    document.getElementById('login-overlay').style.display = 'flex';
    document.getElementById('app').style.display = 'none';
    setTimeout(() => document.getElementById('login-password').focus(), 50);
}

function hideLoginOverlay() {
    document.getElementById('login-overlay').style.display = 'none';
    document.getElementById('app').style.display = 'flex';
}

async function doLogin() {
    const password = document.getElementById('login-password').value;
    const errEl    = document.getElementById('login-error');
    errEl.textContent = '';
    try {
        const resp = await fetch('/api/auth', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ password }),
        });
        if (resp.ok) {
            hideLoginOverlay();
            bindEvents();
        } else {
            errEl.textContent = 'Incorrect password.';
            document.getElementById('login-password').select();
        }
    } catch (e) {
        errEl.textContent = 'Connection error.';
    }
}

// ─── Error Toast ──────────────────────────────────────────────────────────────
function showError(msg) {
    const div = document.createElement('div');
    div.style.cssText =
        'position:fixed;bottom:40px;left:50%;transform:translateX(-50%);' +
        'background:#cc0000;color:#fff;padding:8px 18px;font-size:12px;' +
        'z-index:9999;border:1px solid #900;box-shadow:2px 2px 6px rgba(0,0,0,.4);';
    div.textContent = msg;
    document.body.appendChild(div);
    setTimeout(() => div.remove(), 4000);
}

// ─── Resize Handling ──────────────────────────────────────────────────────────
let resizeTimer = null;
window.addEventListener('resize', () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => {
        if (currentNode) renderTreemap(currentNode);
    }, 150);
});

// ─── Keyboard Shortcuts ───────────────────────────────────────────────────────
document.addEventListener('keydown', e => {
    if (e.key === 'Backspace' || e.key === 'ArrowLeft') {
        if (navStack.length > 1 &&
            document.activeElement.tagName !== 'INPUT') {
            zoomOut();
        }
    }
    if (e.key === 'Escape') closeModal('open-modal');
});

// ─── Initialization ───────────────────────────────────────────────────────────
async function init() {
    initTooltip();

    // Check auth: if API returns 401, show login overlay
    const resp = await fetch('/api/mounts').catch(() => null);
    if (!resp || resp.status === 401) {
        showLoginOverlay();
        return;
    }
    document.getElementById('app').style.display = 'flex';
    document.getElementById('login-overlay').style.display = 'none';
    bindEvents();
}

function bindEvents() {
    document.getElementById('btn-open')
        .addEventListener('click', openDriveDialog);

    document.getElementById('btn-reload')
        .addEventListener('click', () => {
            if (lastScanPath) startScan(lastScanPath);
        });

    document.getElementById('btn-zoom-full')
        .addEventListener('click', zoomFull);

    document.getElementById('btn-zoom-in')
        .addEventListener('click', () => {
            // zoom into the clicked node — double-click also works
        });

    document.getElementById('btn-zoom-out')
        .addEventListener('click', zoomOut);

    document.getElementById('btn-free-space')
        .addEventListener('click', toggleFreeSpace);

    document.getElementById('close-open-modal')
        .addEventListener('click', () => closeModal('open-modal'));

    document.getElementById('open-modal')
        .addEventListener('click', e => {
            if (e.target === e.currentTarget) closeModal('open-modal');
        });

    document.getElementById('login-btn')
        .addEventListener('click', doLogin);

    document.getElementById('login-password')
        .addEventListener('keydown', e => { if (e.key === 'Enter') doLogin(); });
}

document.addEventListener('DOMContentLoaded', init);
