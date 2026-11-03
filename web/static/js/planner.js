// The planner: the reader lists pieces to build and items to make, and
// the page adds up what it costs. Each thing's recipe is followed down
// through what it is made of — a sword to its bronze, the bronze to its
// copper and tin, those to their ore and the coal the smelter burns, the
// coal to the wood the kiln chars — until something is found rather than
// made. What is found is the materials list, split by what a portal will
// carry. What is made along the way is listed as steps, each of which
// can be marked "bring it" to carry it ready-made instead. The stations
// the recipes ask for, at the levels the qualities need, are listed and,
// unless the reader has them, costed too.
//
// A reader keeps any number of named plans, in localStorage. A plan is
// one or more stages — a playthrough's biomes, say — each with its own
// list; what an earlier stage builds counts as standing in the later
// ones, so the forge raised in the Black Forest is not costed again in
// the Swamp, unless a stage is marked a fresh base. A link carries one
// whole plan in its hash and opens as a plan of the reader's own.

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
    this.owned = opts.owned || new Set(); // pieces built in earlier stages
    this.listed = new Set(list.filter(e => e.n > 0).map(e => e.k));
    this.auto = [];             // station pieces counted without being listed
    this.run(opts.stations);
    // What stands once this stage is done, for the next one.
    this.built = new Set([...this.owned, ...this.listed, ...this.auto]);
  }

  // has is whether a piece stands by this stage: built before, listed or
  // counted in.
  has(k) { return this.owned.has(k) || this.listed.has(k) || this.auto.includes(k); }

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

  // missingStations is what the levels and converters the sum asks for
  // still lack: the station, then as many of its upgrades as the level
  // needs beyond the ones already standing, in the game's order.
  missingStations() {
    const out = [];
    for (const [station, lv] of this.stationLevel) {
      const ext = (this.d.stations[station] || {}).ext || [];
      if (!this.has(station)) out.push(station);
      let got = 1 + ext.filter(e => this.has(e)).length;
      for (const e of ext) {
        if (got >= lv) break;
        if (!this.has(e)) { out.push(e); got++; }
      }
    }
    for (const conv of this.converters.keys()) if (!this.has(conv)) out.push(conv);
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

// levelIn is the level a station stands at among the pieces has says
// stand: 0 without the station, then one more per upgrade piece.
function levelIn(d, has, station) {
  if (!has(station)) return 0;
  const ext = (d.stations[station] || {}).ext || [];
  return 1 + ext.filter(has).length;
}

// stagePlans works out every stage of a plan in order, each starting
// from what the ones before it built.
function stagePlans(d, plan) {
  const out = [];
  let owned = new Set();
  for (const st of plan.stages) {
    const p = new Plan(d, st.list, { bring: new Set(st.bring), from: new Map(st.from), stations: plan.stations, owned: st.fresh ? new Set() : owned });
    out.push(p);
    owned = st.fresh ? p.built : new Set([...owned, ...p.built]);
  }
  return out;
}

// totalOf adds the stages up into one view of the same shape: materials
// and makes summed, station levels at their highest.
function totalOf(plans) {
  const t = { need: new Map(), steps: new Map(), order: [], stationLevel: new Map(), converters: new Map(), tools: new Set(), owned: new Set(), listed: new Set(), auto: [] };
  for (const p of plans) {
    for (const [k, n] of p.need) t.need.set(k, (t.need.get(k) || 0) + n);
    for (const k of p.order) {
      const s = p.steps.get(k);
      const prev = t.steps.get(k);
      if (!prev) {
        t.steps.set(k, { ...s, inputs: s.kind === 'recipe' ? new Map(s.inputs) : s.inputs });
        t.order.push(k);
        continue;
      }
      prev.n += s.n;
      prev.top = prev.top || s.top;
      if (s.kind === 'recipe' && prev.kind === 'recipe') { prev.batches += s.batches; for (const [i, c] of s.inputs) prev.inputs.set(i, (prev.inputs.get(i) || 0) + c); }
      else if (s.kind === 'conv' && prev.kind === 'conv') { prev.inputs += s.inputs; prev.fuelN += s.fuelN; prev.secs += s.secs; }
    }
    for (const [k, lv] of p.stationLevel) t.stationLevel.set(k, Math.max(t.stationLevel.get(k) || 0, lv));
    for (const k of p.converters.keys()) t.converters.set(k, true);
    for (const k of p.tools) t.tools.add(k);
    for (const k of p.listed) t.listed.add(k);
    for (const k of p.auto) if (!t.auto.includes(k)) t.auto.push(k);
  }
  return t;
}

// ---------- the page ----------

const STORE = 'vegvisir.plans';
const OLD_STORE = 'vegvisir.plan';
const ALL = -1; // the stage index of the all-stages view

const newID = () => Date.now().toString(36) + Math.random().toString(36).slice(2, 7);
const clampN = n => Math.max(1, Math.min(9999, Math.floor(+n) || 1));

// A stage as kept: its name, its list, what is carried ready-made, the
// ore chosen where there are several, and whether it is a fresh base.
const newStage = name => ({ name, list: [], bring: [], from: [], fresh: false });
const newPlan = (name, stages) => ({ id: newID(), name, stations: true, at: 0, stages: stages.map(newStage) });

class Planner {
  constructor(root, data) {
    this.root = root;
    this.d = data;
    try { this.biomes = JSON.parse(root.dataset.biomes || '[]'); } catch (e) { this.biomes = []; }
    const q = sel => $(sel, root);
    this.el = {
      pick: q('[data-plan-pick]'), name: q('[data-plan-name]'), rename: q('[data-plan-rename]'),
      newPlan: q('[data-plan-new]'), newPlay: q('[data-plan-new-play]'), dup: q('[data-plan-dup]'), del: q('[data-plan-del]'),
      stages: q('[data-plan-stages]'), add: q('.plan-add'),
      search: q('[data-plan-search]'), results: q('[data-plan-results]'),
      stageHead: q('[data-plan-stage-head]'), stageName: q('[data-plan-stage-name]'), stageTools: q('[data-plan-stage-tools]'),
      fresh: q('[data-plan-fresh]'), freshRow: q('[data-plan-fresh-row]'), allHead: q('[data-plan-all-head]'),
      entries: q('[data-plan-entries]'), empty: q('[data-plan-empty]'), count: q('[data-plan-count]'), rows: q('[data-plan-stage-rows]'),
      out: q('[data-plan-out]'), materials: q('[data-plan-materials]'), matCount: q('[data-plan-mat-count]'), matTitle: q('[data-plan-mat-title]'),
      stepsPanel: q('[data-plan-steps-panel]'), steps: q('[data-plan-steps]'),
      stationsPanel: q('[data-plan-stations-panel]'), stationList: q('[data-plan-station-list]'),
      stationsBox: q('[data-plan-stations]'), share: q('[data-plan-share]'), clear: q('[data-plan-clear]'), note: q('[data-plan-shared]'),
    };
    this.index = this.buildIndex();
    this.load();
    this.importHash();
    this.bind();
    this.bindSearch();
    window.addEventListener('hashchange', () => { if (this.importHash()) this.changed(); });
    this.render();
  }

  // ---- state ----

  get plan() { return this.plans.find(p => p.id === this.current) || this.plans[0]; }
  get stage() { const p = this.plan; return p.at === ALL ? null : p.stages[Math.min(p.at, p.stages.length - 1)]; }

  load() {
    this.plans = [];
    try {
      const s = JSON.parse(localStorage.getItem(STORE) || 'null');
      if (s && Array.isArray(s.plans)) { this.plans = s.plans.map(p => this.clean(p)).filter(Boolean); this.current = s.current; }
    } catch (e) { /* nothing kept */ }
    // The planner's first shape kept one list: it becomes a plan.
    if (!this.plans.length) {
      try {
        const old = JSON.parse(localStorage.getItem(OLD_STORE) || 'null');
        if (old && Array.isArray(old.list) && old.list.length) {
          const p = newPlan('My plan', ['Stage 1']);
          Object.assign(p.stages[0], { list: old.list, bring: old.bring || [], from: old.from || [] });
          p.stations = old.stations !== false;
          this.plans.push(this.clean(p));
        }
      } catch (e) { /* nothing kept */ }
    }
    if (!this.plans.length) this.plans.push(newPlan('My plan', ['Stage 1']));
    if (!this.plans.some(p => p.id === this.current)) this.current = this.plans[0].id;
  }

  // clean keeps what a stored or shared plan says that the data knows.
  clean(p) {
    if (!p || !Array.isArray(p.stages)) return null;
    const stages = p.stages.filter(Boolean).map((st, i) => ({
      name: String(st.name || `Stage ${i + 1}`).slice(0, 60),
      list: (st.list || []).filter(e => e && this.d.things[e.k]).map(e => ({ k: e.k, n: clampN(e.n), q: Math.max(1, Math.min(maxQuality(this.d, e.k), +e.q || 1)) })),
      bring: (st.bring || []).filter(k => this.d.things[k]),
      from: (st.from || []).filter(x => Array.isArray(x) && x.length === 2 && this.d.things[x[0]] && this.d.things[x[1]]),
      fresh: !!st.fresh,
    }));
    if (!stages.length) stages.push(newStage('Stage 1'));
    const at = p.at === ALL && stages.length > 1 ? ALL : Math.max(0, Math.min(stages.length - 1, +p.at || 0));
    return { id: String(p.id || newID()), name: String(p.name || 'Plan').slice(0, 60), stations: p.stations !== false, at, stages };
  }

  save() {
    try { localStorage.setItem(STORE, JSON.stringify({ v: 1, current: this.current, plans: this.plans })); } catch (e) { /* private mode */ }
  }

  changed() { this.save(); this.render(); }

  // ---- links ----

  // A link: #plan=<the plan as compact JSON, base64url>. The planner's
  // first links, #list=SwordBronze.1.3,forge.1&bring=...&from=..., still
  // open, as a plan of one stage.
  encode(p) {
    const o = { n: p.name, s: p.stages.map(st => {
      const x = { n: st.name, l: st.list.map(e => e.q > 1 ? [e.k, e.n, e.q] : [e.k, e.n]) };
      if (st.bring.length) x.b = st.bring;
      if (st.from.length) x.f = st.from;
      if (st.fresh) x.r = 1;
      return x;
    }) };
    if (!p.stations) o.x = 0;
    return b64(JSON.stringify(o));
  }

  decode(h) {
    const p = new URLSearchParams(h.replace(/^#/, ''));
    if (p.has('plan')) {
      const o = JSON.parse(unb64(p.get('plan')));
      return this.clean({ name: o.n, stations: o.x !== 0, stages: (o.s || []).map(x => ({ name: x.n, list: (x.l || []).map(([k, n, q]) => ({ k, n, q })), bring: x.b, from: x.f, fresh: x.r === 1 })) });
    }
    if (p.has('list')) {
      const list = p.get('list').split(',').filter(Boolean).map(s => { const [k, n, q] = s.split('.'); return { k, n, q }; });
      const bring = (p.get('bring') || '').split(',').filter(Boolean);
      const from = (p.get('from') || '').split(',').filter(Boolean).map(s => s.split('.'));
      return this.clean({ name: 'Shared plan', stations: p.get('stations') !== '0', stages: [{ name: 'Stage 1', list, bring, from }] });
    }
    return null;
  }

  // importHash opens a linked plan as one of the reader's own: the same
  // plan already kept is opened again rather than copied, and the hash
  // is then dropped so a reload does not bring it in twice.
  importHash() {
    if (!location.hash) return false;
    let p = null;
    try { p = this.decode(location.hash); } catch (e) { p = null; }
    if (!p) return false;
    history.replaceState(null, '', location.pathname + location.search);
    const code = this.encode(p);
    const same = this.plans.find(x => this.encode(x) === code);
    if (same) { this.current = same.id; this.say(`Opened “${same.name}”, which you already had.`); return true; }
    // A first visit's empty default plan makes way for the shared one.
    if (this.plans.length === 1 && this.plans[0].stages.every(st => !st.list.length)) this.plans = [];
    p.name = this.freeName(p.name);
    this.plans.push(p);
    this.current = p.id;
    this.say(`Opened the shared plan as “${p.name}”. It is yours now: changes stay in this browser.`);
    this.save();
    return true;
  }

  freeName(name) {
    if (!this.plans.some(p => p.name === name)) return name;
    for (let i = 2; ; i++) if (!this.plans.some(p => p.name === `${name} (${i})`)) return `${name} (${i})`;
  }

  async share() {
    const url = location.origin + location.pathname + '#plan=' + this.encode(this.plan);
    // A clipboard that neither takes the link nor says no, as some
    // embedded or locked-down browsers do, is treated as a no.
    const timeout = new Promise((_, no) => setTimeout(() => no(new Error('no answer')), 1500));
    try {
      await Promise.race([navigator.clipboard.writeText(url), timeout]);
      this.say('Link copied. It holds this whole plan, every stage, and opens as a copy for whoever follows it.');
    } catch (e) {
      history.replaceState(null, '', url);
      this.say('Copying was refused: the link is in the address bar now.');
    }
  }

  say(text) {
    this.el.note.textContent = text;
    this.el.note.hidden = false;
    clearTimeout(this.noteTimer);
    this.noteTimer = setTimeout(() => { this.el.note.hidden = true; }, 7000);
  }

  // ---- plans and stages ----

  bind() {
    const el = this.el;
    el.pick.addEventListener('change', () => { this.current = el.pick.value; this.changed(); });
    // Renaming swaps the picker for a box with the name in it, and back
    // on Enter, Escape or leaving it.
    el.rename.addEventListener('click', () => this.renaming(true));
    el.name.addEventListener('input', () => { this.plan.name = el.name.value.slice(0, 60) || 'Plan'; this.save(); });
    el.name.addEventListener('keydown', e => { if (e.key === 'Enter' || e.key === 'Escape') { e.preventDefault(); el.name.blur(); } });
    el.name.addEventListener('blur', () => this.renaming(false));
    el.newPlan.addEventListener('click', () => this.addPlan(newPlan(this.freeName('New plan'), ['Stage 1'])));
    el.newPlay.addEventListener('click', () => this.addPlan(newPlan(this.freeName('Playthrough'), this.biomes.length ? this.biomes : ['Stage 1'])));
    el.dup.addEventListener('click', () => {
      const p = this.clean(JSON.parse(JSON.stringify(this.plan)));
      p.id = newID();
      p.name = this.freeName(this.plan.name + ' copy');
      this.addPlan(p);
    });
    el.del.addEventListener('click', () => {
      // Two clicks, so a plan is not lost to a stray one.
      if (!el.del.classList.contains('is-armed')) {
        el.del.classList.add('is-armed');
        el.del.textContent = 'Delete for good?';
        clearTimeout(this.delTimer);
        this.delTimer = setTimeout(() => this.disarm(), 4000);
        return;
      }
      this.disarm();
      const gone = this.plan.name;
      this.plans = this.plans.filter(p => p.id !== this.current);
      if (!this.plans.length) this.plans.push(newPlan('My plan', ['Stage 1']));
      this.current = this.plans[0].id;
      this.say(`Deleted “${gone}”.`);
      this.changed();
    });
    el.stages.addEventListener('click', e => {
      const b = e.target.closest('button');
      if (!b) return;
      const p = this.plan;
      if (b.dataset.stage !== undefined) p.at = +b.dataset.stage;
      if (b.dataset.addStage !== undefined) {
        p.stages.push(newStage(this.nextStageName(p)));
        p.at = p.stages.length - 1;
      }
      this.changed();
      if (b.dataset.addStage !== undefined) { el.stageName.focus(); el.stageName.select(); }
    });
    el.stageName.addEventListener('input', () => { if (this.stage) { this.stage.name = el.stageName.value.slice(0, 60) || 'Stage'; this.save(); this.renderStages(); } });
    el.stageTools.addEventListener('click', e => {
      const b = e.target.closest('button[data-stage-act]');
      const p = this.plan;
      if (!b || p.at === ALL) return;
      const i = p.at;
      if (b.dataset.stageAct === 'earlier' && i > 0) { [p.stages[i - 1], p.stages[i]] = [p.stages[i], p.stages[i - 1]]; p.at = i - 1; }
      if (b.dataset.stageAct === 'later' && i < p.stages.length - 1) { [p.stages[i + 1], p.stages[i]] = [p.stages[i], p.stages[i + 1]]; p.at = i + 1; }
      if (b.dataset.stageAct === 'remove' && p.stages.length > 1) {
        if (p.stages[i].list.length && !b.classList.contains('is-armed')) {
          b.classList.add('is-armed');
          b.textContent = 'Remove with its list?';
          setTimeout(() => { b.classList.remove('is-armed'); b.textContent = 'Remove stage'; }, 4000);
          return;
        }
        p.stages.splice(i, 1);
        p.at = Math.max(0, i - 1);
      }
      this.changed();
    });
    el.fresh.addEventListener('change', () => { if (this.stage) { this.stage.fresh = el.fresh.checked; this.changed(); } });
    el.rows.addEventListener('click', e => {
      const b = e.target.closest('button[data-stage]');
      if (b) { this.plan.at = +b.dataset.stage; this.changed(); }
    });
    el.stationsBox.addEventListener('change', () => { this.plan.stations = el.stationsBox.checked; this.changed(); });
    el.clear.addEventListener('click', () => {
      const st = this.stage;
      if (!st) return;
      Object.assign(st, { list: [], bring: [], from: [] });
      this.changed();
      el.search.focus();
    });
    el.share.addEventListener('click', () => this.share());
    el.entries.addEventListener('input', e => this.onEntryInput(e));
    el.entries.addEventListener('change', e => this.onEntryInput(e));
    el.entries.addEventListener('click', e => this.onEntryClick(e));
    el.steps.addEventListener('click', e => this.onStepClick(e));
    el.materials.addEventListener('click', e => this.onStepClick(e));
  }

  disarm() {
    clearTimeout(this.delTimer);
    this.el.del.classList.remove('is-armed');
    this.el.del.textContent = 'Delete';
  }

  addPlan(p) {
    this.plans.push(p);
    this.current = p.id;
    this.changed();
    this.renaming(true);
  }

  renaming(on) {
    const el = this.el;
    if (on) {
      el.name.value = this.plan.name;
      el.pick.closest('.plan-pick').hidden = true;
      el.rename.hidden = true;
      el.name.hidden = false;
      el.name.focus();
      el.name.select();
      return;
    }
    el.name.hidden = true;
    el.pick.closest('.plan-pick').hidden = false;
    el.rename.hidden = false;
    this.renderPick();
  }

  // nextStageName follows the biomes when the stage before is one.
  nextStageName(p) {
    const last = p.stages[p.stages.length - 1];
    const i = last ? this.biomes.indexOf(last.name) : -1;
    if (i >= 0 && i + 1 < this.biomes.length && !p.stages.some(st => st.name === this.biomes[i + 1])) return this.biomes[i + 1];
    return `Stage ${p.stages.length + 1}`;
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
    document.addEventListener('click', e => { if (!this.el.add.contains(e.target)) close(); });
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
    const st = this.stage;
    if (!st) return;
    const e = st.list.find(x => x.k === k);
    const per = this.batch(k);
    if (e) e.n = Math.min(9999, e.n + per);
    else st.list.push({ k, n: per, q: 1 });
    this.changed();
    const row = [...this.el.entries.children].find(li => li.dataset.k === k);
    if (row) row.classList.add('is-new');
  }

  onEntryInput(e) {
    const st = this.stage;
    const row = e.target.closest('[data-k]');
    if (!st || !row) return;
    const entry = st.list.find(x => x.k === row.dataset.k);
    if (!entry) return;
    if (e.target.matches('[data-n]')) entry.n = clampN(e.target.value);
    if (e.target.matches('[data-q]')) entry.q = +e.target.value || 1;
    this.save();
    this.renderOut();
    this.renderStages();
  }

  onEntryClick(e) {
    const st = this.stage;
    const b = e.target.closest('button[data-act]');
    if (!st || !b) return;
    const i = st.list.findIndex(x => x.k === b.closest('[data-k]').dataset.k);
    if (i < 0) return;
    const per = this.batch(st.list[i].k);
    if (b.dataset.act === 'remove') st.list.splice(i, 1);
    else if (b.dataset.act === 'more') st.list[i].n = Math.min(9999, st.list[i].n + per);
    else if (b.dataset.act === 'less') st.list[i].n = Math.max(1, st.list[i].n - per);
    this.changed();
  }

  onStepClick(e) {
    const st = this.stage;
    const b = e.target.closest('button[data-bring], button[data-make], button[data-from]');
    if (!st || !b) return;
    if (b.dataset.bring && !st.bring.includes(b.dataset.bring)) st.bring.push(b.dataset.bring);
    if (b.dataset.make) st.bring = st.bring.filter(k => k !== b.dataset.make);
    if (b.dataset.from) st.from = [...st.from.filter(x => x[0] !== b.dataset.item), [b.dataset.item, b.dataset.from]];
    this.changed();
  }

  // ---- drawing ----

  render() {
    this.plansNow = stagePlans(this.d, this.plan);
    this.renderPick();
    this.renderStages();
    this.renderList();
    this.renderOut();
  }

  renderPick() {
    const el = this.el, p = this.plan;
    el.pick.innerHTML = '';
    for (const x of this.plans) {
      const o = document.createElement('option');
      o.value = x.id;
      o.textContent = x.name;
      o.selected = x.id === p.id;
      el.pick.appendChild(o);
    }
    el.stationsBox.checked = p.stations;
  }

  renderStages() {
    const p = this.plan, el = this.el.stages;
    el.innerHTML = '';
    p.stages.forEach((st, i) => {
      const b = document.createElement('button');
      b.type = 'button';
      b.className = 'pill plan-stage-pill';
      b.dataset.stage = i;
      b.setAttribute('aria-pressed', String(p.at === i));
      b.textContent = st.name;
      if (st.list.length) { const n = document.createElement('span'); n.className = 'n'; n.textContent = st.list.length; b.appendChild(n); }
      el.appendChild(b);
    });
    const add = document.createElement('button');
    add.type = 'button';
    add.className = 'pill plan-stage-add';
    add.dataset.addStage = '';
    add.textContent = '+ Stage';
    el.appendChild(add);
    if (p.stages.length > 1) {
      const all = document.createElement('button');
      all.type = 'button';
      all.className = 'pill plan-stage-all';
      all.dataset.stage = ALL;
      all.setAttribute('aria-pressed', String(p.at === ALL));
      all.textContent = 'All stages';
      el.appendChild(all);
    }
  }

  renderList() {
    const el = this.el, p = this.plan, st = this.stage;
    const all = !st;
    el.add.hidden = all;
    el.stageHead.hidden = all;
    el.allHead.hidden = !all;
    el.entries.hidden = all;
    el.rows.hidden = !all;
    el.clear.hidden = all;
    if (all) {
      el.empty.hidden = true;
      this.renderStageRows();
      return;
    }
    const i = p.stages.indexOf(st);
    const key = p.id + ':' + i;
    if (document.activeElement !== el.stageName || el.stageName.dataset.for !== key) { el.stageName.value = st.name; el.stageName.dataset.for = key; }
    el.stageTools.hidden = p.stages.length < 2;
    $('[data-stage-act="earlier"]', el.stageTools).disabled = i === 0;
    $('[data-stage-act="later"]', el.stageTools).disabled = i === p.stages.length - 1;
    el.freshRow.hidden = i === 0;
    el.fresh.checked = st.fresh;
    el.search.placeholder = p.stages.length > 1 ? `Add to ${st.name}: a piece or an item to make…` : 'Add a piece or an item to make: portal, forge, bronze sword…';
    el.count.textContent = st.list.length ? `${st.list.length} ${st.list.length === 1 ? 'thing' : 'things'}` : '';
    el.empty.hidden = st.list.length > 0;
    const ul = el.entries;
    ul.innerHTML = '';
    for (const e of st.list) {
      const t = this.d.things[e.k];
      const li = document.createElement('li');
      li.className = 'plan-entry';
      li.dataset.k = e.k;
      const maxQ = maxQuality(this.d, e.k);
      if (e.q > maxQ) e.q = maxQ;
      li.innerHTML = `${chipHTML(e.k, t)}<span class="plan-entry-sub"></span>
        <span class="plan-qty"><button type="button" class="plan-pm" data-act="less" aria-label="Fewer">−</button><input type="number" inputmode="numeric" min="1" max="9999" data-n value="${e.n}" aria-label="How many"><button type="button" class="plan-pm" data-act="more" aria-label="More">+</button></span>
        ${maxQ > 1 ? `<select class="plan-q" data-q aria-label="Quality">${Array.from({ length: maxQ }, (_, i) => `<option value="${i + 1}"${e.q === i + 1 ? ' selected' : ''}>quality ${i + 1}</option>`).join('')}</select>` : ''}
        <button type="button" class="plan-x" data-act="remove" aria-label="Remove">×</button>`;
      li.querySelector('.plan-entry-sub').textContent = this.sub(e.k, t);
      ul.appendChild(li);
    }
  }

  // renderStageRows is the all-stages view's list: each stage with what
  // it holds and weighs, a click away.
  renderStageRows() {
    const p = this.plan, ul = this.el.rows;
    ul.innerHTML = '';
    this.el.count.textContent = '';
    p.stages.forEach((st, i) => {
      const plan = this.plansNow[i];
      const weight = [...plan.need].reduce((a, [k, n]) => a + n * ((this.d.things[k] || {}).w || 0), 0);
      const li = document.createElement('li');
      li.innerHTML = `<button type="button" class="plan-stage-row" data-stage="${i}"><span class="plan-stage-row-name"></span><span class="muted small">${st.list.length ? `${st.list.length} ${st.list.length === 1 ? 'thing' : 'things'} · ${plan.need.size} ${plan.need.size === 1 ? 'material' : 'materials'} · ${fmtWeight(weight)}` : 'nothing yet'}${st.fresh ? ' · fresh base' : ''}</span></button>`;
      li.querySelector('.plan-stage-row-name').textContent = st.name;
      ul.appendChild(li);
    });
  }

  renderOut() {
    this.plansNow = stagePlans(this.d, this.plan);
    const p = this.plan, st = this.stage;
    const view = st ? this.plansNow[p.stages.indexOf(st)] : totalOf(this.plansNow);
    const any = st ? st.list.length > 0 : p.stages.some(x => x.list.length);
    this.el.out.hidden = !any;
    if (!any) return;
    this.el.matTitle.textContent = st ? (p.stages.length > 1 ? `Materials for ${st.name}` : 'Materials') : 'Materials, all stages';
    this.renderMaterials(view, !!st);
    this.renderSteps(view, !!st);
    this.renderStations(view);
  }

  renderMaterials(plan, live) {
    const st = this.stage;
    const rows = [...plan.need].map(([k, n]) => ({ k, n, t: this.d.things[k] })).filter(r => r.n > 0);
    rows.sort((a, b) => b.n - a.n || (a.t ? a.t.n : a.k).localeCompare(b.t ? b.t.n : b.k));
    const portal = rows.filter(r => !r.t || r.t.tp), heavy = rows.filter(r => r.t && !r.t.tp);
    this.el.matCount.textContent = rows.length ? `${rows.length} ${rows.length === 1 ? 'kind' : 'kinds'} · ${fmtWeight(rows.reduce((a, r) => a + r.n * (r.t ? r.t.w || 0 : 0), 0))}` : '';
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
        const made = live && st && st.bring.includes(r.k);
        tr.innerHTML = `<td>${chipHTML(r.k, r.t)}</td><td class="num"><b>${fmt(r.n)}</b></td><td class="num muted">${stacks ? fmtStacks(stacks, r.t.st) : ''}</td><td class="num muted">${r.t && r.t.w ? fmtWeight(r.n * r.t.w) : ''}</td><td class="plan-act">${made ? `<button type="button" class="link small" data-make="${r.k}">make it</button>` : ''}</td>`;
        tb.appendChild(tr);
      }
      el.appendChild(sec);
    };
    group('Through a portal', 'Carried in the inventory and fine through a portal.', portal);
    group('Not through a portal', 'The game marks these as not teleportable: they go by foot, cart or boat.', heavy);
  }

  renderSteps(plan, live) {
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
        const alt = a => a === s.from ? `<b>${esc(this.name(a))}</b>` : live ? `<button type="button" class="link" data-from="${a}" data-item="${k}">${esc(this.name(a))}</button>` : esc(this.name(a));
        const alts = s.alts.length > 1 ? ` <span class="plan-step-alts">from ${s.alts.map(alt).join(' or ')}</span>` : '';
        const fuel = s.fuel ? ' ' + chipHTML(s.fuel, this.d.things[s.fuel], s.fuelN) : '';
        how = `<span class="plan-step-how">in the ${chipHTML(s.piece, this.d.things[s.piece])}${alts}${s.secs ? ` <span class="muted">· ${fmtTime(s.secs)} in one</span>` : ''}</span><span class="plan-step-from">${chipHTML(s.from, this.d.things[s.from], s.inputs)}${fuel}</span>`;
      }
      if (s.top) li.classList.add('is-goal');
      const q = s.kind === 'recipe' && s.q > 1 ? ` <span class="muted">at quality ${s.q}</span>` : '';
      const bring = live && !s.top ? `<button type="button" class="link small plan-step-bring" data-bring="${k}" title="Carry it ready-made instead of what makes it">bring it</button>` : '';
      li.innerHTML = `<div class="plan-step-what">${chipHTML(k, t, s.n)}${q}</div><div class="plan-step-body">${how}</div>${bring}`;
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
    const counting = this.plan.stations;
    const earlier = k => plan.owned.has(k), here = k => plan.owned.has(k) || plan.listed.has(k);
    const status = k => plan.listed.has(k) ? '<span class="ok">in the plan</span>'
      : earlier(k) ? '<span class="ok">built in an earlier stage</span>'
      : counting ? '<span class="muted">counted in the materials</span>' : '<span class="muted">not counted</span>';
    for (const [station, lv] of stations) {
      const ext = (this.d.stations[station] || {}).ext || [];
      const max = (this.d.stations[station] || {}).max || 1;
      const li = document.createElement('li');
      let text = '';
      if (lv > 1) {
        const needExt = lv - 1;
        const standing = levelIn(this.d, here, station);
        const extra = Math.max(0, needExt - Math.max(0, standing - 1));
        const counted = ext.filter(e => !here(e)).slice(0, extra);
        // Where the upgrades come from, said only when it is not where
        // the station itself does.
        const upEarlier = levelIn(this.d, earlier, station) >= lv;
        const where = extra > 0 ? (counting ? ', counted: ' + counted.map(e => chipHTML(e, this.d.things[e])).join(' ') : `, ${extra} not counted`)
          : upEarlier && !plan.listed.has(station) ? '' : upEarlier ? ', built in an earlier stage' : plan.listed.has(station) ? '' : ', in the plan';
        text = `<span class="plan-station-lv">level ${lv}</span>` + (lv > max ? ` <span class="warn">beyond its top level of ${max}</span>` : '') +
          `<span class="plan-station-ext">${needExt === ext.length ? 'every upgrade' : `any ${needExt === 1 ? 'one' : needExt} of its ${ext.length} upgrades`}${where}</span>`;
      }
      li.innerHTML = `${chipHTML(station, this.d.things[station])} ${text} <span class="plan-station-have">${status(station)}</span>`;
      ul.appendChild(li);
    }
    for (const conv of convs) {
      const li = document.createElement('li');
      li.innerHTML = `${chipHTML(conv, this.d.things[conv])} <span class="plan-station-have">${status(conv)}</span>`;
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
// b64 and unb64 carry text in a URL: UTF-8, then base64 with the
// URL-safe alphabet and no padding.
const b64 = s => btoa(Array.from(new TextEncoder().encode(s), c => String.fromCharCode(c)).join('')).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
const unb64 = s => new TextDecoder().decode(Uint8Array.from(atob(s.replace(/-/g, '+').replace(/_/g, '/')), c => c.charCodeAt(0)));
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
