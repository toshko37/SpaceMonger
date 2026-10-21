'use strict';

function formatSize(bytes) {
    if (!bytes) return '0 B';
    const neg = bytes < 0;
    bytes = Math.abs(bytes);
    const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
    const v = bytes / Math.pow(1024, i);
    return (neg ? '-' : '') + (i === 0 ? v.toFixed(0) : v.toFixed(v >= 100 ? 0 : 1)) + ' ' + units[i];
}

function formatDelta(bytes) {
    return (bytes > 0 ? '+' : '') + formatSize(bytes);
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

function debounce(fn, ms) {
    let t = null;
    return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms); };
}

function toast(msg, kind = 'info', ms = 4000) {
    const el = document.createElement('div');
    el.className = 'toast' + (kind === 'error' ? ' error' : '');
    el.textContent = msg;
    $('toasts').appendChild(el);
    setTimeout(() => el.remove(), ms);
}

async function copyText(text) {
    try {
        await navigator.clipboard.writeText(text);
        toast('Copied: ' + text, 'info', 2500);
    } catch (e) {
        // Clipboard API needs a secure context; fall back to a prompt.
        window.prompt('Copy path:', text);
    }
}

function downloadURL(url) {
    const a = document.createElement('a');
    a.href = url;
    a.download = '';
    document.body.appendChild(a);
    a.click();
    a.remove();
}

function relJoin(rel, name) { return rel ? rel + '/' + name : name; }

function extOf(name) {
    const i = name.lastIndexOf('.');
    return i > 0 && i < name.length - 1 ? name.slice(i + 1).toLowerCase() : '';
}

function isDirNode(n) { return !!(n && n.d); }
function hasChildren(n) { return !!(n && n.c && n.c.length); }
