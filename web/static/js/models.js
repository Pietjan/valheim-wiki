// The models the 3D view stands on the ground: the game's meshes as
// cmd/wiki-models packs them under static/models/ — VMSH files, the
// materials they paint with and their textures — read into GPU buffers
// on demand.
//
//   const store = new ModelStore(gl, base + 'static/models/', () => view.invalidate());
//   await store.ready;                 // index.json and materials.json
//   store.has(name)                    // whether the pack holds it
//   const m = store.get(name);         // a Model once loaded, null while fetching
//
// A Model is one VAO — positions, smooth normals computed here, texture
// coordinates — with its index buffer, the runs of indices per material
// and a bounding sphere. A material is its texture (sRGB, mipmapped,
// repeating), tint, and whether it cuts out by alpha. The frame is the
// game's: x east, y up, z north, as the places the engine hands out.

const MAGIC = 0x48534d56;

// Shaders that draw no lit surface: particles, beams, decals. Their runs
// are left out, as the icon renderer leaves them out.
const SKIP_SHADER = /^(Unlit\/BeaconBeam|Custom\/LitParticles|Custom\/WaterMask|Custom\/InteriorSide|Custom\/Decal|Sprites\/Default|Particles\/|Legacy Shaders\/Particles|Lux Lit Particles|Custom\/Blob)/;

// decode reads a VMSH buffer: positions, triangles, the runs per material,
// the snapped parts, and from version 4 the texture coordinates.
export function decode(buf) {
  const view = new DataView(buf);
  if (view.getUint32(0, true) !== MAGIC) throw new Error('not a VMSH file');
  const version = view.getUint32(4, true);
  const nv = view.getUint32(8, true), ni = view.getUint32(12, true);
  const positions = new Float32Array(buf.slice(16, 16 + nv * 12));
  let at = 16 + nv * 12;
  const indices = new Uint32Array(buf.slice(at, at + ni * 4));
  at += ni * 4;
  const runs = [];
  if (version >= 2) {
    const n = view.getUint32(at, true);
    at += 4;
    const text = new TextDecoder();
    for (let i = 0; i < n; i++) {
      const first = view.getUint32(at, true), count = view.getUint32(at + 4, true), len = view.getUint32(at + 8, true);
      at += 12;
      runs.push({ first, count, material: text.decode(new Uint8Array(buf, at, len)) });
      at += len;
    }
  }
  const parts = [];
  if (version >= 3) {
    const n = view.getUint32(at, true);
    at += 4;
    for (let i = 0; i < n; i++) {
      parts.push({ anchor: [view.getFloat32(at, true), view.getFloat32(at + 4, true), view.getFloat32(at + 8, true)], offset: view.getFloat32(at + 12, true), first: view.getUint32(at + 16, true), count: view.getUint32(at + 20, true) });
      at += 24;
    }
  }
  let uvs = null;
  if (version >= 4 && buf.byteLength >= at + nv * 8) {
    uvs = new Float32Array(buf.slice(at, at + nv * 8));
  }
  return { positions, indices, runs, parts, uvs };
}

// normals are the area-weighted sum of the face normals over shared
// vertices, normalised: what the files lack. The game's triangles wind
// clockwise seen from outside in its left-handed frame, which makes
// cross(b - a, c - a) point inward there; it is flipped.
export function normals(positions, indices) {
  const n = new Float32Array(positions.length);
  for (let i = 0; i < indices.length; i += 3) {
    const a = indices[i] * 3, b = indices[i + 1] * 3, c = indices[i + 2] * 3;
    const abx = positions[b] - positions[a], aby = positions[b + 1] - positions[a + 1], abz = positions[b + 2] - positions[a + 2];
    const acx = positions[c] - positions[a], acy = positions[c + 1] - positions[a + 1], acz = positions[c + 2] - positions[a + 2];
    const nx = -(aby * acz - abz * acy), ny = -(abz * acx - abx * acz), nz = -(abx * acy - aby * acx);
    n[a] += nx; n[a + 1] += ny; n[a + 2] += nz;
    n[b] += nx; n[b + 1] += ny; n[b + 2] += nz;
    n[c] += nx; n[c + 1] += ny; n[c + 2] += nz;
  }
  for (let i = 0; i < n.length; i += 3) {
    const l = Math.hypot(n[i], n[i + 1], n[i + 2]);
    if (l > 0) { n[i] /= l; n[i + 1] /= l; n[i + 2] /= l; } else { n[i + 1] = 1; }
  }
  return n;
}

export class ModelStore {
  constructor(gl, base, onLoad) {
    this.gl = gl;
    this.base = base;
    this.onLoad = onLoad || (() => {});
    this.index = null;
    this.materials = null;
    this.models = new Map();     // name → Model | 'loading' | 'failed'
    this.textures = new Map();   // file → {tex, ready}
    this.aniso = gl.getExtension('EXT_texture_filter_anisotropic');
    this.ready = Promise.all([
      fetch(base + 'index.json').then(r => r.ok ? r.json() : { models: {} }),
      fetch(base + 'materials.json').then(r => r.ok ? r.json() : {}),
    ]).then(([index, materials]) => {
      this.index = index.models || {};
      this.materials = materials || {};
    }).catch(err => {
      console.warn('models:', err);
      this.index = {};
      this.materials = {};
    });
  }

  // has is whether the pack holds a model; false until the index is read.
  has(name) { return !!(this.index && this.index[name] != null); }

  // get is the model, or null while it loads (or when it cannot).
  get(name) {
    const m = this.models.get(name);
    if (m && m !== 'loading' && m !== 'failed') return m;
    if (m) return null;
    if (!this.has(name)) return null;
    this.models.set(name, 'loading');
    fetch(this.base + encodeURIComponent(name) + '.bin').then(r => {
      if (!r.ok) throw new Error(r.status);
      return r.arrayBuffer();
    }).then(buf => {
      const model = this._upload(name, decode(buf));
      this.models.set(name, model);
      this.onLoad(name);
    }).catch(err => {
      console.warn('model ' + name + ':', err && err.message || err);
      this.models.set(name, 'failed');
    });
    return null;
  }

  _upload(name, mesh) {
    const { gl } = this;
    const { positions, indices, uvs } = mesh;
    const nrm = normals(positions, indices);
    const vao = gl.createVertexArray();
    gl.bindVertexArray(vao);
    const vbo = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, vbo);
    gl.bufferData(gl.ARRAY_BUFFER, positions, gl.STATIC_DRAW);
    gl.enableVertexAttribArray(0);
    gl.vertexAttribPointer(0, 3, gl.FLOAT, false, 0, 0);
    const nbo = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, nbo);
    gl.bufferData(gl.ARRAY_BUFFER, nrm, gl.STATIC_DRAW);
    gl.enableVertexAttribArray(1);
    gl.vertexAttribPointer(1, 3, gl.FLOAT, false, 0, 0);
    const tbo = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, tbo);
    gl.bufferData(gl.ARRAY_BUFFER, uvs || new Float32Array(positions.length / 3 * 2), gl.STATIC_DRAW);
    gl.enableVertexAttribArray(2);
    gl.vertexAttribPointer(2, 2, gl.FLOAT, false, 0, 0);
    // The instances: x, y, z, scale and the quaternion, filled per draw.
    const ibo = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, ibo);
    gl.enableVertexAttribArray(3);
    gl.vertexAttribPointer(3, 4, gl.FLOAT, false, 32, 0);
    gl.vertexAttribDivisor(3, 1);
    gl.enableVertexAttribArray(4);
    gl.vertexAttribPointer(4, 4, gl.FLOAT, false, 32, 16);
    gl.vertexAttribDivisor(4, 1);
    const ebo = gl.createBuffer();
    gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, ebo);
    gl.bufferData(gl.ELEMENT_ARRAY_BUFFER, indices, gl.STATIC_DRAW);
    gl.bindVertexArray(null);

    // The runs by material, those with a surface to draw.
    const runs = [];
    for (const r of mesh.runs) {
      const base = r.material.split('#')[0];
      const entry = this.materials[base] || null;
      if (entry && entry.shader && SKIP_SHADER.test(entry.shader)) continue;
      runs.push({ first: r.first, count: r.count, material: this._material(base, entry) });
    }
    // The bounds and the sphere about the origin, for culling.
    let minY = Infinity, maxY = -Infinity, r2 = 0;
    for (let i = 0; i < positions.length; i += 3) {
      const x = positions[i], y = positions[i + 1], z = positions[i + 2];
      if (y < minY) minY = y;
      if (y > maxY) maxY = y;
      const d = x * x + y * y + z * z;
      if (d > r2) r2 = d;
    }
    return { name, vao, ibo, indexCount: indices.length, runs, minY, maxY, radius: Math.sqrt(r2), instances: 0, instanceCap: 0 };
  }

  // _material resolves one material entry to what the draw needs.
  _material(name, entry) {
    const mat = { name, tex: null, color: [1, 1, 1, 1], cutout: false, cutoff: 0.5, scale: [1, 1], offset: [0, 0], tone: toneOf(name) };
    if (!entry) return mat;
    if (entry.color) mat.color = entry.color.map(toLinear);
    mat.cutout = !!entry.cutout;
    mat.cutoff = entry.cutoff != null ? entry.cutoff : 0.5;
    if (entry.scale) mat.scale = entry.scale;
    if (entry.offset) mat.offset = entry.offset;
    if (entry.texture) mat.tex = this._texture(entry.texture);
    return mat;
  }

  _texture(file) {
    let t = this.textures.get(file);
    if (t) return t;
    const { gl } = this;
    const tex = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, tex);
    // One grey texel until the image arrives.
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.SRGB8_ALPHA8, 1, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array([140, 140, 140, 255]));
    t = { tex, ready: false };
    this.textures.set(file, t);
    const img = new Image();
    img.onload = () => {
      gl.bindTexture(gl.TEXTURE_2D, tex);
      gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, true);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.SRGB8_ALPHA8, gl.RGBA, gl.UNSIGNED_BYTE, img);
      gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, false);
      gl.generateMipmap(gl.TEXTURE_2D);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT);
      if (this.aniso) gl.texParameterf(gl.TEXTURE_2D, this.aniso.TEXTURE_MAX_ANISOTROPY_EXT, Math.min(8, gl.getParameter(this.aniso.MAX_TEXTURE_MAX_ANISOTROPY_EXT)));
      t.ready = true;
      this.onLoad(file);
    };
    img.onerror = () => console.warn('texture ' + file + ' did not load');
    img.src = this.base + 'textures/' + encodeURIComponent(file);
    return t;
  }

  destroy() {
    const { gl } = this;
    for (const m of this.models.values()) if (m && m.vao) { gl.deleteVertexArray(m.vao); }
    for (const t of this.textures.values()) gl.deleteTexture(t.tex);
    this.models.clear();
    this.textures.clear();
  }
}

// toLinear is Unity's GammaToLinearSpace: an sRGB component decoded.
export function toLinear(c) {
  if (c <= 0.04045) return c / 12.92;
  return Math.pow((c + 0.055) / 1.055, 2.4);
}

// toneOf is the colour a material without an entry keeps, from its name:
// the icon renderer's rules.
const TONES = [
  [/ashlandstree|ashlandsbranch|ashlandslog|charred|ember|burnt/i, 0x3a3330],
  [/leaf|leaves|needle|foliage|branch|shrub|bush|fern|grass|vine|ivy|moss/i, 0x4f8f4a],
  [/straw|thatch|\breed|\bhay\b/i, 0xc4a34a],
  [/bark|trunk|\bstem|wood|plank|timber|log|stump|beam|stake/i, 0x7a5a3a],
  [/metal|iron|bronze|copper|\btin\b|silver|steel|flametal/i, 0x8f8f96],
  [/rock|stone|cliff|marble|granite|boulder|gravel|slate/i, 0x6e6a63],
  [/\bice\b|\bice[0-9_]|snow|frost/i, 0x9fc4d6],
  [/cloth|banner|linen|\bfur\b|hide|leather|carpet|\brug\b/i, 0xa87f5c],
];
function toneOf(name) {
  let hex = 0x9a8f80;
  for (const [re, h] of TONES) if (re.test(name)) { hex = h; break; }
  return [toLinear(((hex >> 16) & 255) / 255), toLinear(((hex >> 8) & 255) / 255), toLinear((hex & 255) / 255)];
}
