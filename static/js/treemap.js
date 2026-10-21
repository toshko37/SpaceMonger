'use strict';

// ─── Depth palette ────────────────────────────────────────────────────────────
// Colour is a function of absolute depth in the scanned tree, so a folder
// keeps its colour when you zoom in and out.
const DEPTH_COLORS = [
    '#f87171', '#fb923c', '#facc15', '#4ade80', '#2dd4bf', '#38bdf8',
    '#818cf8', '#c084fc', '#f472b6', '#a3e635', '#fbbf24', '#34d399',
];

function cellColor(d) {
    const abs = (state.navStack.length - 1) + d.depth; // depth 1 == child of view root
    return DEPTH_COLORS[(abs - 1) % DEPTH_COLORS.length];
}

// ─── Tooltip ──────────────────────────────────────────────────────────────────
let tooltipEl = null;

function initTooltip() {
    tooltipEl = document.createElement('div');
    tooltipEl.className = 'tooltip';
    tooltipEl.hidden = true;
    document.body.appendChild(tooltipEl);
}

function showTooltip(ev, d, viewTotal) {
    const n = d.data;
    const extra = isDirNode(n) ? `${(n.c ? n.c.length : 0).toLocaleString()} items` : 'File';
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
    const s = el.textContent;
    let lo = 0, hi = s.length;
    while (lo < hi) {
        const mid = (lo + hi + 1) >> 1;
        el.textContent = s.slice(0, mid) + '…';
        if (el.getComputedTextLength() <= maxWidth) lo = mid; else hi = mid - 1;
    }
    el.textContent = lo > 0 ? s.slice(0, lo) + '…' : '';
}

// ─── Rendering ────────────────────────────────────────────────────────────────
const MAX_NODES = 3000;
const HEADER_H  = 20;
let clickTimer  = null;

function renderTreemap() {
    const node = state.current;
    const container = $('treemap-container');
    container.querySelectorAll(':scope > svg, :scope > .free-space-panel').forEach(e => e.remove());
    $('empty-state').hidden = !!node;
    state.renderIndex = new Map();
    if (!node) { updateFilterStatus(0, 0); return; }

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

    // Filter / search: in "hide" mode non-matching files get zero weight and
    // vanish from the layout; otherwise they are drawn dimmed.
    const highlight = isHighlightActive();
    const hide = highlight && state.filter.hide;
    beginMatchPass();
    let matchCount = 0, matchSize = 0;
    const leafValue = d => {
        if (hasChildren(d)) return 0;
        const m = !highlight || nodeMatches(d);
        if (highlight && m && !isDirNode(d)) { matchCount++; matchSize += d.s; }
        if (hide && !m) return 0;
        return Math.max(d.s, 1);
    };

    const root = d3.hierarchy(node, d => d.c)
        .sum(leafValue)
        .sort((a, b) => b.value - a.value);

    d3.treemap()
        .tile(d3.treemapSquarify.ratio(1.2))
        .size([svgW, H])
        .paddingOuter(3)
        .paddingTop(HEADER_H)
        .paddingInner(2)
        .round(true)(root);

    let threshold = 3, nodes;
    do {
        nodes = root.descendants().filter(d =>
            d.depth > 0 && d.value > 0 && (d.x1 - d.x0) >= threshold && (d.y1 - d.y0) >= threshold);
        threshold += 2;
    } while (nodes.length > MAX_NODES && threshold < 60);

    nodes.forEach(d => state.renderIndex.set(d.data, d));
    updateFilterStatus(matchCount, matchSize);

    const viewTotal = root.value;
    const cell = d3.select(svgEl).selectAll('g.cell')
        .data(nodes)
        .join('g')
        .attr('class', d => {
            let c = 'cell';
            if (hasChildren(d.data)) c += ' dir';
            if (highlight && !hide) {
                if (!nodeMatches(d.data)) c += ' dim';
                else if (isSearchActive() && nameMatches(d.data)) c += ' hit';
            }
            return c;
        })
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
            clickTimer = setTimeout(() => {
                clickTimer = null;
                if (state.selected === d.data) selectNode(null, '');
                else selectNode(d.data, relOfD3(d));
            }, 200);
        })
        .on('dblclick', (ev, d) => {
            ev.stopPropagation();
            clearTimeout(clickTimer); clickTimer = null;
            zoomIntoNode(d.data);
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

    svgEl.addEventListener('click', () => selectNode(null, ''));
    applySelectionStyle();
}

function updateFilterStatus(count, size) {
    const el = $('status-filter');
    if (!isHighlightActive() || !state.current) { el.hidden = true; return; }
    el.textContent = `${count.toLocaleString()} matching · ${formatSize(size)}`;
    el.hidden = false;
}

// Show the "building" overlay, let the browser paint, then render.
function renderWithOverlay(title = 'Building treemap…') {
    $('progress-title').textContent = title;
    $('progress-counts').hidden = true;
    $('progress-current').textContent = '';
    $('btn-cancel-scan').hidden = true;
    $('progress-overlay').hidden = false;
    requestAnimationFrame(() => requestAnimationFrame(() => {
        refreshView();
        $('progress-overlay').hidden = true;
    }));
}

// ─── Selection ────────────────────────────────────────────────────────────────
// Selection is data-based (a FileNode plus its relative path), so items can
// be selected from the panel even when their cell is too small to draw.
function selectNode(node, rel) {
    state.selected = node;
    state.selectedRel = node ? rel : '';
    applySelectionStyle();
    const el = $('status-selected');
    el.textContent = node
        ? `${node.n} — ${formatSize(node.s)}` + (isDirNode(node) ? ` · ${(node.c || []).length} items` : '')
        : '';
    updateButtons();
    renderPanel();
}

function applySelectionStyle() {
    const sel = state.selected;
    d3.selectAll('.cell rect')
        .attr('stroke', d => d.data === sel ? 'var(--text)' : null)
        .attr('stroke-width', d => d.data === sel ? 2 : null);
}
