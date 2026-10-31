// The "Your world" panels: given the page's hook (what kind of thing it
// is about, which location prefabs and biomes) and a seed, say where it
// is in the reader's world: how many were placed, the nearest few from
// the start with bearing and distance, a round minimap crop, and a link
// into the map. Progress is shown while the engine is still placing.

import { World, drawMiniMap } from './world.js';

const SIZEABLE = 250000;   // m²: a patch under a quarter km² is not the headline
const CAT_COLOR = { boss: '#e06a5a', start: '#ffffff', trader: '#f0bb75', dungeon: '#9fc4ff', camp: '#dfa258', ruin: '#c9c0ad', runestone: '#b7a2d8', other: '#8fb28a' };

let catalogue = null;
async function loadCatalogue(base) {
  if (catalogue) return catalogue;
  const res = await fetch(base + 'world/locations.json');
  const list = await res.json();
  catalogue = { list, byPrefab: new Map(list.map(l => [l.prefab, l])) };
  return catalogue;
}

export function mountYourWorld(panels, seed, base) {
  const handle = World.open(seed);
  for (const el of panels) {
    const hook = JSON.parse(el.dataset.yw || '{}');
    const body = el.querySelector('[data-yw-body]');
    el.classList.add('yw-on');
    render(el, body, hook, handle, seed, base).catch(err => {
      body.innerHTML = '';
      const p = document.createElement('p');
      p.className = 'yw-empty';
      p.textContent = 'The seed engine could not run here: ' + (err && err.message || err);
      body.appendChild(p);
    });
  }
}

function progressBlock(body) {
  body.innerHTML = `<p class="yw-progress"><span data-p-text>Reading the world…</span><span class="bar"><i data-p-bar></i></span></p>`;
  return { text: body.querySelector('[data-p-text]'), bar: body.querySelector('[data-p-bar]') };
}

async function render(el, body, hook, handle, seed, base) {
  const prog = progressBlock(body);
  const cat = await loadCatalogue(base);
  handle.on('progress', p => {
    if (!prog.text.isConnected) return;
    prog.text.textContent = `Placing ${p.index + 1} of ${p.total}: ${p.placed} places so far`;
    prog.bar.style.width = Math.round(100 * (p.index + 1) / p.total) + '%';
  });
  handle.on('info', info => { if (prog.text.isConnected) prog.text.textContent = `Seed ${info.seed} is world ${info.seedNumber}. Building the map…`; });
  await handle.ready;
  const start = handle.start();
  const kind = hook.kind || 'location';
  const prefabs = hook.prefabs || [];
  const biomes = hook.biomes || [];

  // What to show: location instances for the named prefabs, else the
  // nearest patch of each named biome.
  let sites = [];
  for (const p of prefabs) sites = sites.concat(handle.byPrefab(p));
  for (const s of sites) { s.d = World.distance(start, s); s.b = World.bearing(start.x, start.z, s.x, s.z); }
  sites.sort((a, b) => a.d - b.d);

  const grid = document.createElement('div');
  grid.className = 'yw-grid';
  const left = document.createElement('div');
  const right = document.createElement('div');
  const canvas = document.createElement('canvas');
  canvas.className = 'yw-minimap';
  canvas.width = 360; canvas.height = 360;
  right.appendChild(canvas);
  grid.append(left, right);
  body.innerHTML = '';
  body.appendChild(grid);

  const markers = [];
  let focus = { x: start.x, z: start.z, span: 4000 };
  // The links into the map and the 3D view: centred on the spot and, with
  // pin=1, a pin dropped on it. The list of all of them carries no pin.
  const link = (x, z, text, pin = true) => `<a href="${base}world/?seed=${encodeURIComponent(seed)}&x=${Math.round(x)}&z=${Math.round(z)}&zoom=16${pin ? '&pin=1' : ''}">${text}</a>`;
  const link3d = (x, z) => `<a href="${base}world/?seed=${encodeURIComponent(seed)}&x=${Math.round(x)}&z=${Math.round(z)}&zoom=14&view=3d&pitch=35&pin=1" title="Fly there in 3D">3D</a>`;
  let caption = '';

  if (sites.length) {
    const near = sites.slice(0, 5);
    const label = hook.label || 'of these';
    left.innerHTML = `<div class="yw-big">${sites.length}</div><div class="yw-sub">${sites.length === 1 ? 'is' : 'are'} in your world${prefabs.length > 1 ? ` across ${prefabs.length} kinds` : ''} · the nearest from spawn:</div>`;
    const ul = document.createElement('ul');
    ul.className = 'yw-list';
    for (const s of near) {
      const c = cat.byPrefab.get(s.prefab);
      const li = document.createElement('li');
      const icon = c && c.icon ? `<img src="${base}icons/${c.icon}.png" alt="">` : '';
      li.innerHTML = `${icon}<span>${c ? c.name : s.prefab}</span><span class="d">${World.formatDistance(s.d)} ${s.b} · ${link(s.x, s.z, 'map')} · ${link3d(s.x, s.z)}</span>`;
      ul.appendChild(li);
      markers.push({ x: s.x, z: s.z, color: CAT_COLOR[c?.cat] || '#f0bb75' });
    }
    left.appendChild(ul);
    const n = near[0];
    const span = Math.max(500, Math.min(6000, n.d * 2.6));
    focus = { x: (start.x + n.x) / 2, z: (start.z + n.z) / 2, span };
    caption = `${near.length === 1 ? 'the nearest one' : `the ${near.length} nearest`}`;
    if (sites.length > 5) {
      const p = document.createElement('p');
      p.className = 'yw-sub';
      p.style.marginTop = '8px';
      p.innerHTML = `${link(start.x, start.z, 'All of them on the map', false)}.`;
      left.appendChild(p);
    }
  } else if (biomes.length) {
    // Every patch of each named biome, nearest first. The headline is the
    // nearest one of some size, since the nearest of all may be a sliver;
    // one biome lists its nearest patches with their sizes, several list
    // the sizeable patch nearest for each.
    const found = [];
    for (const b of biomes) {
      const r = await biomePatches(handle, b, start);
      if (!r) continue;
      const head = r.patches.find(pch => pch.area >= SIZEABLE) || r.patches[0];
      found.push({ biome: b, patches: r.patches, share: r.share, of: r.of, head });
    }
    found.sort((a, b) => a.head.d - b.head.d);
    if (found.length) {
      const f = found[0];
      const head = f.head;
      const skipped = head !== f.patches[0];
      left.innerHTML = `<div class="yw-big">${World.formatDistance(head.d)}</div><div class="yw-sub">to the nearest ${biomeName(f.biome)}${skipped ? ' of at least ¼ km²' : ''}, ${head.b} of spawn · ${link(head.x, head.z, 'map')} · ${link3d(head.x, head.z)}</div>`;
      const ul = document.createElement('ul');
      ul.className = 'yw-rows';
      let listed = [];
      if (found.length === 1 && f.patches.length > 1) {
        listed = f.patches.slice(0, 5);
        if (!listed.includes(head)) listed.splice(-1, 1, head);
        const li = document.createElement('li');
        li.innerHTML = `<span class="k">${biomeName(f.biome)}</span><span class="v">${f.patches.length} patches${f.share != null ? ` · ${f.share.toFixed(1)}% of ${f.of}` : ''}</span>`;
        ul.appendChild(li);
        for (const g of listed) {
          const row = document.createElement('li');
          row.className = g === head ? 'is-head' : '';
          row.innerHTML = `<span class="k">${World.formatDistance(g.d)} ${g.b}</span><span class="v">${formatArea(g.area)}${g === head ? '' : ` · ${link(g.x, g.z, 'map')}`}</span>`;
          ul.appendChild(row);
        }
      } else {
        for (const g of found) {
          const row = document.createElement('li');
          row.innerHTML = `<span class="k">${biomeName(g.biome)}</span><span class="v">${World.formatDistance(g.head.d)} ${g.head.b} · ${formatArea(g.head.area)}${g.share != null ? ` · ${g.share.toFixed(1)}% of ${g.of}` : ''}${g === f ? '' : ` · ${link(g.head.x, g.head.z, 'map')}`}</span>`;
          ul.appendChild(row);
          listed.push(g.head);
        }
      }
      left.appendChild(ul);
      for (const g of listed) markers.push({ x: g.x, z: g.z, color: '#f0bb75', size: g === head ? 7 : 5 });
      // The crop holds spawn and every listed patch.
      let minx = start.x, maxx = start.x, minz = start.z, maxz = start.z;
      for (const g of listed) { minx = Math.min(minx, g.x); maxx = Math.max(maxx, g.x); minz = Math.min(minz, g.z); maxz = Math.max(maxz, g.z); }
      focus = { x: (minx + maxx) / 2, z: (minz + maxz) / 2, span: Math.max(500, Math.min(8000, 1.6 * Math.max(maxx - minx, maxz - minz))) };
      caption = listed.length > 1 ? (found.length === 1 ? `the ${listed.length} nearest ${biomeName(f.biome)} patches` : `the nearest of each`) : `the nearest ${biomeName(f.biome)}`;
    } else {
      left.innerHTML = `<p class="yw-empty">None of ${biomes.map(biomeName).join(', ')} in this world.</p>`;
    }
  } else {
    left.innerHTML = `<p class="yw-empty">Not placed in this world.</p>`;
  }
  markers.push({ x: start.x, z: start.z, color: '#ffffff', ring: true, size: 8 });
  drawMiniMap(canvas, handle, { cx: focus.x, cz: focus.z, span: focus.span, markers, dotRadius: 7 });
  canvas.title = `${World.formatDistance(focus.span)} across`;
  if (caption) {
    const cap = document.createElement('p');
    cap.className = 'yw-cap';
    cap.innerHTML = `<i class="ring"></i>spawn <i class="dot"></i>${caption}`;
    right.appendChild(cap);
  }
}

function biomeName(b) {
  return { 'meadows': 'Meadows', 'black-forest': 'Black Forest', 'swamp': 'Swamp', 'mountain': 'Mountains', 'plains': 'Plains', 'mistlands': 'Mistlands', 'ashlands': 'Ashlands', 'deep-north': 'Deep North', 'ocean': 'Ocean' }[b] || b;
}

// biomePatches flood-fills the biome plane into the connected patches of
// one biome (four-way, so two patches touching at a corner are two), each
// with its area and its nearest texel to the start, nearest first; and the
// biome's share of the land.
async function biomePatches(handle, biome, start) {
  const map = handle.map;
  if (!map) return null;
  const idx = { meadows: 1, swamp: 2, mountain: 3, 'black-forest': 4, plains: 5, ashlands: 6, 'deep-north': 7, ocean: 9, mistlands: 10 }[biome];
  if (!idx) return null;
  const size = map.size, R = handle.info.radius;
  const texel = 2 * R / size, texelArea = texel * texel;
  const worldR2 = (handle.info.worldRadius || 10500) ** 2;   // the world's edge; the plane runs past it
  const biomesAt = map.biomes;
  const n = size * size;
  const seen = new Uint8Array(n);
  const stack = new Int32Array(n);
  const sx = (start.x + R) / texel, sz = (R - start.z) / texel;
  const cx = size / 2, cz = size / 2;
  const inWorld = (x, y) => { const dx = x + 0.5 - cx, dz = y + 0.5 - cz; return (dx * dx + dz * dz) * texelArea <= worldR2; };
  const patches = [];
  let land = 0, world = 0, count = 0;
  for (let i = 0; i < n; i++) {
    const b = biomesAt[i];
    if (b === 0) continue;
    const x0 = i % size, y0 = (i - x0) / size;
    if (inWorld(x0, y0)) { world++; if (b !== 9) land++; }
    if (b !== idx || seen[i]) continue;
    // A new patch: walk it, counting the area inside the world's edge.
    let top = 0, area = 0, best = -1, bestD2 = Infinity;
    stack[top++] = i; seen[i] = 1;
    while (top) {
      const j = stack[--top];
      const x = j % size, y = (j - x) / size;
      if (inWorld(x, y)) area++;
      const dx = x + 0.5 - sx, dz = y + 0.5 - sz;
      const d2 = dx * dx + dz * dz;
      if (d2 < bestD2) { bestD2 = d2; best = j; }
      if (x > 0 && !seen[j - 1] && biomesAt[j - 1] === idx) { seen[j - 1] = 1; stack[top++] = j - 1; }
      if (x < size - 1 && !seen[j + 1] && biomesAt[j + 1] === idx) { seen[j + 1] = 1; stack[top++] = j + 1; }
      if (y > 0 && !seen[j - size] && biomesAt[j - size] === idx) { seen[j - size] = 1; stack[top++] = j - size; }
      if (y < size - 1 && !seen[j + size] && biomesAt[j + size] === idx) { seen[j + size] = 1; stack[top++] = j + size; }
    }
    if (!area) continue;   // wholly past the edge of the world
    count += area;
    const bx = best % size, by = (best - bx) / size;
    const wx = (bx + 0.5) * texel - R, wz = R - (by + 0.5) * texel;
    patches.push({ x: wx, z: wz, d: Math.sqrt(bestD2) * texel, b: World.bearing(start.x, start.z, wx, wz), area: area * texelArea });
  }
  if (!patches.length) return null;
  patches.sort((a, b) => a.d - b.d);
  // The share: of the land for a land biome, of the whole world for the
  // ocean; both the biome's footprint as the game assigns it.
  const of = idx === 9 ? 'the world' : 'the land';
  const base = idx === 9 ? world : land;
  const share = base ? 100 * count / base : null;
  return { patches, share, of };
}

// formatArea is square kilometres to a sensible precision.
function formatArea(m2) {
  const k = m2 / 1e6;
  if (k >= 100) return `${Math.round(k)} km²`;
  if (k >= 1) return `${k.toFixed(1)} km²`;
  if (k >= 0.01) return `${k.toFixed(2)} km²`;
  return '<0.01 km²';
}
