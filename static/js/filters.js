'use strict';

// ─── Matching ─────────────────────────────────────────────────────────────────
function isFilterActive() {
    const f = state.filter;
    return f.exts.length > 0 || f.minSize > 0 || f.age !== 'any';
}
function isSearchActive() { return state.search.length > 0; }
function isHighlightActive() { return isFilterActive() || isSearchActive(); }

function nameMatches(n) {
    return !isSearchActive() || n.n.toLowerCase().includes(state.search);
}

// matchLeaf applies the filter and search terms to a single file.
function matchLeaf(n) {
    const f = state.filter;
    if (f.exts.length && !f.exts.includes(extOf(n.n))) return false;
    if (f.minSize > 0 && n.s < f.minSize) return false;
    if (f.age !== 'any') {
        const days = parseInt(f.age, 10);
        const ageDays = (Date.now() / 1000 - n.m) / 86400;
        if (days > 0 && ageDays > days) return false;
        if (days < 0 && ageDays < -days) return false;
    }
    return nameMatches(n);
}

// Per-render memo stamp so match results are computed once per node.
let matchStamp = 0;
function beginMatchPass() { matchStamp++; }

// nodeMatches: a file matches the filters; a directory matches when any
// descendant does, or when its own name matches a name-only search.
function nodeMatches(n) {
    if (n._ms === matchStamp) return n._mv;
    let m;
    if (!isDirNode(n)) {
        m = matchLeaf(n);
    } else {
        m = !isFilterActive() && isSearchActive() && nameMatches(n);
        if (n.c) for (const c of n.c) if (nodeMatches(c)) { m = true; }
    }
    n._ms = matchStamp;
    n._mv = m;
    return m;
}

// ─── Filter popover ───────────────────────────────────────────────────────────
function openFilterPopover() {
    const f = state.filter;
    $('filter-ext').value = f.exts.join(', ');
    const unitEl = $('filter-min-unit');
    let unit = parseInt(unitEl.value, 10);
    if (f.minSize > 0) {
        unit = [1073741824, 1048576, 1024].find(u => f.minSize % u === 0) || 1048576;
        unitEl.value = String(unit);
    }
    $('filter-min').value = f.minSize > 0 ? (f.minSize / unit) : '';
    $('filter-age').value = f.age;
    $('filter-hide').checked = f.hide;
    showPopover('filter-popover', $('btn-filter'));
    $('filter-ext').focus();
}

function applyFilterFromPopover() {
    const exts = $('filter-ext').value.split(/[,\s]+/)
        .map(e => e.trim().replace(/^\./, '').toLowerCase()).filter(Boolean);
    const min = parseFloat($('filter-min').value);
    const unit = parseInt($('filter-min-unit').value, 10);
    state.filter = {
        exts,
        minSize: isFinite(min) && min > 0 ? Math.round(min * unit) : 0,
        age: $('filter-age').value,
        hide: $('filter-hide').checked,
    };
    closePopovers();
    onFilterChanged();
}

function clearFilter() {
    state.filter = { exts: [], minSize: 0, age: 'any', hide: false };
    closePopovers();
    onFilterChanged();
}

function onFilterChanged() {
    const active = isFilterActive();
    $('btn-filter').classList.toggle('active', active);
    const n = state.filter.exts.length + (state.filter.minSize > 0 ? 1 : 0) + (state.filter.age !== 'any' ? 1 : 0);
    $('filter-badge').hidden = !active;
    $('filter-badge').textContent = n;
    if (state.current) { renderTreemap(); renderPanel(); }
}

// ─── Search box ───────────────────────────────────────────────────────────────
const onSearchInput = debounce(() => {
    state.search = $('search-input').value.trim().toLowerCase();
    $('tab-search').hidden = !isSearchActive();
    if (isSearchActive()) state.panelTab = 'search';
    else if (state.panelTab === 'search') state.panelTab = 'contents';
    if (state.current) { renderTreemap(); renderPanel(); }
}, 180);

function clearSearch() {
    $('search-input').value = '';
    state.search = '';
    $('tab-search').hidden = true;
    if (state.panelTab === 'search') state.panelTab = 'contents';
    if (state.current) { renderTreemap(); renderPanel(); }
}

// ─── Popover helpers ──────────────────────────────────────────────────────────
function showPopover(id, anchor) {
    closePopovers();
    const el = $(id);
    el.hidden = false;
    const r = anchor.getBoundingClientRect();
    let left = r.left;
    if (left + el.offsetWidth > window.innerWidth - 8) left = window.innerWidth - el.offsetWidth - 8;
    el.style.left = Math.max(8, left) + 'px';
    el.style.top = (r.bottom + 6) + 'px';
    el.dataset.anchor = anchor.id;
}

function closePopovers() {
    document.querySelectorAll('.popover').forEach(el => { el.hidden = true; });
}

function anyPopoverOpen() {
    return [...document.querySelectorAll('.popover')].some(el => !el.hidden);
}
