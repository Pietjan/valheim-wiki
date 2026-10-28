// The page's small behaviours: theme, menu, search palette, index
// filters, the seed chip and dialog, and the "Your world" panels. The
// seed engine itself lives in world.js and is only loaded once a seed is
// known, so a reader without one pays nothing for it.

const base = document.documentElement.dataset.base || '/';
const $ = (sel, el = document) => el.querySelector(sel);
const $$ = (sel, el = document) => Array.from(el.querySelectorAll(sel));

// ---------- theme ----------
const themeBtn = $('[data-theme-toggle]');
if (themeBtn) {
  themeBtn.addEventListener('click', () => {
    const dark = matchMedia('(prefers-color-scheme: dark)').matches;
    const cur = document.documentElement.dataset.theme || (dark ? 'dark' : 'light');
    const next = cur === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    try { localStorage.setItem('vegvisir.theme', next); } catch (e) { /* private mode */ }
  });
}

// ---------- menu ----------
const navBtn = $('[data-nav-toggle]');
if (navBtn) {
  navBtn.addEventListener('click', () => {
    const top = $('.top');
    const open = top.classList.toggle('nav-open');
    navBtn.setAttribute('aria-expanded', String(open));
  });
}

// ---------- index filters ----------
for (const input of $$('[data-filter]')) {
  const sel = input.dataset.filter;
  const items = $$(sel);
  const groups = $$('[data-group]');
  const apply = () => {
    const q = input.value.trim().toLowerCase();
    for (const el of items) el.hidden = q !== '' && !el.dataset.name.includes(q);
    for (const g of groups) {
      const any = $$(sel, g).some(el => !el.hidden);
      g.hidden = !any;
    }
  };
  input.addEventListener('input', apply);
}
for (const pills of $$('[data-pills]')) {
  const buttons = $$('[data-pill]', pills);
  const groups = $$('[data-group]');
  let active = null;
  for (const b of buttons) {
    b.addEventListener('click', () => {
      active = active === b.dataset.pill ? null : b.dataset.pill;
      for (const bb of buttons) bb.setAttribute('aria-pressed', String(bb.dataset.pill === active));
      for (const g of groups) g.hidden = active !== null && g.dataset.group !== active;
      if (active) location.hash = active;
    });
  }
}

// ---------- search ----------
let searchIndex = null;
async function loadIndex() {
  if (searchIndex) return searchIndex;
  const res = await fetch(base + 'search.json');
  searchIndex = await res.json();
  return searchIndex;
}
function score(entry, q, words) {
  const name = entry.N.toLowerCase();
  if (name === q) return 100;
  let s = 0;
  if (name.startsWith(q)) s += 60;
  else if (name.includes(q)) s += 40;
  for (const w of words) {
    if (!w) continue;
    if (name.split(/\s+/).some(n => n.startsWith(w))) s += 20;
    else if (name.includes(w)) s += 8;
    else if ((entry.T || '').toLowerCase().includes(w)) s += 5;
    else if ((entry.S || '').toLowerCase().includes(w)) s += 3;
    else return 0;
  }
  // Short names first among equals.
  return s + Math.max(0, 10 - name.length / 4);
}
function search(index, q, limit = 12) {
  q = q.trim().toLowerCase();
  if (!q) return [];
  const words = q.split(/\s+/);
  const out = [];
  for (const e of index) {
    const s = score(e, q, words);
    if (s > 0) out.push([s, e]);
  }
  out.sort((a, b) => b[0] - a[0] || a[1].N.localeCompare(b[1].N));
  return out.slice(0, limit).map(x => x[1]);
}
function renderResults(list, results, selected) {
  list.innerHTML = '';
  results.forEach((e, i) => {
    const li = document.createElement('li');
    if (i === selected) li.setAttribute('aria-selected', 'true');
    const a = document.createElement('a');
    a.href = base + e.U;
    const img = e.I ? `<img src="${base}icons/${e.I}.png" alt="" loading="lazy">` : '<span class="blank"></span>';
    a.innerHTML = `${img}<span><span class="r-name"></span><span class="r-sub"></span></span><span class="r-kind"></span>`;
    a.querySelector('.r-name').textContent = e.N;
    a.querySelector('.r-sub').textContent = e.S || '';
    a.querySelector('.r-kind').textContent = e.K;
    li.appendChild(a);
    list.appendChild(li);
  });
}
const dialog = $('[data-search]');
if (dialog) {
  const input = $('[data-search-input]', dialog);
  const list = $('[data-search-results]', dialog);
  let results = [];
  let selected = 0;
  const open = () => { if (!dialog.open) dialog.showModal(); input.focus(); input.select(); loadIndex(); };
  for (const b of $$('[data-search-open]')) b.addEventListener('click', open);
  document.addEventListener('keydown', e => {
    const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName || '');
    if ((e.key === '/' && !typing) || ((e.metaKey || e.ctrlKey) && e.key === 'k')) { e.preventDefault(); open(); }
  });
  input.addEventListener('input', async () => {
    const idx = await loadIndex();
    results = search(idx, input.value);
    selected = 0;
    renderResults(list, results, selected);
  });
  input.addEventListener('keydown', e => {
    if (e.key === 'ArrowDown') { e.preventDefault(); selected = Math.min(selected + 1, results.length - 1); renderResults(list, results, selected); }
    if (e.key === 'ArrowUp') { e.preventDefault(); selected = Math.max(selected - 1, 0); renderResults(list, results, selected); }
    if (e.key === 'Enter' && results[selected]) { e.preventDefault(); location.href = base + results[selected].U; }
  });
  dialog.addEventListener('click', e => { if (e.target === dialog) dialog.close(); });
}
// The search page: the same index, inline.
const pageForm = $('[data-search-page]');
if (pageForm) {
  const input = $('[data-search-page-input]');
  const list = $('[data-search-page-results]');
  const params = new URLSearchParams(location.search);
  const run = async () => { const idx = await loadIndex(); renderResults(list, search(idx, input.value, 50), -1); };
  input.addEventListener('input', run);
  pageForm.addEventListener('submit', e => e.preventDefault());
  if (params.get('q')) { input.value = params.get('q'); run(); }
  input.focus();
}

// ---------- seed ----------
const SEED_KEY = 'vegvisir.seed';
function readSeed() {
  const p = new URLSearchParams(location.search).get('seed');
  if (p) { try { localStorage.setItem(SEED_KEY, p); } catch (e) { /* ignore */ } return p; }
  try { return localStorage.getItem(SEED_KEY) || ''; } catch (e) { return ''; }
}
function writeSeed(seed) {
  try { seed ? localStorage.setItem(SEED_KEY, seed) : localStorage.removeItem(SEED_KEY); } catch (e) { /* ignore */ }
  window.dispatchEvent(new CustomEvent('vegvisir:seed', { detail: { seed } }));
}
let seed = readSeed();
const chip = $('[data-seed-chip]');
const chipText = $('[data-seed-text]');
function paintChip() {
  if (!chip) return;
  chip.classList.toggle('is-set', !!seed);
  chipText.textContent = seed || 'Set seed';
  chip.title = seed ? `Your world: ${seed}. Click to change.` : 'Enter your world seed';
}
paintChip();
const seedDialog = $('[data-seed-dialog]');
if (seedDialog) {
  const input = $('[data-seed-input]', seedDialog);
  const form = $('[data-seed-form]', seedDialog);
  const openSeed = () => { input.value = seed; seedDialog.showModal(); input.focus(); input.select(); };
  chip?.addEventListener('click', openSeed);
  document.addEventListener('click', e => { if (e.target.closest('[data-seed-open]')) openSeed(); });
  form.addEventListener('submit', e => {
    const which = e.submitter?.value;
    if (which === 'set') {
      const v = input.value.trim();
      if (!v) return;
      seed = v; writeSeed(seed); paintChip(); startYourWorld();
    } else if (which === 'clear') {
      seed = ''; writeSeed(''); paintChip();
      for (const el of $$('[data-yw-body]')) el.innerHTML = el.dataset.empty || '';
      for (const el of $$('.yw')) el.classList.remove('yw-on');
      location.reload();
    }
  });
  seedDialog.addEventListener('click', e => { if (e.target === seedDialog) seedDialog.close(); });
}
const homeSeed = $('[data-home-seed]');
if (homeSeed) {
  const input = homeSeed.querySelector('input');
  input.value = seed;
  homeSeed.addEventListener('submit', e => {
    e.preventDefault();
    const v = input.value.trim();
    if (!v) return;
    writeSeed(v);
    location.href = base + 'world/?seed=' + encodeURIComponent(v);
  });
}

// ---------- Your world ----------
const ywPanels = $$('[data-yw]');
for (const el of $$('[data-yw-body]')) el.dataset.empty = el.innerHTML;
let ywStarted = false;
async function startYourWorld() {
  if (!seed || ywPanels.length === 0) return;
  const { mountYourWorld } = await import(base + 'static/js/yourworld.js');
  mountYourWorld(ywPanels, seed, base);
  ywStarted = true;
}
if (seed && ywPanels.length) startYourWorld();
