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
  const name = entry.n.toLowerCase();
  if (name === q) return 100;
  let s = 0;
  if (name.startsWith(q)) s += 60;
  else if (name.includes(q)) s += 40;
  for (const w of words) {
    if (!w) continue;
    if (name.split(/\s+/).some(n => n.startsWith(w))) s += 20;
    else if (name.includes(w)) s += 8;
    else if ((entry.t || '').toLowerCase().includes(w)) s += 5;
    else if ((entry.s || '').toLowerCase().includes(w)) s += 3;
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
  out.sort((a, b) => b[0] - a[0] || a[1].n.localeCompare(b[1].n));
  return out.slice(0, limit).map(x => x[1]);
}
function renderResults(list, results, selected) {
  list.innerHTML = '';
  results.forEach((e, i) => {
    const li = document.createElement('li');
    if (i === selected) li.setAttribute('aria-selected', 'true');
    const a = document.createElement('a');
    a.href = base + e.u;
    const img = e.i ? `<img src="${base}icons/${e.i}.png" alt="" loading="lazy">` : '<span class="blank"></span>';
    a.innerHTML = `${img}<span><span class="r-name"></span><span class="r-sub"></span></span><span class="r-kind"></span>`;
    a.querySelector('.r-name').textContent = e.n;
    a.querySelector('.r-sub').textContent = e.s || '';
    a.querySelector('.r-kind').textContent = e.k;
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
    if (e.key === 'Enter' && results[selected]) { e.preventDefault(); location.href = base + results[selected].u; }
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

// ---------- hover cards ----------
// A chip shows a card after a short hover, or at once on keyboard focus:
// name, type line, icon and a few lines from tips.json, fetched the first
// time one is wanted. The card never takes the pointer, so the chip's
// link still gets the click; the chip's own title is dropped while the
// card is up so the browser's tooltip does not double it.
{
  let tips = null, loading = null;
  const loadTips = () => loading || (loading = fetch(base + 'tips.json').then(r => r.json()).then(t => (tips = t)).catch(() => (tips = {})));
  let card = null, timer = 0, current = null;
  const hide = () => {
    clearTimeout(timer); timer = 0;
    if (card) { card.remove(); card = null; }
    if (current) { if (current.dataset.tipTitle) current.title = current.dataset.tipTitle; current = null; }
  };
  const show = chip => {
    const tip = tips && tips[chip.dataset.tip];
    if (!tip || current !== chip) return;
    if (chip.title) { chip.dataset.tipTitle = chip.title; chip.removeAttribute('title'); }
    card = document.createElement('div');
    card.className = 'tip-card';
    card.setAttribute('role', 'tooltip');
    const icon = tip.i ? `<img src="${base}icons/${tip.i}.png" alt="" width="40" height="40">` : '';
    card.innerHTML = `<div class="tip-head">${icon}<div><b></b><span class="tip-sub"></span></div></div>`;
    card.querySelector('b').textContent = tip.n;
    card.querySelector('.tip-sub').textContent = tip.s || '';
    // A line is text, or parts: text and things drawn as small chips.
    for (const l of tip.l || []) {
      const p = document.createElement('p');
      if (typeof l === 'string') p.textContent = l;
      else for (const part of l) {
        if (part.t) { p.appendChild(document.createTextNode(part.t)); continue; }
        const c = document.createElement('span');
        c.className = 'tip-chip';
        if (part.i) { const img = document.createElement('img'); img.src = `${base}icons/${part.i}.png`; img.alt = ''; img.width = 18; img.height = 18; c.appendChild(img); }
        c.appendChild(document.createTextNode(part.n || part.k));
        if (part.x > 1) { const n = document.createElement('b'); n.textContent = '×' + part.x; c.appendChild(n); }
        p.appendChild(c);
      }
      card.appendChild(p);
    }
    document.body.appendChild(card);
    // Below the chip, or above when there is no room; kept inside the
    // window. Fixed to the viewport, so no scroll offset comes into it.
    const r = chip.getBoundingClientRect();
    const cw = card.offsetWidth, ch = card.offsetHeight;
    let x = r.left + r.width / 2 - cw / 2;
    x = Math.max(8, Math.min(window.innerWidth - cw - 8, x));
    let y = r.bottom + 8;
    if (y + ch > window.innerHeight - 8 && r.top - ch - 8 > 0) { y = r.top - ch - 8; card.classList.add('is-above'); }
    card.style.left = Math.round(x) + 'px';
    card.style.top = Math.round(y) + 'px';
  };
  const arm = (chip, delay) => {
    if (current === chip) return;
    hide();
    current = chip;
    timer = setTimeout(() => { loadTips().then(() => show(chip)); }, delay);
  };
  document.addEventListener('mouseover', e => {
    const chip = e.target.closest && e.target.closest('[data-tip]');
    if (chip) arm(chip, 220);
  });
  document.addEventListener('mouseout', e => {
    const chip = e.target.closest && e.target.closest('[data-tip]');
    if (chip && chip === current && !(e.relatedTarget && chip.contains(e.relatedTarget))) hide();
  });
  document.addEventListener('focusin', e => {
    const chip = e.target.closest && e.target.closest('[data-tip]');
    if (chip) arm(chip, 0);
  });
  document.addEventListener('focusout', hide);
  document.addEventListener('keydown', e => { if (e.key === 'Escape') hide(); });
  window.addEventListener('scroll', hide, { passive: true });
  document.addEventListener('touchstart', hide, { passive: true });
}
