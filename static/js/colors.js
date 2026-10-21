'use strict';

// ─── Colour modes ─────────────────────────────────────────────────────────────
// classic : original SpaceMonger palette by absolute depth (structure).
// type    : what kind of data a file is (3 groups + neutral "other").
// age     : how long since the file was modified (one hue, older = stronger).
// Folders are coloured by depth in classic mode and neutral in the others.

const COLOR_MODES = ['classic', 'type', 'age'];

// Exact GDI colours from the original Windows SpaceMonger. Used unchanged in
// both themes: the tiles touch and carry black text, so the canvas reads as
// one surface (no glowing tiles on a dark background).
const CLASSIC_COLORS = [
    '#ff7f7f', '#ffbf7f', '#ffff00', '#7fff7f', '#7fffff', '#bfbfff', '#bfbfbf', '#ff7fff',
    '#ffffbf', '#bfffbf', '#bfbf3f', '#7fbf7f', '#7fbfbf', '#9f9fff', '#9f9f9f', '#ffbfbf',
];

// File-type groups. In a treemap any two cells can touch, so the palette was
// validated on all pairs: only three hues stay distinguishable (including
// colour-vision deficiencies); everything else is neutral gray.
const TYPE_GROUPS = [
    { label: 'Media',
      re: /\.(mp4|mkv|mov|avi|webm|m4v|wmv|flv|mp3|flac|wav|ogg|opus|m4a|aac|jpe?g|png|gif|webp|heic|heif|nef|cr2|cr3|arw|dng|raw|tiff?|bmp|psd)$/i },
    { label: 'Archives & disk images',
      re: /\.(zip|tar|gz|tgz|xz|txz|bz2|zst|7z|rar|iso|img|qcow2|vmdk|vdi|vhdx?|ova|deb|rpm|snap|flatpak|appimage|bak|dump|box)$/i },
    { label: 'Logs & databases',
      re: /\.(log|journal|db|sqlite3?|ibd|frm|myd|wal|mdb|accdb|csv|tsv|parquet|npy|ndjson|sd)$/i },
];
const TYPE_COLORS = { dark: ['#2f65aa', '#a04626', '#1e7656'], light: ['#5591df', '#f2865e', '#5abe91'] };
const TYPE_OTHER  = { dark: '#2c3038', light: '#e3e6eb' };

const AGE_BUCKETS = [
    { label: '< 1 month', days: 30 }, { label: '< 6 months', days: 182 },
    { label: '< 1 year', days: 365 }, { label: '< 3 years', days: 1095 }, { label: '3+ years', days: Infinity },
];
const AGE_COLORS = {
    dark:  ['#281d13', '#402a12', '#5a370e', '#754401', '#915100'],
    light: ['#f8ece1', '#edd2b8', '#e2b890', '#d69e67', '#c98438'],
};
const NEUTRAL_DIR = { dark: '#161a21', light: '#f1f3f6' };

function currentTheme() { return document.documentElement.dataset.theme === 'light' ? 'light' : 'dark'; }

function typeIndex(name) {
    for (let i = 0; i < TYPE_GROUPS.length; i++) if (TYPE_GROUPS[i].re.test(name)) return i;
    return -1;
}

function ageIndex(mtime) {
    const days = (Date.now() / 1000 - mtime) / 86400;
    return AGE_BUCKETS.findIndex(b => days < b.days);
}

// fillFor returns the fill of a rendered d3 node in the active mode.
function fillFor(d, absDepth) {
    const theme = currentTheme();
    switch (state.colorMode) {
        case 'type': {
            if (d.children) return NEUTRAL_DIR[theme];
            const i = typeIndex(d.data.n);
            return i < 0 ? TYPE_OTHER[theme] : TYPE_COLORS[theme][i];
        }
        case 'age':
            return d.children ? NEUTRAL_DIR[theme] : AGE_COLORS[theme][ageIndex(d.data.m)];
        default:
            return CLASSIC_COLORS[(absDepth - 1) % CLASSIC_COLORS.length];
    }
}

// ─── Text colour per fill (WCAG contrast, cached) ─────────────────────────────
const TEXT_DARK = '#000000', TEXT_LIGHT = '#eef0f4';
const textCache = new Map();

function relLum(hex) {
    const c = [1, 3, 5].map(i => {
        const v = parseInt(hex.slice(i, i + 2), 16) / 255;
        return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
}

function contrastRatio(a, b) {
    const [hi, lo] = [relLum(a), relLum(b)].sort((x, y) => y - x);
    return (hi + 0.05) / (lo + 0.05);
}

function textFor(fill) {
    let t = textCache.get(fill);
    if (!t) {
        t = contrastRatio(fill, TEXT_DARK) >= contrastRatio(fill, TEXT_LIGHT) ? TEXT_DARK : TEXT_LIGHT;
        textCache.set(fill, t);
    }
    return t;
}

// ─── Legend & mode switch ─────────────────────────────────────────────────────
function renderLegend() {
    const el = $('legend');
    const theme = currentTheme();
    let items = null;
    if (state.colorMode === 'type') {
        items = TYPE_GROUPS.map((g, i) => [g.label, TYPE_COLORS[theme][i]]).concat([['Other', TYPE_OTHER[theme]]]);
    } else if (state.colorMode === 'age') {
        items = AGE_BUCKETS.map((b, i) => [b.label, AGE_COLORS[theme][i]]);
    }
    el.hidden = !items || !state.current;
    if (items) {
        el.innerHTML = items.map(([label, c]) => `<span><i style="background:${c}"></i>${esc(label)}</span>`).join('');
    }
}

function setColorMode(mode) {
    state.colorMode = COLOR_MODES.includes(mode) ? mode : 'classic';
    $('color-mode').value = state.colorMode;
    try { localStorage.setItem('sm-color', state.colorMode); } catch (e) {}
    if (state.current) renderTreemap();
    renderLegend();
}

function loadColorMode() {
    let mode = 'classic';
    try { mode = localStorage.getItem('sm-color') || mode; } catch (e) {}
    state.colorMode = COLOR_MODES.includes(mode) ? mode : 'classic';
    $('color-mode').value = state.colorMode;
}
