'use strict';

// ─── Scanning (SSE) ───────────────────────────────────────────────────────────
function startScan(path) {
    stopScan();
    closePopovers();
    showProgress(path);

    const es = new EventSource(`/api/scan?path=${encodeURIComponent(path)}`);
    state.evtSource = es;
    let finished = false;

    es.onmessage = e => {
        const msg = JSON.parse(e.data);
        if (msg.status === 'scanning') { updateProgress(msg); return; }
        if (msg.status === 'done') {
            finished = true;
            es.close();
            state.evtSource = null;
            if (msg.error || !msg.id) {
                hideProgress();
                toast('Scan failed: ' + (msg.error || 'no data'), 'error', 6000);
                return;
            }
            loadScanData(msg.id);
        }
    };

    es.onerror = () => {
        if (finished) return;
        es.close();
        state.evtSource = null;
        hideProgress();
        if (state.authRequired) {
            fetch('/api/auth').then(r => r.json()).then(st => {
                if (!st.authenticated) showLogin();
                else toast('Scan failed — connection lost.', 'error');
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

// loadScanData fetches a stored scan's tree (gzip-compressed by the
// server) and makes it the current view.
async function loadScanData(id) {
    showLoading('Loading tree…');
    try {
        const msg = await apiJSON('/api/data?id=' + encodeURIComponent(id));
        if (!msg.root) throw new Error(msg.error || 'empty result');
        applyScanResult(msg);
        clearSearch();
        renderWithOverlay();
    } catch (e) {
        hideProgress();
        toast('Could not load scan: ' + e.message, 'error', 6000);
    }
}

function showProgress(path) {
    $('progress-title').textContent = 'Scanning…';
    $('progress-counts').hidden = false;
    $('btn-cancel-scan').hidden = false;
    updateProgress({ files: 0, dirs: 0, current: path });
    $('progress-overlay').hidden = false;
}

function showLoading(title) {
    $('progress-title').textContent = title;
    $('progress-counts').hidden = true;
    $('progress-current').textContent = '';
    $('btn-cancel-scan').hidden = true;
    $('progress-overlay').hidden = false;
}

function hideProgress() { $('progress-overlay').hidden = true; }

function updateProgress(msg) {
    $('progress-files').textContent = (msg.files || 0).toLocaleString();
    $('progress-dirs').textContent  = (msg.dirs  || 0).toLocaleString();
    if (msg.current) $('progress-current').textContent = msg.current;
}

// ─── Restore previous scan after page load ───────────────────────────────────
async function restoreLastScan() {
    try {
        const resp = await apiFetch('/api/data');
        if (!resp.ok) { refreshView(); return; }
        const msg = await resp.json();
        if (msg.root) { applyScanResult(msg); renderWithOverlay(); } else { refreshView(); }
    } catch (e) {
        refreshView();
    }
}

// ─── Open drive dialog ────────────────────────────────────────────────────────
async function openDriveDialog() {
    let mounts;
    try {
        mounts = await apiJSON('/api/mounts');
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
function anyModalOpen() { return [...document.querySelectorAll('.modal')].some(el => !el.hidden); }
