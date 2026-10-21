'use strict';

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
// Classic SpaceMonger geometry: tiles touch, each outlined by a 1px darker
// shade of its own fill, square corners, a header strip with the folder
// name, and a centred name / exact size / date block in file cells.
const MAX_NODES = 3000;
const HEADER_H  = 13;
const CELL_FONT = "Tahoma, Verdana, 'DejaVu Sans', 'Segoe UI', sans-serif";
const MONTHS    = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
let clickTimer  = null;

function classicDate(unixSec) {
    const d = new Date(unixSec * 1000), p = n => String(n).padStart(2, '0');
    return `${p(d.getDate())} ${MONTHS[d.getMonth()]} ${d.getFullYear()}  ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

const outlineCache = new Map();
function outlineFor(fill) {
    let o = outlineCache.get(fill);
    if (!o) { o = d3.color(fill).darker(1.6).formatHex(); outlineCache.set(fill, o); }
    return o;
}

// Widest glyph advance of the cell font at 10-10.5px is ~6.5px; text that
// fits by that estimate skips the (layout-forcing) measurement.
const MAX_CHAR_W = 6.5;

function addText(g, x, y, anchor, size, color, text, maxWidth) {
    const t = g.append('text')
        .attr('x', x).attr('y', y).attr('text-anchor', anchor)
        .attr('font-size', size).attr('fill', color)
        .text(text).node();
    if (text.length * MAX_CHAR_W > maxWidth) fitText(t, maxWidth);
}

function renderTreemap() {
    const node = state.current;
    const container = $('treemap-container');
    container.querySelectorAll(':scope > svg, :scope > .free-space-panel').forEach(e => e.remove());
    $('empty-state').hidden = !!node;
    state.renderIndex = new Map();
    renderLegend();
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
    svgEl.setAttribute('shape-rendering', 'crispEdges');
    svgEl.setAttribute('font-family', CELL_FONT);
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
        .tile(d3.treemapSquarify)
        .size([svgW, H])
        .paddingOuter(2)
        .paddingTop(HEADER_H)
        .paddingInner(0)
        .round(true)(root);

    let threshold = 2, nodes;
    do {
        nodes = root.descendants().filter(d =>
            d.depth > 0 && d.value > 0 && (d.x1 - d.x0) >= threshold && (d.y1 - d.y0) >= threshold);
        threshold += 2;
    } while (nodes.length > MAX_NODES && threshold < 60);

    nodes.forEach(d => state.renderIndex.set(d.data, d));
    updateFilterStatus(matchCount, matchSize);

    const baseDepth = state.navStack.length - 1;
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
        .attr('transform', d => `translate(${d.x0},${d.y0})`)
        .each(function (d) { d._fill = fillFor(d, baseDepth + d.depth); });

    cell.append('rect')
        .attr('x', 0.5).attr('y', 0.5)
        .attr('width',  d => Math.max(0, d.x1 - d.x0 - 1))
        .attr('height', d => Math.max(0, d.y1 - d.y0 - 1))
        .attr('fill', d => d._fill)
        .attr('stroke', d => outlineFor(d._fill))
        .attr('stroke-width', 1)
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

    // Folder header strip: name, left aligned.
    cell.filter(d => d.children && (d.x1 - d.x0) >= 20)
        .each(function (d) {
            addText(d3.select(this), 3, 10, 'start', '10.5px', textFor(d._fill), d.data.n, d.x1 - d.x0 - 6);
        });

    // Files: centred name, exact size in bytes and modification date.
    cell.filter(d => !d.children && (d.x1 - d.x0) >= 22 && (d.y1 - d.y0) >= 13)
        .each(function (d) {
            const w = d.x1 - d.x0, h = d.y1 - d.y0, g = d3.select(this), color = textFor(d._fill);
            const lines = [d.data.n];
            if (w >= 70 && h >= 36) lines.push(d.data.s.toLocaleString('en-US') + ' bytes');
            if (w >= 70 && h >= 48) lines.push(classicDate(d.data.m));
            const lh = 11.5, y0 = h / 2 - (lines.length - 1) * lh / 2 + 3.5;
            lines.forEach((l, i) => addText(g, w / 2, y0 + i * lh, 'middle', '10px', color, l, w - 6));
        });

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

// The selected cell gets a 2px black frame, as in the original.
function applySelectionStyle() {
    const sel = state.selected;
    d3.selectAll('.cell rect')
        .attr('stroke', d => d.data === sel ? '#000' : outlineFor(d._fill))
        .attr('stroke-width', d => d.data === sel ? 2.5 : 1);
}
