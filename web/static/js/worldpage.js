// The World page: any seed as a map. The engine runs in a worker
// (world.js), the painter is the game's own map shader (worldmap.js);
// this file wires them to the page: the seed form, progress, the layer
// list built from the site's location catalogue, the click popover and
// the URL (?seed=&x=&z=&zoom=).

import { World } from './world.js';
import { WorldMap } from './worldmap.js';

const base = document.documentElement.dataset.base || '/';
const $ = (sel, el = document) => el.querySelector(sel);

const CATS = [
  { id: 'start', name: 'Spawn', color: '#ffffff', size: 34, on: true },
  { id: 'boss', name: 'Forsaken altars', color: '#e06a5a', size: 36, on: true },
  { id: 'trader', name: 'Traders', color: '#f0bb75', size: 32, on: true },
  { id: 'dungeon', name: 'Dungeons and caves', color: '#9fc4ff', size: 26, on: true },
  { id: 'camp', name: 'Camps and villages', color: '#dfa258', size: 26, on: true },
  { id: 'ruin', name: 'Ruins with loot', color: '#c9c0ad', size: 22, on: false },
  { id: 'runestone', name: 'Runestones', color: '#b7a2d8', size: 20, on: false },
  { id: 'other', name: 'Everything else', color: '#8fb28a', size: 18, on: false },
];
const BIOMES = [['meadows', 'Meadows', '#7c9e4a'], ['black-forest', 'Black Forest', '#3b5233'], ['swamp', 'Swamp', '#5e543d'], ['mountain', 'Mountain', '#c9d3dc'], ['plains', 'Plains', '#baa860'], ['mistlands', 'Mistlands', '#4a4658'], ['ashlands', 'Ashlands', '#823e2f'], ['deep-north', 'Deep North', '#9fb7c9'], ['ocean', 'Ocean', '#2e4f7a']];

const root = $('[data-world]');
if (root) init(root);

async function init(root) {
  const canvas = $('[data-world-canvas]', root);
  const form = $('[data-world-seed-form]', root);
  const input = $('[data-world-seed]', root);
  const status = $('[data-world-status]', root);
  const filterInput = $('[data-world-filter]', root);
  const layersEl = $('[data-world-layers]', root);
  const legend = $('[data-world-legend]', root);
  const empty = $('[data-world-empty]', root);

  legend.innerHTML = BIOMES.map(([, n, c]) => `<span><i style="background:${c}"></i>${n}</span>`).join('');

  const params = new URLSearchParams(location.search);
  const map = new WorldMap(canvas, { assetsBase: base + 'static/' });
  const catalogue = await (await fetch(base + 'world/locations.json')).json();
  const byPrefab = new Map(catalogue.map(l => [l.prefab, l]));
  const kinds = new Map(); // kind slug -> {name, cat, icon, url, prefabs:Set, count}
  for (const l of catalogue) {
    let k = kinds.get(l.kind);
    if (!k) kinds.set(l.kind, k = { slug: l.kind, name: l.name, cat: l.cat, icon: l.icon, url: l.url, prefabs: new Set(), count: 0, on: null });
    k.prefabs.add(l.prefab);
    if (!k.icon && l.icon) k.icon = l.icon;
  }
  const catOn = Object.fromEntries(CATS.map(c => [c.id, c.on]));
  const kindOn = new Map();
  let handle = null;
  let markers = [];
  let popover = null;

  const seedNow = params.get('seed') || World.seedFromPage() || '';
  input.value = seedNow;

  form.addEventListener('submit', e => {
    e.preventDefault();
    const s = input.value.trim();
    if (!s) return;
    World.setSeed(s);
    const u = new URL(location.href);
    u.searchParams.set('seed', s);
    u.searchParams.delete('x'); u.searchParams.delete('z'); u.searchParams.delete('zoom');
    history.replaceState(null, '', u);
    open(s);
  });
  window.addEventListener('vegvisir:seed', e => { if (e.detail.seed && e.detail.seed !== handle?.seed) { input.value = e.detail.seed; open(e.detail.seed); } });

  // Zoomed out, only the landmarks show (spawn, altars, traders); the
  // dungeons and camps come in at 2x, the rest at 5x, so the overview
  // reads as a map and not as a rash. A name typed in the filter shows
  // its places at any zoom.
  const minPriority = () => map.zoom < 2 ? 90 : map.zoom < 5 ? 60 : 0;
  const applyFilter = () => {
    const q = filterInput.value.trim().toLowerCase();
    const minPri = minPriority();
    map.setFilter(m => {
      const k = m.data.kindObj;
      if (q) return k.name.toLowerCase().includes(q) || m.data.prefab.toLowerCase().includes(q);
      if (kindOn.has(k.slug)) return kindOn.get(k.slug);
      return catOn[k.cat] && m.priority >= minPri;
    });
    paintLayers();
  };
  filterInput.addEventListener('input', applyFilter);
  let lastPri = minPriority();
  map.onView(() => { const p = minPriority(); if (p !== lastPri) { lastPri = p; applyFilter(); } });

  function paintLayers() {
    layersEl.innerHTML = '';
    for (const c of CATS) {
      const ks = [...kinds.values()].filter(k => k.cat === c.id && k.count > 0).sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
      if (!ks.length) continue;
      const head = document.createElement('li');
      const gated = catOn[c.id] && (c.id === 'dungeon' || c.id === 'camp' ? map.zoom < 2 : c.id === 'ruin' || c.id === 'runestone' || c.id === 'other' ? map.zoom < 5 : false);
      head.innerHTML = `<label><input type="checkbox" ${catOn[c.id] ? 'checked' : ''}><b style="color:${c.color}">${c.name}</b><span class="n">${gated ? 'zoom in · ' : ''}${ks.reduce((n, k) => n + k.count, 0)}</span></label>`;
      head.querySelector('input').addEventListener('change', ev => { catOn[c.id] = ev.target.checked; for (const k of ks) kindOn.delete(k.slug); applyFilter(); });
      layersEl.appendChild(head);
      for (const k of ks) {
        const li = document.createElement('li');
        const on = kindOn.has(k.slug) ? kindOn.get(k.slug) : catOn[c.id];
        li.style.paddingLeft = '18px';
        li.innerHTML = `<label><input type="checkbox" ${on ? 'checked' : ''}>${k.icon ? `<img src="${base}icons/${k.icon}.png" alt="">` : ''}<span>${k.name}</span><span class="n">${k.count}</span></label>`;
        li.querySelector('input').addEventListener('change', ev => { kindOn.set(k.slug, ev.target.checked); applyFilter(); });
        layersEl.appendChild(li);
      }
    }
  }

  function rebuildMarkers() {
    markers = [];
    for (const k of kinds.values()) k.count = 0;
    const sites = handle.sites();
    for (const s of sites) {
      const l = byPrefab.get(s.prefab);
      if (!l) continue;
      const k = kinds.get(l.kind);
      k.count++;
      const c = CATS.find(c => c.id === l.cat) || CATS[CATS.length - 1];
      markers.push({ x: s.x, z: s.z, img: l.icon ? base + 'icons/' + l.icon + '.png' : null, size: c.size, kind: l.kind, color: c.color, label: l.name, priority: l.pri, data: { prefab: s.prefab, kindObj: k, loc: l, site: s } });
    }
    map.setMarkers(markers);
    applyFilter();
  }

  function setStatus(text, frac) {
    status.innerHTML = '';
    status.textContent = text;
    if (frac != null) {
      const bar = document.createElement('span');
      bar.className = 'bar';
      bar.innerHTML = '<i></i>';
      bar.querySelector('i').style.width = Math.round(frac * 100) + '%';
      status.appendChild(bar);
    }
  }

  function open(seed) {
    empty.hidden = true;
    closePopover();
    handle = World.open(seed);
    map.setWorld(handle);
    setStatus('Reading the seed…', 0);
    handle.on('info', info => { setStatus(`World ${info.seedNumber}. Drawing the biomes…`, 0.05); });
    handle.on('map', () => setStatus('Placing the world…', 0.15));
    handle.on('progress', p => {
      if (p.total) setStatus(`Placing ${p.index + 1} of ${p.total}: ${p.placed} places`, 0.15 + 0.85 * (p.index + 1) / p.total);
    });
    let lastSites = 0;
    handle.on('sites', () => {
      // Draw what is placed so far, every so often: the bosses come first.
      const n = handle.sites().length;
      if (n - lastSites > 400 || n < 200) { lastSites = n; rebuildMarkers(); }
    });
    handle.on('error', e => setStatus('The seed engine failed: ' + e.message));
    handle.ready.then(() => {
      rebuildMarkers();
      const n = handle.sites().length;
      setStatus(`${n} places in this world.`);
      const x = parseFloat(params.get('x')), z = parseFloat(params.get('z')), zoom = parseFloat(params.get('zoom'));
      if (!Number.isNaN(x) && !Number.isNaN(z)) {
        map.centerOn(x, z, Number.isNaN(zoom) ? 8 : zoom);
        params.delete('x'); params.delete('z'); params.delete('zoom');
      }
    });
  }

  function closePopover() { if (popover) { popover.remove(); popover = null; } }

  map.onClick(async ({ x, z, marker }) => {
    closePopover();
    const start = handle ? handle.start() : { x: 0, z: 0 };
    const d = World.formatDistance(World.distance(start, { x, z }));
    const b = World.bearing(start.x, start.z, x, z);
    const pt = handle ? await handle.point(x, z).catch(() => null) : null;
    const biome = pt && pt.biome ? (BIOMES.find(bb => bb[0] === pt.biome) || [])[1] || pt.biome : '';
    const where = `${d} ${b} of spawn${biome ? ` · ${biome}` : ''}${pt && pt.height != null ? ` · ${Math.round(pt.height - 30)} m${pt.height < 30 ? ' (under water)' : ''}` : ''}${pt && pt.sector && pt.sector.length ? ` · ${pt.sector.join(', ')}` : ''}`;
    const pop = document.createElement('div');
    pop.className = 'world-panel world-pop';
    if (marker) {
      const l = marker.data.loc;
      pop.innerHTML = `<button class="close" aria-label="Close">×</button><div class="pop-head">${l.icon ? `<img src="${base}icons/${l.icon}.png" alt="">` : ''}<div><b>${l.name}</b><div class="meta">${marker.data.prefab}</div></div></div><p class="meta">${where}</p><p style="margin:0"><a href="${base}${l.url}">About this place →</a></p>`;
    } else {
      pop.innerHTML = `<button class="close" aria-label="Close">×</button><div class="pop-head"><div><b>${biome || 'Here'}</b><div class="meta">${Math.round(x)}, ${Math.round(z)}</div></div></div><p class="meta" style="margin:0">${where}</p>`;
    }
    pop.querySelector('.close').addEventListener('click', closePopover);
    root.appendChild(pop);
    popover = pop;
    place();
    function place() {
      const p = map.worldToScreen(x, z);
      pop.style.left = p.x + 'px';
      pop.style.top = p.y + 'px';
    }
    const off = map.onView(() => { if (popover === pop) place(); else off(); });
  });

  if (seedNow) open(seedNow);
}
