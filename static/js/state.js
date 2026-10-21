'use strict';

// Shared application state. Modules read and mutate it directly; the
// render functions in nav.js / treemap.js / panel.js derive the UI from it.
const state = {
    scan:         null,   // { id, path, root, files, dirs, totalDisk, freeDisk, apparentSize, scannedAt, durationMs }
    navStack:     [],     // zoom history: FileNode[] (scan root first)
    current:      null,   // node currently displayed as treemap root
    selected:     null,   // selected FileNode (data node)
    selectedRel:  '',     // its path relative to the scan root
    freeSpace:    false,
    authRequired: false,
    config:       { allowDelete: false, historySize: 5 },
    evtSource:    null,   // active EventSource during a scan
    filter:       { exts: [], minSize: 0, age: 'any', hide: false },
    search:       '',
    panelTab:     'contents',
    renderIndex:  new Map(), // FileNode -> d3 node for the current render
    compare:      null,   // last compare result { a, b, summary, changes }
};

const $ = id => document.getElementById(id);
