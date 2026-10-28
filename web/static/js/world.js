// Your world: the seed engine's page-side API.
//
//   import { World } from './world.js';
//   const w = World.open(seed);                 // the same handle for the same seed
//   w.on('sites', ev => ...);                   // info, map, sectors, rivers, sites, progress, done, error
//   await w.ready;                              // resolves on done
//   w.near(x, z, 3, s => s.prefab === 'Crypt2') // nearest sites
//   w.countOf('Crypt2')                         // how many were placed
//   w.start()                                   // the StartTemple, or 0,0
//   await w.point(x, z)                         // biome, height, depth, forest, sector names
//
// One worker, world-worker.js, serves every handle; it runs world.wasm and
// caches each computed seed in IndexedDB by seed and engine version, so a
// seed costs twenty-odd seconds once per browser and a few milliseconds
// after. A late subscriber gets what already arrived, immediately, in the
// order it came.
//
// Sites are {rule, prefab, x, z, y}: x east, z north, y the height the
// placement sampled. The seed itself is a page-wide lens: seedFromPage(),
// setSeed() and clearSeed() read and write ?seed= and localStorage, and
// setSeed dispatches 'vegvisir:seed' on window for panels to redraw.

const WORKER_URL = new URL('./world-worker.js', import.meta.url);
const ENGINE_BASE = new URL('../world/', import.meta.url).href;
const STORAGE_KEY = 'vegvisir.seed';

// The height plane's encoding and the grid's radius, as the engine's info
// message states them; these are the defaults until it does.
const HEIGHT_OFFSET = 512, HEIGHT_SCALE = 16, MAP_RADIUS = 12288, WATER = 30;

// Biome indices as the biome plane carries them (worldmap.BiomeIndex: the
// flag's bit plus one), by slug.
export const BIOME_INDEX = {
  meadows: 1, swamp: 2, mountain: 3, 'black-forest': 4, plains: 5,
  ashlands: 6, 'deep-north': 7, ocean: 9, mistlands: 10,
};
export const BIOME_SLUGS = [];
for (const [slug, i] of Object.entries(BIOME_INDEX)) BIOME_SLUGS[i] = slug;

// The flat palette, for minimaps and legends: by slug, and by plane index.
export const FLAT_COLOURS = {
  meadows: '#7c9e4a', 'black-forest': '#3b5233', swamp: '#5e543d', mountain: '#c9d3dc',
  plains: '#baa860', mistlands: '#4a4658', ashlands: '#823e2f', 'deep-north': '#9fb7c9',
  ocean: '#2e4f7a', none: '#0b0d12',
};

let worker = null;
let engineVersion = null;
const handles = new Map();   // seed → Handle
const points = new Map();    // id → {resolve, reject}
let pointID = 0;

function getWorker() {
  if (worker) return worker;
  worker = new Worker(WORKER_URL);
  worker.onmessage = e => dispatch(e.data);
  worker.onerror = e => {
    const message = 'world worker: ' + (e.message || 'failed');
    for (const h of handles.values()) if (!h.done) h._fail(message);
  };
  worker.postMessage({ type: 'init', base: ENGINE_BASE });
  return worker;
}

function dispatch(m) {
  if (!m || typeof m !== 'object') return;
  if (m.type === 'engine') {
    engineVersion = m.version;
    return;
  }
  if (m.type === 'point' || m.type === 'tile' || (m.type === 'error' && m.id !== undefined)) {
    const p = points.get(m.id);
    if (p) {
      points.delete(m.id);
      if (m.type === 'error') p.reject(new Error(m.message));
      else p.resolve(m);
    }
    return;
  }
  const h = m.seed !== undefined ? handles.get(m.seed) : null;
  if (h) h._receive(m);
  else if (m.type === 'error') console.error('world:', m.message);
}

const EVENTS = ['info', 'map', 'sectors', 'rivers', 'sites', 'progress', 'done', 'error'];

class Handle {
  constructor(seed, mapSize) {
    this.seed = seed;
    this.mapSize = mapSize;
    this.info = null;
    this.map = null;         // {size, heights, biomes, mask}
    this.sectors = null;     // {size, ids, list:[{id, biome, names, x, z}]}
    this.rivers = null;      // {rivers, streams}
    this.report = null;      // [{rule, prefab, wanted, placed}] on done
    this.progress = null;    // the last progress message
    this.done = false;
    this.error = null;
    this.took = 0;
    this.cached = false;
    this._batches = [];      // each sites message, enriched
    this._sites = [];        // every site so far, flat
    this._byPrefab = new Map();
    this._listeners = new Map();
    for (const ev of EVENTS) this._listeners.set(ev, new Set());
    this.ready = new Promise((resolve, reject) => { this._resolve = resolve; this._reject = reject; });
    this.ready.catch(() => {});
  }

  // on subscribes fn to one event and replays what already arrived; the
  // return value unsubscribes.
  on(ev, fn) {
    const set = this._listeners.get(ev);
    if (!set) throw new Error(`World: no event ${ev}`);
    set.add(fn);
    this._replay(ev, fn);
    return () => set.delete(fn);
  }

  off(ev, fn) {
    const set = this._listeners.get(ev);
    if (set) set.delete(fn);
  }

  _replay(ev, fn) {
    const call = arg => { try { fn(arg, this); } catch (err) { console.error('world listener:', err); } };
    switch (ev) {
      case 'info': if (this.info) call(this.info); break;
      case 'map': if (this.map) call(this.map); break;
      case 'sectors': if (this.sectors) call(this.sectors); break;
      case 'rivers': if (this.rivers) call(this.rivers); break;
      case 'sites': for (const b of this._batches) call(b); break;
      case 'progress': if (this.progress) call(this.progress); break;
      case 'done': if (this.done) call(this._doneEvent()); break;
      case 'error': if (this.error) call(this.error); break;
    }
  }

  _emit(ev, arg) {
    for (const fn of Array.from(this._listeners.get(ev))) {
      try { fn(arg, this); } catch (err) { console.error('world listener:', err); }
    }
  }

  _doneEvent() {
    return { seed: this.seed, report: this.report, took: this.took, placed: this._sites.length, cached: this.cached };
  }

  _fail(message) {
    this.error = { seed: this.seed, message };
    this._emit('error', this.error);
    if (!this.done) this._reject(new Error(message));
  }

  _receive(m) {
    switch (m.type) {
      case 'progress':
        this.progress = m;
        this._emit('progress', m);
        break;
      case 'info':
        this.info = m;
        this._emit('info', m);
        break;
      case 'map':
        this.map = { size: m.size, heights: m.heights, biomes: m.biomes, mask: m.mask };
        this._emit('map', this.map);
        break;
      case 'sectors':
        this.sectors = { size: m.size, ids: m.ids, list: m.list };
        this._emit('sectors', this.sectors);
        break;
      case 'rivers':
        this.rivers = { rivers: m.rivers, streams: m.streams };
        this._emit('rivers', this.rivers);
        break;
      case 'sites': {
        const xyz = m.xyz;
        const sites = new Array(xyz.length / 3);
        for (let i = 0, j = 0; j < xyz.length; i++, j += 3) {
          sites[i] = { rule: m.rule, prefab: m.prefab, x: xyz[j], z: xyz[j + 1], y: xyz[j + 2] };
        }
        const batch = { rule: m.rule, prefab: m.prefab, index: m.index, total: m.total, sites, placed: m.placed, elapsed: m.elapsed };
        this._batches.push(batch);
        for (const s of sites) this._sites.push(s);
        let list = this._byPrefab.get(m.prefab);
        if (!list) this._byPrefab.set(m.prefab, list = []);
        for (const s of sites) list.push(s);
        if (m.rule !== m.prefab) {
          let byRule = this._byPrefab.get(m.rule);
          if (!byRule) this._byPrefab.set(m.rule, byRule = []);
          for (const s of sites) byRule.push(s);
        }
        this.progress = { type: 'progress', seed: this.seed, phase: 'sites', index: m.index + 1, total: m.total, placed: m.placed, elapsed: m.elapsed };
        this._emit('sites', batch);
        this._emit('progress', this.progress);
        break;
      }
      case 'done':
        this.report = m.report;
        this.took = m.took;
        this.cached = !!m.cached;
        this.done = true;
        this._emit('done', this._doneEvent());
        this._resolve(this);
        break;
      case 'error':
        this._fail(m.message);
        break;
    }
  }

  // sites is every site placed so far (all of them after ready), in the
  // placement's order: prioritised rules first.
  sites() { return this._sites; }

  // byPrefab is the sites of one prefab (or rule name), [] for none.
  byPrefab(prefab) { return this._byPrefab.get(prefab) || []; }

  // near is the n nearest sites to x, z, closest first, each with its
  // distance in metres; filter(site) narrows them, or is a prefab name.
  near(x, z, n = 1, filter = null) {
    const pool = typeof filter === 'string' ? this.byPrefab(filter) : this._sites;
    const test = typeof filter === 'function' ? filter : null;
    const found = [];
    for (const s of pool) {
      if (test && !test(s)) continue;
      const d = Math.hypot(s.x - x, s.z - z);
      if (found.length < n) {
        found.push({ site: s, d });
        if (found.length === n) found.sort((a, b) => a.d - b.d);
      } else if (d < found[n - 1].d) {
        let i = n - 1;
        while (i > 0 && found[i - 1].d > d) { found[i] = found[i - 1]; i--; }
        found[i] = { site: s, d };
      }
    }
    if (found.length < n) found.sort((a, b) => a.d - b.d);
    return found.map(f => Object.assign({ distance: f.d }, f.site));
  }

  // countOf is how many of a prefab (or rule) were placed so far.
  countOf(prefab) { return this.byPrefab(prefab).length; }

  // wantedOf is how many the game's table asks for, from the report; null
  // before done.
  wantedOf(prefab) {
    if (!this.report) return null;
    let n = 0, hit = false;
    for (const r of this.report) if (r.prefab === prefab || r.rule === prefab) { n += r.wanted; hit = true; }
    return hit ? n : 0;
  }

  // start is where the player spawns: the StartTemple, or the world's
  // centre until it is placed.
  start() {
    const s = this.byPrefab('StartTemple')[0];
    return s ? { x: s.x, z: s.z, y: s.y } : { x: 0, z: 0, y: 0 };
  }

  // point asks the engine about one position.
  point(x, z) {
    const id = ++pointID;
    return new Promise((resolve, reject) => {
      points.set(id, { resolve, reject });
      getWorker().postMessage({ type: 'point', seed: this.seed, x: +x, z: +z, id });
    });
  }

  // tile asks the engine for a close-up of the ground: n samples a side
  // over span metres centred on cx, cz, row zero north, as {n, cx, cz,
  // span, heights: Float32Array (metres), biomes: Uint8Array, mask:
  // Uint8Array RGBA}. Half a second or so for 320 a side; the requests
  // queue behind an open in progress.
  tile(cx, cz, span, n = 256) {
    const id = ++pointID;
    return new Promise((resolve, reject) => {
      points.set(id, { resolve, reject });
      getWorker().postMessage({ type: 'tile', seed: this.seed, cx: +cx, cz: +cz, span: +span, n: n | 0, id });
    });
  }

  // heightAt and biomeAt read the map plane the page has, without the
  // engine: nearest texel, null before the map arrives or past the grid.
  heightAt(x, z) {
    const i = this._texel(x, z);
    if (i < 0) return null;
    const info = this.info || {};
    return this.map.heights[i] / (info.heightScale || HEIGHT_SCALE) - (info.heightOffset || HEIGHT_OFFSET);
  }

  biomeAt(x, z) {
    const i = this._texel(x, z);
    if (i < 0) return null;
    return BIOME_SLUGS[this.map.biomes[i]] || null;
  }

  _texel(x, z) {
    if (!this.map) return -1;
    const R = this.info ? this.info.radius : MAP_RADIUS;
    const n = this.map.size;
    const tx = Math.floor((x + R) / (2 * R) * n), ty = Math.floor((R - z) / (2 * R) * n);
    if (tx < 0 || ty < 0 || tx >= n || ty >= n) return -1;
    return ty * n + tx;
  }
}

export const World = {
  // open returns the handle for a seed, starting the engine on first use.
  // The same seed gives the same handle; mapSize applies to the first open
  // (the default, 1024, is what the cache holds).
  open(seed, { mapSize = 1024 } = {}) {
    seed = String(seed == null ? '' : seed).trim();
    if (!seed) throw new Error('World.open: a seed is required');
    let h = handles.get(seed);
    if (h) return h;
    h = new Handle(seed, mapSize);
    handles.set(seed, h);
    getWorker().postMessage({ type: 'open', seed, mapSize });
    return h;
  },

  // handle is the open handle for a seed, or null.
  handle(seed) { return handles.get(String(seed)) || null; },

  // engineVersion is the engine's version once the worker has reported it.
  get engineVersion() { return engineVersion; },

  distance,
  bearing,
  formatDistance,
  seedFromPage,
  setSeed,
  clearSeed,
  drawMiniMap,
};

// distance is the metres between two things with x and z.
export function distance(a, b) { return Math.hypot(a.x - b.x, a.z - b.z); }

// bearing is the compass point from x0, z0 towards x1, z1: N is +z.
const POINTS = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW'];
export function bearing(x0, z0, x1, z1) {
  const a = Math.atan2(x1 - x0, z1 - z0) * 180 / Math.PI;   // 0 north, 90 east
  const i = Math.round(((a % 360) + 360) % 360 / 45) % 8;
  return POINTS[i];
}

// formatDistance is "820 m" under a kilometre, "1.4 km" under ten, "12 km"
// beyond.
export function formatDistance(m) {
  m = Math.abs(+m) || 0;
  if (m < 1000) return `${Math.round(m)} m`;
  if (m < 10000) return `${(m / 1000).toFixed(1)} km`;
  return `${Math.round(m / 1000)} km`;
}

// seedFromPage is the page's seed: ?seed= in the URL, else the last one
// remembered, else null.
export function seedFromPage() {
  try {
    const q = new URLSearchParams(location.search).get('seed');
    if (q && q.trim()) return q.trim();
  } catch (err) { /* no location */ }
  try {
    const s = localStorage.getItem(STORAGE_KEY);
    if (s && s.trim()) return s.trim();
  } catch (err) { /* storage blocked */ }
  return null;
}

// setSeed remembers a seed for every page and tells this one.
export function setSeed(seed) {
  seed = String(seed == null ? '' : seed).trim();
  if (!seed) { clearSeed(); return; }
  try { localStorage.setItem(STORAGE_KEY, seed); } catch (err) { /* storage blocked */ }
  window.dispatchEvent(new CustomEvent('vegvisir:seed', { detail: { seed } }));
}

// clearSeed forgets the remembered seed.
export function clearSeed() {
  try { localStorage.removeItem(STORAGE_KEY); } catch (err) { /* storage blocked */ }
  window.dispatchEvent(new CustomEvent('vegvisir:seed', { detail: { seed: null } }));
}

// drawMiniMap paints the biome plane in the flat palette onto a 2D canvas:
// span metres across the canvas's width centred on cx, cz (the whole world
// by default), water where the height is under sea level as the ocean colour
// darkened by depth, then the markers as dots. Returns false until the map
// has arrived. The canvas's own width and height are the pixels drawn.
export function drawMiniMap(canvas, handle, { cx = 0, cz = 0, span = null, markers = [], dotRadius = 4 } = {}) {
  const map = handle && handle.map;
  if (!map) return false;
  const info = handle.info || {};
  const R = info.radius || MAP_RADIUS;
  const worldR = info.worldRadius || 10500;
  const water = info.water != null ? info.water : WATER;
  const hScale = info.heightScale || HEIGHT_SCALE, hOff = info.heightOffset || HEIGHT_OFFSET;
  const W = canvas.width, H = canvas.height;
  if (!W || !H) return false;
  if (span == null) span = 2 * worldR * 1.02 * Math.max(1, W / H);
  const mpp = span / W;
  const ctx = canvas.getContext('2d');
  const img = ctx.createImageData(W, H);
  const px = img.data;
  const n = map.size;
  const rgb = Object.create(null);
  for (const [slug, hex] of Object.entries(FLAT_COLOURS)) rgb[slug] = [parseInt(hex.slice(1, 3), 16), parseInt(hex.slice(3, 5), 16), parseInt(hex.slice(5, 7), 16)];
  const byIndex = [];
  for (let i = 0; i < 16; i++) byIndex[i] = rgb[BIOME_SLUGS[i]] || rgb.none;
  const ocean = rgb.ocean, none = rgb.none;
  const left = cx - span / 2, top = cz + (H * mpp) / 2;
  const scale = n / (2 * R);
  for (let py = 0; py < H; py++) {
    const wz = top - (py + 0.5) * mpp;
    const ty = Math.floor((R - wz) * scale);
    for (let pxi = 0; pxi < W; pxi++) {
      const wx = left + (pxi + 0.5) * mpp;
      const tx = Math.floor((wx + R) * scale);
      let c = none;
      let r, g, b;
      if (tx >= 0 && ty >= 0 && tx < n && ty < n && wx * wx + wz * wz <= worldR * worldR) {
        const i = ty * n + tx;
        const bi = map.biomes[i];
        if (bi === 0) {
          c = none;
        } else {
          const h = map.heights[i] / hScale - hOff;
          if (h < water) {
            // The sea, darker the deeper: full colour at the shore, 45% at
            // sixty metres down.
            const k = 1 - 0.55 * Math.min(1, (water - h) / 60);
            r = ocean[0] * k; g = ocean[1] * k; b = ocean[2] * k;
            c = null;
          } else {
            c = byIndex[bi];
          }
        }
      }
      if (c) { r = c[0]; g = c[1]; b = c[2]; }
      const o = (py * W + pxi) * 4;
      px[o] = r; px[o + 1] = g; px[o + 2] = b; px[o + 3] = 255;
    }
  }
  ctx.putImageData(img, 0, 0);
  for (const m of markers || []) {
    const sx = (m.x - left) / mpp, sy = (top - m.z) / mpp;
    const rad = m.size || dotRadius;
    ctx.beginPath();
    ctx.arc(sx, sy, rad, 0, Math.PI * 2);
    ctx.fillStyle = m.color || '#fff';
    ctx.fill();
    ctx.lineWidth = Math.max(1, rad / 3);
    ctx.strokeStyle = 'rgba(0,0,0,0.7)';
    ctx.stroke();
  }
  return true;
}

export default World;
