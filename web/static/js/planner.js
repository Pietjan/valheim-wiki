// The planner: the reader lists pieces to build and items to make, and
// the page adds up what it costs. Each thing's recipe is followed down
// through what it is made of — a sword to its bronze, the bronze to its
// copper and tin, those to their ore and the coal the smelter burns, the
// coal to the wood the kiln chars — until something is found rather than
// made. What is found is the materials list, split by what a portal will
// carry. What is made along the way is listed as steps, each of which
// can be marked "bring it" to carry it ready-made instead. The stations
// the recipes ask for, at the levels the qualities need, are listed and,
// unless the reader has them, costed too. The plan lives in the URL's
// hash, so a link holds it, and in localStorage, so it survives a visit.

const base = document.documentElement.dataset.base || '/';
const $ = (sel, el = document) => el.querySelector(sel);
const $$ = (sel, el = document) => Array.from(el.querySelectorAll(sel));

const root = $('[data-plan]');
if (root) init();

async function init() {
  const res = await fetch(base + 'planner.json');
  const data = await res.json();
  new Planner(root, data);
}

// ---------- the arithmetic ----------

// upgradeCost is what one quality level's upgrade costs when the recipe
// asks perLevel of a material: Requirement.GetAmount in the client.
function upgradeCost(perLevel, q) {
  if (q <= 1) return 0;
  const f = q >= 4 ? 4 + (q - 4) / 2 : q - 1;
  return Math.floor(f * perLevel);
}

// unitCost is what one item at quality q costs of a requirement: the
// first make, then each upgrade up to q.
function unitCost(req, q) {
  let n = req.n;
  for (let l = 2; l <= q; l++) n += upgradeCost(req.l || 0, l);
  return n;
}

// maxQuality is how far an item can be upgraded at its station: its own
// top quality, or as far as the station's levels go.
function maxQuality(d, k) {
  const t = d.things[k];
  if (!t || !t.r || !t.r.length) return 1;
  let q = t.q || 1;
  const r = t.r[0];
  if (r.at && d.stations[r.at]) {
    const top = d.stations[r.at].max - Math.max(1, r.lv || 1) + 1;
    q = Math.max(1, Math.min(q, top));
  }
  return q;
}

// conversions is every converter rule that makes k, in the data's order.
function conversions(d, k) {
  const out = [];
  for (const [piece, c] of Object.entries(d.conv)) {
    for (const rule of c.c) if (rule.to === k && !out.some(o => o.from === rule.from)) out.push({ piece, from: rule.from, n: rule.n });
  }
  return out;
}

// Plan computes everything the page shows from the list and options.
class Plan {
  constructor(data, list, opts) {
    this.d = data;
    this.list = list;
    this.bring = opts.bring;
    this.from = opts.from;
    this.need = new Map();      // material -> count found, not made
    this.steps = new Map();     // thing made along the way -> step
    this.order = [];            // steps, inputs before what they make
    this.stationLevel = new Map(); // station piece -> level needed
    this.converters = new Map();   // converter piece -> what it makes
    this.tools = new Set();
    this.auto = [];             // station pieces counted without being listed
    this.run(opts.stations);
  }

  thing(k) { return this.d.things[k]; }

  run(countStations) {
    const pass = () => {
      this.need.clear(); this.steps.clear(); this.order = [];
      this.stationLevel.clear(); this.converters.clear(); this.tools.clear();
      for (const e of this.list) this.want(e.k, e.n, e.q || 1, 0);
      for (const k of this.auto) this.want(k, 1, 1, 0);
    };
    pass();
    if (!countStations) return;
    // The stations and upgrades the plan needs but does not list are
    // added and the sum redone, since they cost materials and ask for
    // stations of their own, until nothing new is needed.
    for (let round = 0; round < 6; round++) {
      const missing = this.missingStations();
      if (!missing.length) break;
      this.auto.push(...missing);
      pass();
    }
  }

  // have is the level of a station the list (and the auto-added pieces)
  // already build: the station piece itself and one per upgrade piece.
  have(station, listOnly) {
    const listed = new Set(this.list.filter(e => e.n > 0).map(e => e.k));
    if (!listOnly) for (const a of this.auto) listed.add(a);
    const st = this.d.stations[station];
    if (!listed.has(station)) return 0;
    let lv = 1;
    for (const e of (st && st.ext) || []) if (listed.has(e)) lv++;
    return lv;
  }

  missingStations() {
    const out = [];
    for (const [station, lv] of this.stationLevel) {
      const st = this.d.stations[station];
      const have = this.have(station);
      if (have === 0 && !this.auto.includes(station)) out.push(station);
      let got = have > 0 ? have : 1;
      for (const e of (st && st.ext) || []) {
        if (got >= lv) break;
        const listed = this.list.some(x => x.k === e && x.n > 0) || this.auto.includes(e);
        if (!listed) { out.push(e); }
        got++;
      }
    }
    for (const conv of this.converters.keys()) {
      if (!this.list.some(x => x.k === conv && x.n > 0) && !this.auto.includes(conv)) out.push(conv);
    }
    return [...new Set(out)];
  }

  addNeed(k, n) { this.need.set(k, (this.need.get(k) || 0) + n); }
  needStation(at, lv) { if (at) this.stationLevel.set(at, Math.max(this.stationLevel.get(at) || 0, lv)); }

  // want asks for n of k at quality q, and follows what that takes.
  want(k, n, q, depth) {
    if (n <= 0 || depth > 40) return;
    const t = this.thing(k);
    if (!t) { this.addNeed(k, n); return; }
    if (t.k === 'piece' && t.p) {
      const p = t.p;
      this.needStation(p.at, 1);
      if (p.tool) this.tools.add(p.tool);
      for (const r of p.res) this.want(r.i, r.n * n, 1, depth + 1);
      return;
    }
    // A thing the reader carries ready-made, or that is found, not made.
    if (depth > 0 && this.bring.has(k)) { this.addNeed(k, n); return; }
    if (t.r && t.r.length) {
      const r = t.r[0];
      const per = r.n || 1;
      const batches = Math.ceil(n / per);
      this.needStation(r.at, Math.max(1, r.lv || 1) + q - 1);
      const inputs = new Map();
      for (const req of r.res) {
        const c = per > 1 ? batches * req.n : n * unitCost(req, q);
        if (c > 0) inputs.set(req.i, (inputs.get(req.i) || 0) + c);
      }
      for (const [i, c] of inputs) this.want(i, c, 1, depth + 1);
      this.step(k, { kind: 'recipe', k, n, q, at: r.at, batches, per, inputs, top: depth === 0 });
      return;
    }
    const ways = this.conversions(k);
    if (ways.length) {
      const chosen = ways.find(w => w.from === this.from.get(k)) || ways[0];
      const per = chosen.n || 1;
      const inputs = Math.ceil(n / per);
      const conv = this.d.conv[chosen.piece];
      const fuel = conv.fuel && conv.fpi ? Math.ceil(n * conv.fpi) : 0;
      this.converters.set(chosen.piece, true);
      this.want(chosen.from, inputs, 1, depth + 1);
      if (fuel) this.want(conv.fuel, fuel, 1, depth + 1);
      this.step(k, { kind: 'conv', k, n, piece: chosen.piece, from: chosen.from, inputs, fuel: fuel ? conv.fuel : '', fuelN: fuel, secs: conv.spi ? conv.spi * n : 0, alts: ways.map(w => w.from), top: depth === 0 });
      return;
    }
    this.addNeed(k, n);
  }

  conversions(k) { return conversions(this.d, k); }

  // step records a thing made along the way, summing repeat makes.
  step(k, s) {
    const prev = this.steps.get(k);
    if (!prev) { this.steps.set(k, s); this.order.push(k); return; }
    prev.n += s.n;
    prev.top = prev.top || s.top;
    if (s.kind === 'recipe') { prev.batches += s.batches; for (const [i, c] of s.inputs) prev.inputs.set(i, (prev.inputs.get(i) || 0) + c); }
    else { prev.inputs += s.inputs; prev.fuelN += s.fuelN; prev.secs += s.secs; }
  }
}

// ---------- the page ----------

const STORE = 'vegvisir.plan';

class Planner {
  constructor(root, data) {
    this.root = root;
    this.d = data;
    this.list = [];
    this.bring = new Set();
    this.from = new Map();
    this.stations = true;
    this.el = {
      search: $('[data-plan-search]', root), results: $('[data-plan-results]', root),
      entries: $('[data-plan-entries]', root), empty: $('[data-plan-empty]', root), count: $('[data-plan-count]', root),
      out: $('[data-plan-out]', root), materials: $('[data-plan-materials]', root), matCount: $('[data-plan-mat-count]', root),
      stepsPanel: $('[data-plan-steps-panel]', root), steps: $('[data-plan-steps]', root),
      stationsPanel: $('[data-plan-stations-panel]', root), stationList: $('[data-plan-station-list]', root),
      stationsBox: $('[data-plan-stations]', root), share: $('[data-plan-share]', root), clear: $('[data-plan-clear]', root), shared: $('[data-plan-shared]', root),
    };
    this.index = this.buildIndex();
    this.load();
    this.bindSearch();
    this.el.stationsBox.addEventListener('change', () => { this.stations = this.el.stationsBox.checked; this.changed(); });
    this.el.clear.addEventListener('click', () => { this.list = []; this.bring.clear(); this.from.clear(); this.changed(); this.el.search.focus(); });
    this.el.share.addEventListener('click', () => this.share());
    this.el.entries.addEventListener('input', e => this.onEntryInput(e));
    this.el.entries.addEventListener('change', e => this.onEntryInput(e));
    this.el.entries.addEventListener('click', e => this.onEntryClick(e));
    this.el.steps.addEventListener('click', e => this.onStepClick(e));
    this.el.materials.addEventListener('click', e => this.onStepClick(e));
    window.addEventListener('hashchange', () => { if (this.readHash()) this.render(); });
    this.render();
  }

  // ---- state ----

  load() {
    if (this.readHash()) { this.save(); return; }
    try {
      const s = JSON.parse(localStorage.getItem(STORE) || 'null');
      if (s && Array.isArray(s.list)) this.apply(s);
    } catch (e) { /* nothing kept */ }
  }

  apply(s) {
    this.list = s.list.filter(e => e && this.d.things[e.k]).map(e => ({ k: e.k, n: Math.max(1, Math.min(9999, +e.n || 1)), q: Math.max(1, +e.q || 1) }));
    this.bring = new Set((s.bring || []).filter(k => this.d.things[k]));
    this.from = new Map((s.from || []).filter(p => p && p.length === 2));
    this.stations = s.stations !== false;
    this.el.stationsBox.checked = this.stations;
  }

  // The hash: #list=SwordBronze.1.3,forge.1&bring=Bronze&from=Iron.IronScrap&stations=0
  readHash() {
    const h = location.hash.replace(/^#/, '');
    if (!h) return false;
    const p = new URLSearchParams(h);
    if (!p.has('list')) return false;
    const list = p.get('list').split(',').filter(Boolean).map(s => { const [k, n, q] = s.split('.'); return { k, n: +n || 1, q: +q || 1 }; });
    const bring = (p.get('bring') || '').split(',').filter(Boolean);
    const from = (p.get('from') || '').split(',').filter(Boolean).map(s => s.split('.'));
    this.apply({ list, bring, from, stations: p.get('stations') !== '0' });
    return true;
  }

  state() { return { list: this.list, bring: [...this.bring], from: [...this.from], stations: this.stations }; }

  save() {
    const s = this.state();
    try { localStorage.setItem(STORE, JSON.stringify(s)); } catch (e) { /* private mode */ }
    const p = new URLSearchParams();
    if (this.list.length) p.set('list', this.list.map(e => e.k + '.' + e.n + (e.q > 1 ? '.' + e.q : '')).join(','));
    if (this.bring.size) p.set('bring', [...this.bring].join(','));
    if (this.from.size) p.set('from', [...this.from].map(x => x.join('.')).join(','));
    if (!this.stations) p.set('stations', '0');
    const hash = p.toString() ? '#' + p.toString().replace(/%2C/g, ',').replace(/%2E/g, '.') : '';
    if (hash !== location.hash && !(hash === '' && location.hash === '')) history.replaceState(null, '', location.pathname + location.search + hash);
  }

  changed() { this.save(); this.render(); }

  async share() {
    this.save();
    const url = location.href;
    try {
      await navigator.clipboard.writeText(url);
      this.el.shared.textContent = 'Link copied. Anyone who opens it sees this plan.';
    } catch (e) {
      this.el.shared.textContent = 'The plan is in this page\'s address: copy it from the address bar.';
    }
    this.el.shared.hidden = false;
    clearTimeout(this.sharedTimer);
    this.sharedTimer = setTimeout(() => { this.el.shared.hidden = true; }, 5000);
  }

  // ---- search ----

  buildIndex() {
    const out = [];
    for (const [k, t] of Object.entries(this.d.things)) {
      if (!(t.p || (t.r && t.r.length) || conversions(this.d, k).length)) continue;
      out.push({ k, t, name: t.n.toLowerCase(), words: t.n.toLowerCase().split(/\s+/) });
    }
    return out;
  }

  search(q, limit = 10) {
    q = q.trim().toLowerCase();
    if (!q) return [];
    const words = q.split(/\s+/).filter(Boolean);
    const out = [];
    for (const e of this.index) {
      let s = 0;
      if (e.name === q) s = 100;
      else if (e.name.startsWith(q)) s = 60;
      else if (e.name.includes(q)) s = 40;
      for (const w of words) {
        if (e.words.some(n => n.startsWith(w))) s += 20;
        else if (e.name.includes(w)) s += 8;
        else if ((e.t.s || '').toLowerCase().includes(w)) s += 5;
        else { s = 0; break; }
      }
      if (s <= 0) continue;
      // Pieces of the hammer first among equals, then short names.
      if (e.t.p && e.t.p.tool === 'Hammer') s += 2;
      s += Math.max(0, 10 - e.name.length / 4);
      out.push([s, e]);
    }
    out.sort((a, b) => b[0] - a[0] || a[1].name.localeCompare(b[1].name));
    return out.slice(0, limit).map(x => x[1]);
  }

  bindSearch() {
    const input = this.el.search, list = this.el.results;
    let results = [], selected = 0;
    const close = () => { list.hidden = true; list.innerHTML = ''; results = []; input.setAttribute('aria-expanded', 'false'); };
    const draw = () => {
      list.innerHTML = '';
      results.forEach((e, i) => {
        const li = document.createElement('li');
        li.setAttribute('role', 'option');
        if (i === selected) li.setAttribute('aria-selected', 'true');
        const b = document.createElement('button');
        b.type = 'button';
        b.dataset.k = e.k;
        b.innerHTML = `${iconHTML(e.t, 36)}<span><span class="r-name"></span><span class="r-sub"></span></span><span class="r-kind"></span>`;
        b.querySelector('.r-name').textContent = e.t.n + (e.t.h ? ' ' : '');
        if (e.t.h) { const sm = document.createElement('small'); sm.className = 'muted'; sm.textContent = e.t.h; b.querySelector('.r-name').appendChild(sm); }
        b.querySelector('.r-sub').textContent = this.sub(e.k, e.t);
        b.querySelector('.r-kind').textContent = e.t.p ? 'piece' : 'item';
        li.appendChild(b);
        list.appendChild(li);
      });
      list.hidden = results.length === 0;
      input.setAttribute('aria-expanded', String(results.length > 0));
    };
    const pick = i => {
      const e = results[i];
      if (!e) return;
      this.add(e.k);
      input.value = '';
      close();
      input.focus();
    };
    input.addEventListener('input', () => { results = this.search(input.value); selected = 0; draw(); });
    input.addEventListener('focus', () => { if (input.value.trim()) { results = this.search(input.value); draw(); } });
    input.addEventListener('keydown', e => {
      if (e.key === 'ArrowDown' && results.length) { e.preventDefault(); selected = (selected + 1) % results.length; draw(); }
      else if (e.key === 'ArrowUp' && results.length) { e.preventDefault(); selected = (selected - 1 + results.length) % results.length; draw(); }
      else if (e.key === 'Enter') { e.preventDefault(); pick(selected); }
      else if (e.key === 'Escape') { close(); }
    });
    list.addEventListener('mousedown', e => {
      const b = e.target.closest('button[data-k]');
      if (!b) return;
      e.preventDefault();
      pick(results.findIndex(r => r.k === b.dataset.k));
    });
    document.addEventListener('click', e => { if (!this.root.querySelector('.plan-add').contains(e.target)) close(); });
  }

  // sub is a thing's type line for the search and the list: a piece's
  // menu and tool, an item's type and station.
  sub(k, t) {
    if (t.p) return [t.s, t.p.tool ? 'with the ' + this.name(t.p.tool) : ''].filter(Boolean).join(' · ');
    const r = t.r && t.r[0];
    const c = !r && conversions(this.d, k)[0];
    return [t.s, r && r.at ? 'at the ' + this.name(r.at) : r ? 'by hand' : c ? 'in the ' + this.name(c.piece) : '', r && r.n > 1 ? r.n + ' a batch' : ''].filter(Boolean).join(' · ');
  }

  name(k) { const t = this.d.things[k]; return t ? t.n : pretty(k); }

  // batch is how many of an item one make gives: arrows come twenty at
  // a time, so the list counts them by the batch.
  batch(k) { const t = this.d.things[k]; const r = t && t.r && t.r[0]; return r && r.n > 1 ? r.n : 1; }

  // ---- the list ----

  add(k) {
    const e = this.list.find(x => x.k === k);
    const per = this.batch(k);
    if (e) e.n = Math.min(9999, e.n + per);
    else this.list.push({ k, n: per, q: 1 });
    this.changed();
    if (!e) { const row = this.el.entries.lastElementChild; if (row) row.classList.add('is-new'); }
  }

  onEntryInput(e) {
    const row = e.target.closest('[data-k]');
    if (!row) return;
    const entry = this.list.find(x => x.k === row.dataset.k);
    if (!entry) return;
    if (e.target.matches('[data-n]')) entry.n = Math.max(1, Math.min(9999, Math.floor(+e.target.value) || 1));
    if (e.target.matches('[data-q]')) entry.q = +e.target.value || 1;
    this.save();
    this.renderOut();
    this.el.count.textContent = this.countText();
  }

  onEntryClick(e) {
    const b = e.target.closest('button[data-act]');
    if (!b) return;
    const row = b.closest('[data-k]');
    const i = this.list.findIndex(x => x.k === row.dataset.k);
    if (i < 0) return;
    if (b.dataset.act === 'remove') this.list.splice(i, 1);
    const per = this.batch(this.list[i].k);
    if (b.dataset.act === 'more') this.list[i].n = Math.min(9999, this.list[i].n + per);
    if (b.dataset.act === 'less') this.list[i].n = Math.max(1, this.list[i].n - per);
    this.changed();
  }

  onStepClick(e) {
    const b = e.target.closest('button[data-bring], button[data-make], button[data-from]');
    if (!b) return;
    if (b.dataset.bring) this.bring.add(b.dataset.bring);
    if (b.dataset.make) this.bring.delete(b.dataset.make);
    if (b.dataset.from) this.from.set(b.dataset.item, b.dataset.from);
    this.changed();
  }

  countText() {
    const n = this.list.length;
    return n ? `${n} ${n === 1 ? 'thing' : 'things'}` : '';
  }

  render() {
    this.renderList();
    this.renderOut();
  }

  renderList() {
    const ul = this.el.entries;
    ul.innerHTML = '';
    this.el.empty.hidden = this.list.length > 0;
    this.el.count.textContent = this.countText();
    for (const e of this.list) {
      const t = this.d.things[e.k];
      const li = document.createElement('li');
      li.className = 'plan-entry';
      li.dataset.k = e.k;
      const maxQ = maxQuality(this.d, e.k);
      if (e.q > maxQ) e.q = maxQ;
      li.innerHTML = `${chipHTML(e.k, t)}<span class="plan-entry-sub"></span>
        <span class="plan-qty"><button type="button" class="plan-pm" data-act="less" aria-label="One fewer">−</button><input type="number" inputmode="numeric" min="1" max="9999" data-n value="${e.n}" aria-label="How many"><button type="button" class="plan-pm" data-act="more" aria-label="One more">+</button></span>
        ${maxQ > 1 ? `<select class="plan-q" data-q aria-label="Quality">${Array.from({ length: maxQ }, (_, i) => `<option value="${i + 1}"${e.q === i + 1 ? ' selected' : ''}>quality ${i + 1}</option>`).join('')}</select>` : ''}
        <button type="button" class="plan-x" data-act="remove" aria-label="Remove">×</button>`;
      li.querySelector('.plan-entry-sub').textContent = this.sub(e.k, t);
      ul.appendChild(li);
    }
  }

  renderOut() {
    const plan = new Plan(this.d, this.list, { bring: this.bring, from: this.from, stations: this.stations });
    const any = this.list.length > 0;
    this.el.out.hidden = !any;
    if (!any) return;
    this.renderMaterials(plan);
    this.renderSteps(plan);
    this.renderStations(plan);
  }

  renderMaterials(plan) {
    const rows = [...plan.need].map(([k, n]) => ({ k, n, t: this.d.things[k] })).filter(r => r.n > 0);
    rows.sort((a, b) => b.n - a.n || (a.t ? a.t.n : a.k).localeCompare(b.t ? b.t.n : b.k));
    const portal = rows.filter(r => !r.t || r.t.tp), heavy = rows.filter(r => r.t && !r.t.tp);
    this.el.matCount.textContent = rows.length ? `${rows.length} kinds · ${fmtWeight(rows.reduce((a, r) => a + r.n * (r.t ? r.t.w || 0 : 0), 0))}` : '';
    const el = this.el.materials;
    el.innerHTML = '';
    if (!rows.length) { el.innerHTML = '<p class="empty">Nothing to gather: everything listed is carried ready-made.</p>'; return; }
    const group = (title, note, list) => {
      if (!list.length) return;
      const sec = document.createElement('div');
      sec.className = 'plan-group';
      const slots = list.reduce((a, r) => a + (r.t && r.t.st ? Math.ceil(r.n / r.t.st) : 1), 0);
      const weight = list.reduce((a, r) => a + r.n * (r.t ? r.t.w || 0 : 0), 0);
      sec.innerHTML = `<h3>${title} <span class="count">${slots} ${slots === 1 ? 'slot' : 'slots'} · ${fmtWeight(weight)}</span></h3><p class="muted small plan-group-note">${note}</p><table class="plan-table"><thead><tr><th>Material</th><th class="num">Need</th><th class="num">Stacks</th><th class="num">Weight</th><th></th></tr></thead><tbody></tbody></table>`;
      const tb = sec.querySelector('tbody');
      for (const r of list) {
        const tr = document.createElement('tr');
        const stacks = r.t && r.t.st ? r.n / r.t.st : 0;
        const made = this.bring.has(r.k);
        tr.innerHTML = `<td>${chipHTML(r.k, r.t)}</td><td class="num"><b>${fmt(r.n)}</b></td><td class="num muted">${stacks ? fmtStacks(stacks, r.t.st) : ''}</td><td class="num muted">${r.t && r.t.w ? fmtWeight(r.n * r.t.w) : ''}</td><td class="plan-act">${made ? `<button type="button" class="link small" data-make="${r.k}">make it</button>` : ''}</td>`;
        tb.appendChild(tr);
      }
      el.appendChild(sec);
    };
    group('Through a portal', 'Carried in the inventory and fine through a portal.', portal);
    group('Not through a portal', 'The game marks these as not teleportable: they go by foot, cart or boat.', heavy);
  }

  renderSteps(plan) {
    const ol = this.el.steps;
    ol.innerHTML = '';
    this.el.stepsPanel.hidden = plan.order.length === 0;
    for (const k of plan.order) {
      const s = plan.steps.get(k);
      const t = this.d.things[k];
      const li = document.createElement('li');
      li.className = 'plan-step';
      let how;
      if (s.kind === 'recipe') {
        const parts = [...s.inputs].map(([i, c]) => chipHTML(i, this.d.things[i], c)).join(' ');
        how = `<span class="plan-step-how">${s.at ? 'at the ' + chipHTML(s.at, this.d.things[s.at]) : 'by hand'}${s.per > 1 ? ` <span class="muted">· ${s.batches} ${s.batches === 1 ? 'batch' : 'batches'} of ${s.per}</span>` : ''}</span><span class="plan-step-from">${parts}</span>`;
      } else {
        const alts = s.alts.length > 1 ? ` <span class="plan-step-alts">from ${s.alts.map(a => a === s.from ? `<b>${esc(this.name(a))}</b>` : `<button type="button" class="link" data-from="${a}" data-item="${k}">${esc(this.name(a))}</button>`).join(' or ')}</span>` : '';
        const fuel = s.fuel ? ' ' + chipHTML(s.fuel, this.d.things[s.fuel], s.fuelN) : '';
        how = `<span class="plan-step-how">in the ${chipHTML(s.piece, this.d.things[s.piece])}${alts}${s.secs ? ` <span class="muted">· ${fmtTime(s.secs)} in one</span>` : ''}</span><span class="plan-step-from">${chipHTML(s.from, this.d.things[s.from], s.inputs)}${fuel}</span>`;
      }
      if (s.top) li.classList.add('is-goal');
      const q = s.kind === 'recipe' && s.q > 1 ? ` <span class="muted">at quality ${s.q}</span>` : '';
      li.innerHTML = `<div class="plan-step-what">${chipHTML(k, t, s.n)}${q}</div><div class="plan-step-body">${how}</div>${s.top ? '' : `<button type="button" class="link small plan-step-bring" data-bring="${k}" title="Carry it ready-made instead of what makes it">bring it</button>`}`;
      ol.appendChild(li);
    }
  }

  renderStations(plan) {
    const ul = this.el.stationList;
    ul.innerHTML = '';
    const stations = [...plan.stationLevel].sort((a, b) => b[1] - a[1] || this.name(a[0]).localeCompare(this.name(b[0])));
    const convs = [...plan.converters.keys()].sort((a, b) => this.name(a).localeCompare(this.name(b)));
    const tools = [...plan.tools].sort();
    this.el.stationsPanel.hidden = !stations.length && !convs.length && !tools.length;
    const listed = new Set(this.list.filter(e => e.n > 0).map(e => e.k));
    const status = own => own ? '<span class="ok">in the plan</span>' : this.stations ? '<span class="muted">counted in the materials</span>' : '<span class="muted">not counted</span>';
    for (const [station, lv] of stations) {
      const st = this.d.stations[station];
      const have = plan.have(station, true);
      const li = document.createElement('li');
      let text = '';
      if (lv > 1) {
        const ext = (st && st.ext) || [];
        const needExt = lv - 1, haveExt = have > 0 ? have - 1 : 0;
        const extra = Math.max(0, needExt - haveExt);
        const counted = ext.filter(e => !listed.has(e)).slice(0, extra);
        text = `<span class="plan-station-lv">level ${lv}</span>` + (st && lv > st.max ? ` <span class="warn">beyond its top level of ${st.max}</span>` : '') +
          `<span class="plan-station-ext">${needExt === ext.length ? 'every upgrade' : `any ${needExt === 1 ? 'one' : needExt} of its ${ext.length} upgrades`}` +
          (extra === 0 ? ', in the plan' : this.stations ? ', counted: ' + counted.map(e => chipHTML(e, this.d.things[e])).join(' ') : `, ${extra} not counted`) + '</span>';
      }
      li.innerHTML = `${chipHTML(station, this.d.things[station])} ${text} <span class="plan-station-have">${status(have > 0)}</span>`;
      ul.appendChild(li);
    }
    for (const conv of convs) {
      const li = document.createElement('li');
      li.innerHTML = `${chipHTML(conv, this.d.things[conv])} <span class="plan-station-have">${status(listed.has(conv))}</span>`;
      ul.appendChild(li);
    }
    if (tools.length) {
      const li = document.createElement('li');
      li.className = 'plan-tools';
      li.innerHTML = `<span class="muted">Placed with</span> ${tools.map(t => chipHTML(t, this.d.things[t])).join(' ')}`;
      ul.appendChild(li);
    }
  }
}

// ---------- bits ----------

const esc = s => String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const pretty = k => k.replace(/^piece_/, '').replace(/_/g, ' ').replace(/([a-z])([A-Z])/g, '$1 $2');
const fmt = n => Math.round(n).toLocaleString('en');
const fmtWeight = w => (w >= 100 ? Math.round(w) : Math.round(w * 10) / 10).toLocaleString('en') + ' weight';
const fmtStacks = (s, size) => s < 1 ? `${Math.ceil(s * 100)}% of ${size}` : s === Math.floor(s) ? `${s} × ${size}` : `${Math.ceil(s)} × ${size}`;
const fmtTime = s => s < 60 ? `${Math.round(s)} s` : s < 3600 ? `${Math.round(s / 60)} min` : `${Math.floor(s / 3600)} h ${Math.round((s % 3600) / 60)} min`;

function iconHTML(t, size) {
  return t && t.i ? `<img src="${base}icons/${t.i}.png" alt="" loading="lazy" width="${size}" height="${size}">` : '<span class="blank"></span>';
}

// chipHTML is the site's chip: icon and name linking to the thing's
// page, with its key for the hover card, and a count when given.
function chipHTML(k, t, n) {
  const name = esc(t ? t.n : pretty(k));
  const icon = t && t.i ? `<img class="chip-icon" src="${base}icons/${t.i}.png" alt="" loading="lazy" width="28" height="28">` : '<span class="chip-icon chip-blank"></span>';
  const count = n > 1 ? `<span class="chip-n">×${fmt(n)}</span>` : '';
  const hint = t && t.h ? ` <small class="muted">${esc(t.h)}</small>` : '';
  const title = t ? ` title="${name}${t.s ? ' · ' + esc(t.s) : ''}"` : '';
  if (t && t.u) return `<a class="chip" href="${base}${t.u}" data-tip="${esc(k)}"${title}>${icon}<span class="chip-name">${name}${hint}</span>${count}</a>`;
  return `<span class="chip ${t ? 'chip-still' : 'chip-dead'}" data-tip="${esc(k)}"${title}>${icon}<span class="chip-name">${name}${hint}</span>${count}</span>`;
}
