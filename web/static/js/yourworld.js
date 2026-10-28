// The "Your world" panels: given the page's hook (what kind of thing it
// is about, which location prefabs and biomes) and a seed, say where it
// is in the reader's world: how many were placed, the nearest few from
// the start with bearing and distance, a round minimap crop, and a link
// into the map. Progress is shown while the engine is still placing.

import { World, drawMiniMap } from './world.js';

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
  const link = (x, z, text) => `<a href="${base}world/?seed=${encodeURIComponent(seed)}&x=${Math.round(x)}&z=${Math.round(z)}&zoom=12">${text}</a>`;
  const link3d = (x, z) => `<a href="${base}world/?seed=${encodeURIComponent(seed)}&x=${Math.round(x)}&z=${Math.round(z)}&zoom=14&view=3d&pitch=35" title="Fly there in 3D">3D</a>`;

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
    const span = Math.max(1500, Math.min(6000, n.d * 2.6));
    focus = { x: (start.x + n.x) / 2, z: (start.z + n.z) / 2, span };
    if (sites.length > 5) {
      const p = document.createElement('p');
      p.className = 'yw-sub';
      p.style.marginTop = '8px';
      p.innerHTML = `${link(start.x, start.z, 'All of them on the map')}.`;
      left.appendChild(p);
    }
  } else if (biomes.length) {
    const found = [];
    for (const b of biomes) {
      const hit = await nearestBiome(handle, b, start);
      if (hit) found.push({ biome: b, ...hit });
    }
    found.sort((a, b) => a.d - b.d);
    if (found.length) {
      const f = found[0];
      left.innerHTML = `<div class="yw-big">${World.formatDistance(f.d)}</div><div class="yw-sub">to the nearest ${biomeName(f.biome)}, ${f.b} of spawn · ${link(f.x, f.z, 'map')} · ${link3d(f.x, f.z)}</div>`;
      const ul = document.createElement('ul');
      ul.className = 'yw-rows';
      for (const g of found) {
        const li = document.createElement('li');
        li.innerHTML = `<span class="k">${biomeName(g.biome)}</span><span class="v">${World.formatDistance(g.d)} ${g.b}${g.share != null ? ` · ${g.share.toFixed(1)}% of the land` : ''}</span>`;
        ul.appendChild(li);
      }
      left.appendChild(ul);
      markers.push({ x: f.x, z: f.z, color: '#f0bb75' });
      focus = { x: (start.x + f.x) / 2, z: (start.z + f.z) / 2, span: Math.max(2000, Math.min(8000, f.d * 2.6)) };
    } else {
      left.innerHTML = `<p class="yw-empty">None of ${biomes.map(biomeName).join(', ')} in this world.</p>`;
    }
  } else {
    left.innerHTML = `<p class="yw-empty">Not placed in this world.</p>`;
  }
  markers.push({ x: start.x, z: start.z, color: '#ffffff', ring: true });
  drawMiniMap(canvas, handle, { cx: focus.x, cz: focus.z, span: focus.span, markers });
  canvas.title = 'Spawn is the white dot';
}

function biomeName(b) {
  return { 'meadows': 'Meadows', 'black-forest': 'Black Forest', 'swamp': 'Swamp', 'mountain': 'Mountains', 'plains': 'Plains', 'mistlands': 'Mistlands', 'ashlands': 'Ashlands', 'deep-north': 'Deep North', 'ocean': 'Ocean' }[b] || b;
}

// nearestBiome scans the biome plane in rings around the start for the
// closest texel of the biome, and counts its share of the land.
async function nearestBiome(handle, biome, start) {
  const map = handle.map;
  if (!map) return null;
  const idx = { meadows: 1, swamp: 2, mountain: 3, 'black-forest': 4, plains: 5, ashlands: 6, 'deep-north': 7, ocean: 9, mistlands: 10 }[biome];
  if (!idx) return null;
  const size = map.size, R = handle.info.radius;
  const texel = 2 * R / size;
  let best = null, count = 0, land = 0;
  const sx = (start.x + R) / texel, sz = (R - start.z) / texel;
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const b = map.biomes[y * size + x];
      if (b === 0) continue;
      if (b !== 9) land++;
      if (b !== idx) continue;
      count++;
      const dx = x + 0.5 - sx, dz = y + 0.5 - sz;
      const d2 = dx * dx + dz * dz;
      if (!best || d2 < best.d2) best = { d2, x, y };
    }
  }
  if (!best) return null;
  const wx = (best.x + 0.5) * texel - R, wz = R - (best.y + 0.5) * texel;
  return { x: wx, z: wz, d: Math.sqrt(best.d2) * texel, b: World.bearing(start.x, start.z, wx, wz), share: land ? 100 * count / land : null };
}
