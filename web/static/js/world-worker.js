// The seed engine's worker: world.wasm driven off the page's thread.
//
// A classic worker (importScripts, no modules) so wasm_exec.js loads as it
// ships. The page sends:
//
//   {type:'init', base}                   the URL of web/static/world/, with its slash
//   {type:'open', seed, mapSize}          compute, or replay from the cache
//   {type:'point', seed, x, z, id}        one position; answered with the same id
//
// and gets back the protocol of docs/architecture.md, relayed message by
// message as the engine emits it: info, map, sectors, rivers, sites (one per
// rule as placed), progress, done — or {type:'error', message}. Every reply
// carries the seed and mapSize it belongs to.
//
// A world is computed once per browser: on done the accumulated messages go
// into IndexedDB (database 'vegvisir-world', store 'worlds', key
// seed + '|' + engine version), and a later open of the same seed replays
// the record in the same order within a few milliseconds and hands the
// engine the sector plane (vegvisirWorld.adopt) so point() names sectors
// without rebuilding the biome map.
//
// vegvisirWorld.open runs to completion inside its call — twenty-odd seconds
// — so messages arriving meanwhile queue on this thread and are answered
// after; the page's handle serialises its own requests the same way.
'use strict';

const DB_NAME = 'vegvisir-world';
const STORE = 'worlds';
const DB_VERSION = 1;

let engineReady = null;   // Promise<void>: the wasm is instantiated and vegvisirReady fired
let engineVersion = '';
let queue = Promise.resolve();   // opens, one after another
let emitTo = null;        // where vegvisirEmit's messages go while an open runs

// vegvisirEmit is what the engine calls, synchronously, for each message of
// an open. The typed arrays in `transfer` are fresh buffers; a copy of each
// goes to the record and the originals go to the page.
self.vegvisirEmit = (message, transfer) => {
  if (emitTo) emitTo(message, transfer || []);
};

self.onmessage = e => {
  const m = e.data || {};
  try {
    switch (m.type) {
      case 'init':
        init(m.base);
        break;
      case 'open':
        queue = queue.then(() => open(m.seed, m.mapSize | 0)).catch(err => fail(m.seed, m.mapSize | 0, err));
        break;
      case 'point':
        point(m);
        break;
      default:
        self.postMessage({ type: 'error', message: `world-worker: unknown message type ${m.type}` });
    }
  } catch (err) {
    fail(m.seed, m.mapSize | 0, err);
  }
};

function fail(seed, mapSize, err, extra) {
  const message = err && err.message ? err.message : String(err);
  self.postMessage(Object.assign({ type: 'error', seed, mapSize, message }, extra || {}));
}

// init loads wasm_exec.js and world.wasm from under base, once.
function init(base) {
  if (engineReady) return;
  engineReady = (async () => {
    importScripts(base + 'wasm_exec.js');
    const go = new self.Go();
    const ready = new Promise(resolve => { self.vegvisirReady = resolve; });
    let result;
    const url = base + 'world.wasm';
    if (typeof WebAssembly.instantiateStreaming === 'function') {
      try {
        result = await WebAssembly.instantiateStreaming(fetch(url), go.importObject);
      } catch (err) {
        // A server without the wasm media type: fall back to bytes.
        result = null;
      }
    }
    if (!result) {
      const r = await fetch(url);
      if (!r.ok) throw new Error(`${url}: ${r.status}`);
      result = await WebAssembly.instantiate(await r.arrayBuffer(), go.importObject);
    }
    go.run(result.instance).catch(err => {
      self.postMessage({ type: 'error', message: 'world.wasm exited: ' + (err && err.message || err) });
    });
    await ready;
    engineVersion = self.vegvisirWorld.version();
    self.postMessage({ type: 'engine', version: engineVersion });
  })();
  engineReady.catch(err => fail(undefined, 0, err));
}

async function engine() {
  if (!engineReady) throw new Error('world-worker: not initialised; send {type:"init", base} first');
  await engineReady;
  return self.vegvisirWorld;
}

// open serves one seed: from the cache when it is there whole, else by
// computing it and caching the result.
async function open(seed, mapSize) {
  const api = await engine();
  if (typeof seed !== 'string' || seed === '') throw new Error('open: a seed is required');
  const key = seed + '|' + engineVersion;
  let cached = null;
  try {
    cached = await dbGet(key);
  } catch (err) {
    // No IndexedDB (private window, blocked storage): compute every time.
    cached = null;
  }
  if (cached && cached.complete && (!mapSize || cached.mapSize === mapSize)) {
    replay(api, cached);
    return;
  }
  const record = compute(api, seed, mapSize);
  if (record && record.complete) {
    try {
      await dbPut(key, record);
    } catch (err) {
      // Storage failed (quota, private mode): the page has its world anyway.
      self.postMessage({ type: 'progress', seed, mapSize: record.mapSize, phase: 'cache-failed', index: 0, total: 0, placed: record.placed, elapsed: record.took, message: String(err && err.message || err) });
    }
  }
}

// compute runs the engine, relaying each message as it arrives and keeping a
// copy of it for the record. Returns the record, complete when done arrived.
function compute(api, seed, mapSize) {
  const record = {
    seed, mapSize, engine: engineVersion, complete: false, stored: Date.now(),
    info: null, map: null, sectors: null, rivers: null, sites: [], done: null, placed: 0, took: 0,
  };
  const copy = a => (a && a.buffer ? a.slice() : a);
  emitTo = (m, transfer) => {
    switch (m.type) {
      case 'info':
        record.info = Object.assign({}, m, { palette: copy(m.palette) });
        if (m.mapSize) record.mapSize = m.mapSize;
        break;
      case 'map':
        record.map = { size: m.size, heights: copy(m.heights), biomes: copy(m.biomes), mask: copy(m.mask) };
        break;
      case 'sectors':
        record.sectors = { size: m.size, ids: copy(m.ids), list: m.list };
        break;
      case 'rivers':
        record.rivers = { rivers: m.rivers, streams: m.streams };
        break;
      case 'sites':
        record.sites.push({ rule: m.rule, prefab: m.prefab, index: m.index, total: m.total, xyz: copy(m.xyz) });
        break;
      case 'done':
        record.done = { report: m.report, took: m.took, placed: m.placed };
        record.placed = m.placed;
        record.took = m.took;
        record.complete = !!(record.info && record.map && record.sectors && record.rivers);
        break;
      case 'error':
        record.complete = false;
        break;
    }
    self.postMessage(m, transfer);
  };
  try {
    api.open(seed, mapSize);
  } finally {
    emitTo = null;
  }
  return record;
}

// replay sends a cached record in the engine's order and makes it current in
// the engine for point().
function replay(api, rec) {
  const seed = rec.seed, mapSize = rec.mapSize;
  const t0 = performance.now();
  const msg = (type, fields) => Object.assign({ type, seed, mapSize }, fields);
  const elapsed = () => Math.round(performance.now() - t0);
  self.postMessage(msg('progress', { phase: 'cache', index: 0, total: 0, placed: 0, elapsed: 0 }));
  self.postMessage(msg('info', rec.info));

  // adopt copies the plane into the engine, so the arrays can then be
  // transferred to the page rather than cloned.
  const names = rec.sectors.list.map(s => s.names);
  const adoptErr = api.adopt(seed, mapSize, rec.sectors.size, rec.sectors.ids, names);
  if (adoptErr) self.postMessage(msg('error', { message: 'adopt: ' + adoptErr, recoverable: true }));

  const map = rec.map;
  self.postMessage(msg('map', { size: map.size, heights: map.heights, biomes: map.biomes, mask: map.mask }),
    [map.heights.buffer, map.biomes.buffer, map.mask.buffer]);
  self.postMessage(msg('sectors', { size: rec.sectors.size, ids: rec.sectors.ids, list: rec.sectors.list }), [rec.sectors.ids.buffer]);
  self.postMessage(msg('rivers', rec.rivers));
  let placed = 0;
  for (const s of rec.sites) {
    placed += s.xyz.length / 3;
    self.postMessage(msg('sites', { rule: s.rule, prefab: s.prefab, index: s.index, total: s.total, xyz: s.xyz, placed, elapsed: elapsed() }), [s.xyz.buffer]);
  }
  self.postMessage(msg('done', { report: rec.done.report, took: rec.done.took, placed: rec.done.placed, cached: true, replayed: elapsed() }));
}

async function point(m) {
  try {
    const api = await engine();
    const p = api.point(String(m.seed), +m.x, +m.z);
    if (p && p.error) {
      fail(m.seed, 0, new Error(p.error), { id: m.id });
      return;
    }
    self.postMessage(Object.assign({ type: 'point', id: m.id, seed: m.seed }, p));
  } catch (err) {
    fail(m.seed, 0, err, { id: m.id });
  }
}

// IndexedDB, promisified: one store of records by key.

let dbPromise = null;

function db() {
  if (!dbPromise) {
    dbPromise = new Promise((resolve, reject) => {
      if (typeof indexedDB === 'undefined') {
        reject(new Error('no IndexedDB'));
        return;
      }
      const req = indexedDB.open(DB_NAME, DB_VERSION);
      req.onupgradeneeded = () => {
        const d = req.result;
        if (!d.objectStoreNames.contains(STORE)) d.createObjectStore(STORE);
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error || new Error('IndexedDB open failed'));
      req.onblocked = () => reject(new Error('IndexedDB open blocked'));
    });
    dbPromise.catch(() => { dbPromise = null; });
  }
  return dbPromise;
}

async function dbGet(key) {
  const d = await db();
  return new Promise((resolve, reject) => {
    const tx = d.transaction(STORE, 'readonly');
    const req = tx.objectStore(STORE).get(key);
    req.onsuccess = () => resolve(req.result || null);
    req.onerror = () => reject(req.error);
  });
}

async function dbPut(key, value) {
  const d = await db();
  return new Promise((resolve, reject) => {
    const tx = d.transaction(STORE, 'readwrite');
    tx.objectStore(STORE).put(value, key);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error || new Error('IndexedDB write aborted'));
  });
}
