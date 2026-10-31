// The achievements page's progress: a character file (.fch) dropped on
// the page is read here in the browser, and each achievement shows how
// far along that character is. The file is the game's own save: a
// length-prefixed package holding the profile, which opens with one
// statistics table per difficulty category — counters by PlayerStatType,
// then name-keyed tallies of kills (five lists, by kill modifier), item
// pickups, crafts, pickables, food eaten and pieces placed. Those are the
// very dictionaries the achievement triggers read, keyed by the same
// tokens the page carries in data attributes, so the check is a lookup.
// The file is kept in IndexedDB so it is still there next visit.

const $ = (sel, el = document) => el.querySelector(sel);
const $$ = (sel, el = document) => Array.from(el.querySelectorAll(sel));

const panel = $('[data-ach-file]');
const list = $('[data-ach-list]');

// ---------- the save file ----------

const utf8 = new TextDecoder();

class Reader {
  constructor(bytes) {
    this.b = bytes;
    this.v = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    this.p = 0;
  }
  need(n) { if (n < 0 || this.p + n > this.b.length) throw new Error('the file ends early'); }
  u8() { this.need(1); return this.b[this.p++]; }
  bool() { return this.u8() !== 0; }
  i32() { this.need(4); const x = this.v.getInt32(this.p, true); this.p += 4; return x; }
  f32() { this.need(4); const x = this.v.getFloat32(this.p, true); this.p += 4; return x; }
  skip(n) { this.need(n); this.p += n; }
  bytes() { const n = this.i32(); this.need(n); const s = this.b.subarray(this.p, this.p + n); this.p += n; return s; }
  // A .NET string: a 7-bit length, then UTF-8.
  str() {
    let n = 0, shift = 0, c;
    do {
      c = this.u8();
      n |= (c & 0x7f) << shift;
      shift += 7;
      if (shift > 35) throw new Error('bad string length');
    } while (c & 0x80);
    this.need(n);
    const s = utf8.decode(this.b.subarray(this.p, this.p + n));
    this.p += n;
    return s;
  }
  // A Dictionary<string, float>: count, then pairs.
  named() {
    const n = this.i32();
    if (n < 0 || n > 1e6) throw new Error('bad table length');
    const m = new Map();
    for (let i = 0; i < n; i++) { const k = this.str(); m.set(k, this.f32()); }
    return m;
  }
}

// parseCharacter reads the statistics tables and the character's name
// out of a character file, in the layout of profile versions 46 and 47
// (game 1.0.17 to 1.0.20), which the game's PlayerProfile writes.
export function parseCharacter(bytes) {
  const outer = new Reader(bytes);
  const data = outer.bytes();
  outer.bytes(); // the SHA-512 of data, informational
  const r = new Reader(data);
  const version = r.i32();
  if (version !== 46 && version !== 47) throw new Error(`this is a version ${version} character file; versions 46 and 47 (game 1.0.17 to 1.0.20) can be read`);
  const numStats = r.i32(), numTables = r.i32();
  if (numStats < 0 || numStats > 4096 || numTables < 0 || numTables > 64) throw new Error('bad statistics header');
  const tables = [];
  for (let i = 0; i < numTables; i++) {
    const counters = new Float32Array(numStats);
    for (let j = 0; j < numStats; j++) counters[j] = r.f32();
    r.named(); r.named(); r.named(); // known worlds, world keys, commands
    const nk = r.i32();
    if (nk < 0 || nk > 64) throw new Error('bad kill table count');
    const kills = [];
    for (let k = 0; k < nk; k++) kills.push(r.named());
    const pickedUp = r.named(), crafts = r.named(), caught = r.named(), eaten = r.named(), pieces = r.named();
    tables.push({ counters, kills, pickedUp, crafts, caught, eaten, pieces });
  }
  r.bool(); // first spawn
  const nw = r.i32();
  if (nw < 0 || nw > 100000) throw new Error('bad world count');
  for (let i = 0; i < nw; i++) {
    r.skip(8); // world key
    r.bool(); r.skip(12); // custom spawn
    r.bool(); r.skip(12); // logout point
    r.bool(); r.skip(12); // death point
    r.skip(12); // home point
    if (r.bool()) r.bytes(); // map data
  }
  const name = r.str();
  return { version, name, tables };
}

// ---------- storage ----------

const DB = 'vegvisir-character', STORE = 'files', KEY = 'character';

function db() {
  return new Promise((resolve, reject) => {
    if (typeof indexedDB === 'undefined') { reject(new Error('no IndexedDB')); return; }
    const req = indexedDB.open(DB, 1);
    req.onupgradeneeded = () => { if (!req.result.objectStoreNames.contains(STORE)) req.result.createObjectStore(STORE); };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error || new Error('IndexedDB open failed'));
  });
}
async function load() {
  try {
    const d = await db();
    return await new Promise((resolve, reject) => {
      const req = d.transaction(STORE, 'readonly').objectStore(STORE).get(KEY);
      req.onsuccess = () => resolve(req.result || null);
      req.onerror = () => reject(req.error);
    });
  } catch (e) { return null; }
}
async function save(rec) {
  try {
    const d = await db();
    await new Promise((resolve, reject) => {
      const tx = d.transaction(STORE, 'readwrite');
      if (rec) tx.objectStore(STORE).put(rec, KEY); else tx.objectStore(STORE).delete(KEY);
      tx.oncomplete = resolve;
      tx.onerror = () => reject(tx.error);
    });
  } catch (e) { /* private mode: the page still works for this visit */ }
}

// ---------- progress ----------

const fmt = (n) => n >= 1000 ? Math.round(n).toLocaleString('en') : (Math.round(n * 10) / 10).toLocaleString('en');
const km = (m) => (m / 1000).toFixed(1) + ' km';

// apply marks every need and stat line done or not for one character,
// and sums each achievement into a bar. An achievement with another as
// a need is settled after that one, so the pass repeats until nothing
// changes.
function apply(ch) {
  const done = new Map();
  const secs = $$('.ach[data-ach]', list);
  const sums = new Map();
  for (const sec of secs) {
    // The trigger reads the table of the achievement's difficulty
    // requirement: 1 is any, 6 default or harder, 7 hard or harder.
    const diff = +sec.dataset.diff || 1;
    const t = ch.tables[diff] || ch.tables[1] || ch.tables[0];
    let n = 0, total = 0;
    for (const li of $$('[data-need]', sec)) {
      const kind = li.dataset.need;
      const dict = kind === 'kills' ? t.kills[0] : t[kind];
      const v = dict ? dict.get(li.dataset.token) : undefined;
      const ok = v !== undefined && v >= (+li.dataset.amount || 0);
      li.classList.toggle('is-done', ok);
      let you = li.querySelector('.ach-you');
      if (v !== undefined && +li.dataset.amount > 1) {
        if (!you) { you = document.createElement('span'); you.className = 'ach-you'; li.appendChild(you); }
        you.textContent = fmt(v) + ' so far';
      } else if (you) you.remove();
      total++; if (ok) n++;
    }
    for (const li of $$('[data-stat]', sec)) {
      const v = t.counters[+li.dataset.stat] || 0;
      const amt = +li.dataset.amount || 0;
      const ok = v >= amt;
      li.classList.toggle('is-done', ok);
      let you = li.querySelector('.ach-you');
      if (!you) { you = document.createElement('span'); you.className = 'ach-you'; li.appendChild(you); }
      you.textContent = ok && amt <= 1 ? 'done' : (li.dataset.key.startsWith('Distance') ? km(v) : fmt(v)) + (amt > 1 ? ' so far' : '');
      total++; if (ok) n++;
    }
    sums.set(sec.dataset.ach, { sec, n, total, others: $$('[data-other]', sec) });
  }
  // Settle the ones that need other achievements first.
  for (let pass = 0; pass < 4; pass++) {
    for (const [id, s] of sums) {
      let n = s.n, total = s.total;
      for (const li of s.others) {
        const o = sums.get(li.dataset.other);
        const ok = !!o && o.n === o.total && o.total > 0;
        li.classList.toggle('is-done', ok);
        total++; if (ok) n++;
      }
      done.set(id, total > 0 && n === total);
      s.shownN = n; s.shownTotal = total;
    }
  }
  let unlocked = 0;
  for (const s of sums.values()) {
    const ok = done.get(s.sec.dataset.ach);
    if (ok) unlocked++;
    s.sec.classList.toggle('is-unlocked', ok);
    let bar = s.sec.querySelector('.ach-bar');
    if (!bar) {
      bar = document.createElement('div');
      bar.className = 'ach-bar';
      bar.innerHTML = '<span class="ach-bar-fill"></span><span class="ach-bar-text"></span>';
      s.sec.querySelector('.ach-text').appendChild(bar);
    }
    bar.querySelector('.ach-bar-fill').style.width = (s.shownTotal ? 100 * s.shownN / s.shownTotal : 0) + '%';
    bar.querySelector('.ach-bar-text').textContent = ok ? 'Unlocked' : `${s.shownN} of ${s.shownTotal}`;
  }
  list.classList.add('has-file');
  return { unlocked, total: secs.length };
}

function clear() {
  list.classList.remove('has-file');
  for (const el of $$('.is-done, .is-unlocked', list)) el.classList.remove('is-done', 'is-unlocked');
  for (const el of $$('.ach-bar, .ach-you', list)) el.remove();
}

// ---------- the panel ----------

function init() {
  const input = $('[data-ach-input]', panel);
  const loaded = $('[data-ach-loaded]', panel);
  const nameEl = $('[data-ach-name]', panel);
  const summary = $('[data-ach-summary]', panel);
  const errEl = $('[data-ach-error]', panel);
  const text = $('[data-ach-file-text]', panel);
  panel.hidden = false;

  const fail = (msg) => { errEl.textContent = msg; errEl.hidden = false; };

  function show(ch, when) {
    errEl.hidden = true;
    const p = apply(ch);
    nameEl.textContent = ch.name || 'This character';
    summary.textContent = `has unlocked ${p.unlocked} of ${p.total}` + (when ? `, as of the file from ${when}` : '') + '.';
    loaded.hidden = false;
    text.hidden = true;
  }

  async function take(file) {
    if (!file) return;
    let ch;
    const bytes = new Uint8Array(await file.arrayBuffer());
    try { ch = parseCharacter(bytes); } catch (e) { fail(`Could not read ${file.name}: ${e.message}.`); return; }
    const when = new Date(file.lastModified).toLocaleDateString('en-GB', { day: 'numeric', month: 'long', year: 'numeric' });
    show(ch, when);
    save({ bytes, when, name: file.name });
  }

  input.addEventListener('change', () => { take(input.files[0]); input.value = ''; });
  for (const ev of ['dragenter', 'dragover']) panel.addEventListener(ev, e => { e.preventDefault(); panel.classList.add('is-over'); });
  for (const ev of ['dragleave', 'drop']) panel.addEventListener(ev, e => { e.preventDefault(); panel.classList.remove('is-over'); });
  panel.addEventListener('drop', e => take(e.dataTransfer.files[0]));
  // A file dropped anywhere on the page counts too.
  document.addEventListener('dragover', e => e.preventDefault());
  document.addEventListener('drop', e => { e.preventDefault(); if (!panel.contains(e.target)) take(e.dataTransfer.files[0]); });

  $('[data-ach-forget]', panel).addEventListener('click', () => {
    clear();
    loaded.hidden = true;
    text.hidden = false;
    save(null);
  });

  load().then(rec => {
    if (!rec) return;
    try { show(parseCharacter(rec.bytes), rec.when); } catch (e) { save(null); }
  });
}

if (panel && list) init();
