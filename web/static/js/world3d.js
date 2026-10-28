// The world in three dimensions: the seed engine's height raster as a
// terrain, the sea as a plane over it, close-up tiles sampled from the
// generator as the camera nears the ground, and the site markers standing
// on the land.
//
//   import { World3D } from './world3d.js';
//   const v = new World3D(container, { assetsBase });
//   v.setWorld(World.open(seed));            // the raster as it arrives, tiles as the camera asks
//   v.setMarkers([{x, z, img, size, kind, color, label, priority, data}]);
//   v.setFilter(m => ...);
//   v.onClick(({x, z, marker}) => ...);      // x, z on the ground under the pointer
//   v.lookAt(x, z, { distance, yaw, pitch, animate });
//   v.setRelief(2);                          // vertical exaggeration about sea level
//
// Terrain is drawn without vertex buffers: a 64-quad patch, instanced, whose
// vertices read their height from the layer's R32F texture by gl_VertexID
// and the patch's grid origin. Three layers nest — the whole world at the
// raster's 24 m, then two tiles the engine samples around the camera's
// target at about 8 m and 2 m. A tile fades to its parent's height along
// its edge, and the parent's vertices under a tile sink out of sight, so the
// levels meet without cracks. Patches are culled against the frustum each
// frame. The sea is one quad at the water level, tinted by the depth under
// it; the sky is a gradient behind everything.
//
// Under about a kilometre and a half the view asks the engine what stands
// in the zones around the camera's target — the vegetation the zone build
// scatters and the sites' models — and draws them instanced from the
// pack under static/models/ (models.js), nearest zones first.
//
// Over the GL canvas sits the same 2D overlay the map has: round site icons
// projected onto the ground, scaled by their distance, decluttered when
// crowded. Drag to pan, right-drag (or shift, or two fingers) to orbit and
// tilt, the wheel or a pinch to zoom, double-click to fly closer. The
// compass is the caller's: onView reports the yaw.

import { markerImage, declutter, drawMarkerDisc, drawMarkerLabel } from './worldmap.js';
import { ModelStore } from './models.js';

const MAP_RADIUS = 12288;
const WORLD_RADIUS = 10500;
const HEIGHT_OFFSET = 512, HEIGHT_SCALE = 16;
const WATER = 30;

// The close-up levels: span in metres and samples a side. Each is asked
// for once the camera's target is within `show` metres and recentred once
// the target has moved `recentre` of the span from the tile's middle.
const LEVELS = [
  { span: 2400, n: 320, show: 3800, recentre: 0.22 },
  { span: 600, n: 320, show: 950, recentre: 0.22 },
];

const FLIP_Z = new Float32Array([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, -1, 0, 0, 0, 0, 1]);
// The scene: zones are asked for under SCENE_SHOW metres of camera
// distance, within a radius that grows with it, and forgotten past it.
const SCENE_SHOW = 1500;
const ZONE = 64;
const ZONE_RADIUS = d => Math.max(140, Math.min(420, d * 0.9));
const ZONE_INFLIGHT = 4;

const PATCH = 64;                 // quads a side per instanced patch
const SKIRT = 1;                  // extra ring of vertices, pushed down
const VERTS = PATCH + 1 + 2 * SKIRT;
const FOV = 50 * Math.PI / 180;
const MIN_PITCH = 6 * Math.PI / 180, MAX_PITCH = 89 * Math.PI / 180;
const MIN_DIST = 40, MAX_DIST = 36000;
const DECLUTTER_ABOVE = 300;
const CLICK_SLOP = 4;
const DBLCLICK_WAIT = 260;

// ---------------------------------------------------------------- shaders

const TERRAIN_VERT = `#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;
precision highp usampler2D;

in vec2 aPatch;                 // instance: the patch's origin in vertex-grid units
uniform mat4 uViewProj;
uniform sampler2D uH;           // heights, metres, n×n, row zero north
uniform int uN;                 // samples a side
uniform int uStep;              // samples per vertex (1, or 2 for a decimated base)
uniform vec2 uOrigin;           // world x, z of sample (0, 0)'s centre
uniform float uCell;            // metres between samples
uniform float uWater, uRelief;
uniform float uSkirt;           // metres the skirt ring drops

// The parent layer, for the fade along this layer's edge.
uniform int uHasParent;
uniform sampler2D uParentH;
uniform int uParentN;
uniform vec2 uParentOrigin;
uniform float uParentCell;
uniform vec2 uCentre;           // this layer's middle
uniform float uHalf;            // half its span

// The child layer, whose footprint this layer sinks under.
uniform int uHasChild;
uniform vec2 uChildCentre;
uniform float uChildHalf;

out vec3 vWorld;                // x, exaggerated y, z
out float vH;                   // the true height
out float vSink;

float sampleH(sampler2D tex, int n, ivec2 t) {
  t = clamp(t, ivec2(0), ivec2(n - 1));
  return texelFetch(tex, t, 0).r;
}

// Bilinear height of a layer at a world position.
float bilinear(sampler2D tex, int n, vec2 origin, float cell, vec2 w) {
  vec2 g = vec2((w.x - origin.x) / cell, (origin.y - w.y) / cell);
  vec2 f = floor(g), k = g - f;
  ivec2 i = ivec2(f);
  float a = sampleH(tex, n, i), b = sampleH(tex, n, i + ivec2(1, 0));
  float c = sampleH(tex, n, i + ivec2(0, 1)), d = sampleH(tex, n, i + ivec2(1, 1));
  return mix(mix(a, b, k.x), mix(c, d, k.x), k.y);
}

void main() {
  int vid = gl_VertexID;
  int li = vid % ${VERTS}, lj = vid / ${VERTS};
  // The ring outside the patch is the border vertex again, dropped.
  int ci = clamp(li - ${SKIRT}, 0, ${PATCH}), cj = clamp(lj - ${SKIRT}, 0, ${PATCH});
  bool skirt = (ci != li - ${SKIRT}) || (cj != lj - ${SKIRT});
  ivec2 g = (ivec2(aPatch) + ivec2(ci, cj)) * uStep;
  g = min(g, ivec2(uN - 1));
  vec2 w = vec2(uOrigin.x + float(g.x) * uCell, uOrigin.y - float(g.y) * uCell);
  float h = texelFetch(uH, g, 0).r;

  if (uHasParent == 1) {
    // Along the edge the tile becomes its parent, so the two surfaces meet.
    vec2 u = abs(w - uCentre) / uHalf;
    float edge = max(u.x, u.y);
    float fade = smoothstep(0.86, 1.0, edge);
    if (fade > 0.0) h = mix(h, bilinear(uParentH, uParentN, uParentOrigin, uParentCell, w), fade);
  }
  float sink = 0.0;
  if (uHasChild == 1) {
    // Under the child the parent sinks: its surface there would fight the
    // tile's, and the wall it makes at the child's edge hides any crack.
    vec2 u = abs(w - uChildCentre);
    if (max(u.x, u.y) < uChildHalf - 0.5 * uCell) sink = 1.0;
  }
  float y = uWater + (h - uWater) * uRelief;
  if (skirt) y -= uSkirt;
  if (sink > 0.0) y -= 80.0 * uRelief + uSkirt;
  vWorld = vec3(w.x, y, w.y);
  vH = h;
  vSink = sink;
  gl_Position = uViewProj * vec4(vWorld, 1.0);
}`;

const TERRAIN_FRAG = `#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;
precision highp usampler2D;

in vec3 vWorld;
in float vH;
in float vSink;
out vec4 outColor;

uniform sampler2D uH;
uniform usampler2D uB;
uniform sampler2D uM;
uniform int uN;
uniform vec2 uOrigin;
uniform float uCell;
uniform float uWater, uRelief;
uniform vec3 uCamera;
uniform vec3 uSun;
uniform vec3 uFog;
uniform float uFogK;
uniform float uDetail;          // 0..1: how much close-up grain to draw

const int MEADOWS = 1, SWAMP = 2, MOUNTAIN = 3, BLACKFOREST = 4, PLAINS = 5,
          ASHLANDS = 6, DEEPNORTH = 7, OCEAN = 9, MISTLANDS = 10;
const float WORLD = ${WORLD_RADIUS}.0;

float hAt(ivec2 t) { t = clamp(t, ivec2(0), ivec2(uN - 1)); return texelFetch(uH, t, 0).r; }
int bAt(ivec2 t) { t = clamp(t, ivec2(0), ivec2(uN - 1)); return int(texelFetch(uB, t, 0).r); }

// Height between samples, the weights eased so the slope is continuous
// across a cell and the shading does not show the grid.
float hSmooth(vec2 g) {
  vec2 f = floor(g), k = g - f;
  k = k * k * (3.0 - 2.0 * k);
  ivec2 i = ivec2(f);
  float a = hAt(i), b = hAt(i + ivec2(1, 0)), c = hAt(i + ivec2(0, 1)), d = hAt(i + ivec2(1, 1));
  return mix(mix(a, b, k.x), mix(c, d, k.x), k.y);
}

vec2 gridOf(vec2 w) { return vec2((w.x - uOrigin.x) / uCell, (uOrigin.y - w.y) / uCell); }

float hash(vec2 p) {
  p = fract(p * vec2(0.1031, 0.1030));
  p += dot(p, p.yx + 33.33);
  return fract((p.x + p.y) * p.x);
}
float vnoise(vec2 p) {
  vec2 i = floor(p), f = fract(p);
  f = f * f * (3.0 - 2.0 * f);
  return mix(mix(hash(i), hash(i + vec2(1, 0)), f.x), mix(hash(i + vec2(0, 1)), hash(i + vec2(1, 1)), f.x), f.y);
}

// sRGB colours as the eye reads them, decoded for the lighting.
vec3 lin(vec3 c) { return pow(c, vec3(2.2)); }

// The ground of one biome: its colour, with the forest, mist and lava the
// mask carries, the snow and rock of the slopes, and the shore.
vec3 land(int b, vec2 w, vec4 m, float h, float slope, float grain) {
  vec3 c;
  float above = h - uWater;
  if (b == MEADOWS) {
    c = lin(vec3(0.47, 0.60, 0.29));
    c = mix(c, lin(vec3(0.22, 0.36, 0.17)), 0.55 * m.r);
  } else if (b == BLACKFOREST) {
    c = lin(vec3(0.24, 0.35, 0.20));
    c = mix(c, lin(vec3(0.12, 0.20, 0.11)), 0.6 * m.r);
  } else if (b == SWAMP) {
    c = lin(vec3(0.33, 0.31, 0.21));
    c = mix(c, lin(vec3(0.20, 0.21, 0.14)), smoothstep(6.0, 0.0, above));
  } else if (b == PLAINS) {
    c = lin(vec3(0.72, 0.65, 0.36));
    c = mix(c, lin(vec3(0.45, 0.42, 0.20)), 0.5 * m.r);
  } else if (b == MISTLANDS) {
    c = lin(vec3(0.30, 0.29, 0.34));
    c = mix(c, lin(vec3(0.62, 0.64, 0.72)), 0.55 * m.g * (0.6 + 0.4 * vnoise(w / 40.0)));
  } else if (b == ASHLANDS) {
    c = lin(vec3(0.21, 0.16, 0.14));
    float lava = smoothstep(0.35, 0.8, m.b + 0.25 * vnoise(w / 9.0) - 0.12);
    c = mix(c, vec3(2.4, 0.55, 0.08), lava);
  } else if (b == MOUNTAIN || b == DEEPNORTH) {
    vec3 snow = lin(vec3(0.90, 0.93, 0.96));
    vec3 rock = lin(vec3(0.40, 0.39, 0.41));
    float bare = smoothstep(0.55, 1.1, slope) * (0.7 + 0.3 * vnoise(w / 30.0));
    c = mix(snow, rock, bare);
    if (b == MOUNTAIN) c = mix(lin(vec3(0.44, 0.47, 0.36)), c, smoothstep(85.0, 110.0, h));
  } else if (b == OCEAN) {
    c = lin(vec3(0.36, 0.40, 0.34));
  } else {
    c = lin(vec3(0.30, 0.32, 0.30));
  }
  // Rock where it is steep, anywhere.
  if (b != MOUNTAIN && b != DEEPNORTH && b != ASHLANDS) {
    c = mix(c, lin(vec3(0.36, 0.34, 0.33)), smoothstep(1.2, 2.2, slope));
  }
  // The shore: sand around the waterline, mud in the swamp.
  vec3 sand = b == SWAMP ? lin(vec3(0.27, 0.25, 0.18)) : lin(vec3(0.70, 0.64, 0.47));
  c = mix(sand, c, smoothstep(0.4, 2.2, above));
  c *= 1.0 + grain;
  return c;
}

void main() {
  vec2 w = vWorld.xz;
  float rim = length(w) - WORLD;
  if (rim > 0.0) discard;

  vec2 g = gridOf(w);
  vec2 f = floor(g), k = g - f;
  ivec2 i = ivec2(f);
  vec2 ke = k * k * (3.0 - 2.0 * k);

  // The normal from the height field, half a cell either side, in the
  // drawn (exaggerated) space.
  vec2 gc = g - 0.5;
  float hl = hSmooth(gc + vec2(-0.5, 0.0)), hr = hSmooth(gc + vec2(0.5, 0.0));
  float hd = hSmooth(gc + vec2(0.0, 0.5)), hu = hSmooth(gc + vec2(0.0, -0.5));
  float dhdx = (hr - hl) / uCell, dhdz = (hu - hd) / uCell;
  vec3 n = normalize(vec3(-dhdx * uRelief, 1.0, -dhdz * uRelief));
  float slope = length(vec2(dhdx, dhdz));

  vec4 m = texture(uM, (g + 0.5) / float(uN));
  float grain = 0.0;
  if (uDetail > 0.0) {
    float near = clamp(1.0 - distance(uCamera, vWorld) / 2500.0, 0.0, 1.0) * uDetail;
    grain = ((vnoise(w / 3.0) - 0.5) * 0.09 + (vnoise(w / 0.7) - 0.5) * 0.05) * near;
  }
  grain += (vnoise(w / 45.0) - 0.5) * 0.10;

  int a = bAt(i), b = bAt(i + ivec2(1, 0)), c = bAt(i + ivec2(0, 1)), d = bAt(i + ivec2(1, 1));
  vec3 colour;
  if (a == b && a == c && a == d) {
    colour = land(a, w, m, vH, slope, grain);
  } else {
    colour = mix(mix(land(a, w, m, vH, slope, grain), land(b, w, m, vH, slope, grain), ke.x),
                 mix(land(c, w, m, vH, slope, grain), land(d, w, m, vH, slope, grain), ke.x), ke.y);
  }
  if (vSink > 0.0) colour *= 0.35;

  // Light: the sun, a sky-blue ambient, and the slopes facing away darker.
  float lambert = max(dot(n, uSun), 0.0);
  vec3 sunColour = vec3(1.0, 0.95, 0.86);
  vec3 skyColour = vec3(0.55, 0.65, 0.82);
  vec3 lit = colour * (sunColour * lambert * 0.95 + skyColour * (0.20 + 0.12 * n.y));
  // Beneath the sea the ground darkens with depth; the water above tints it.
  float depth = max(uWater - vH, 0.0);
  lit *= exp(-depth * 0.06);

  // Haze with distance, towards the sky's horizon.
  float dist = distance(uCamera, vWorld);
  float fog = 1.0 - exp(-dist * uFogK);
  lit = mix(lit, uFog, fog);
  outColor = vec4(pow(max(lit, 0.0), vec3(1.0 / 2.2)), 1.0);
}`;

const WATER_VERT = `#version 300 es
precision highp float;
uniform mat4 uViewProj;
uniform float uWater;
uniform float uRadius;
out vec3 vWorld;
void main() {
  // One quad over the grid.
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2) * 2.0 - 1.0;
  vWorld = vec3(p.x * uRadius, uWater, p.y * uRadius);
  gl_Position = uViewProj * vec4(vWorld, 1.0);
}`;

const WATER_FRAG = `#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;
in vec3 vWorld;
out vec4 outColor;

// The finest layer under a point tells the depth.
uniform sampler2D uH0, uH1, uH2;
uniform int uN0, uN1, uN2;
uniform vec2 uO0, uO1, uO2;
uniform float uC0, uC1, uC2;
uniform int uLayers;
uniform vec2 uCentre1, uCentre2;
uniform float uHalf1, uHalf2;

uniform float uWater, uRelief;
uniform vec3 uCamera;
uniform vec3 uSun;
uniform vec3 uFog;
uniform float uFogK;
uniform float uTime;

const float WORLD = ${WORLD_RADIUS}.0;

float sampleH(sampler2D tex, int n, ivec2 t) { t = clamp(t, ivec2(0), ivec2(n - 1)); return texelFetch(tex, t, 0).r; }
float bilinear(sampler2D tex, int n, vec2 origin, float cell, vec2 w) {
  vec2 g = vec2((w.x - origin.x) / cell, (origin.y - w.y) / cell);
  vec2 f = floor(g), k = g - f;
  ivec2 i = ivec2(f);
  float a = sampleH(tex, n, i), b = sampleH(tex, n, i + ivec2(1, 0));
  float c = sampleH(tex, n, i + ivec2(0, 1)), d = sampleH(tex, n, i + ivec2(1, 1));
  return mix(mix(a, b, k.x), mix(c, d, k.x), k.y);
}
float ground(vec2 w) {
  if (uLayers > 2 && max(abs(w.x - uCentre2.x), abs(w.y - uCentre2.y)) < uHalf2) return bilinear(uH2, uN2, uO2, uC2, w);
  if (uLayers > 1 && max(abs(w.x - uCentre1.x), abs(w.y - uCentre1.y)) < uHalf1) return bilinear(uH1, uN1, uO1, uC1, w);
  return bilinear(uH0, uN0, uO0, uC0, w);
}
float hash(vec2 p) { p = fract(p * vec2(0.1031, 0.1030)); p += dot(p, p.yx + 33.33); return fract((p.x + p.y) * p.x); }
float vnoise(vec2 p) {
  vec2 i = floor(p), f = fract(p);
  f = f * f * (3.0 - 2.0 * f);
  return mix(mix(hash(i), hash(i + vec2(1, 0)), f.x), mix(hash(i + vec2(0, 1)), hash(i + vec2(1, 1)), f.x), f.y);
}

void main() {
  vec2 w = vWorld.xz;
  float rim = length(w) - WORLD;
  if (rim > 0.0) discard;
  float h = ground(w);
  float depth = uWater - h;
  if (depth <= 0.0) discard;

  vec3 toCam = uCamera - vWorld;
  float dist = length(toCam);
  vec3 view = toCam / dist;

  // A ripple on the surface, small and slow, only read up close.
  float near = clamp(1.0 - dist / 1800.0, 0.0, 1.0);
  vec2 rip = vec2(vnoise(w / 2.3 + uTime * 0.05) - 0.5, vnoise(w / 1.7 - uTime * 0.04) - 0.5) * 0.08 * near;
  vec3 n = normalize(vec3(rip.x, 1.0, rip.y));

  vec3 shallow = pow(vec3(0.24, 0.50, 0.53), vec3(2.2));
  vec3 deep = pow(vec3(0.05, 0.14, 0.26), vec3(2.2));
  vec3 colour = mix(shallow, deep, 1.0 - exp(-depth / 24.0));
  float fresnel = pow(1.0 - max(dot(n, view), 0.0), 3.0);
  vec3 sky = pow(uFog, vec3(2.2));
  colour = mix(colour, sky, 0.10 + 0.6 * fresnel);
  // The sun's glint.
  vec3 hv = normalize(uSun + view);
  float spec = pow(max(dot(n, hv), 0.0), 220.0) * 0.9;
  colour += vec3(1.0, 0.95, 0.85) * spec;
  // Foam at the waterline.
  float foam = (1.0 - smoothstep(0.0, 0.9, depth)) * (0.55 + 0.45 * vnoise(w / 1.5 + uTime * 0.1));
  colour = mix(colour, vec3(0.8), foam * 0.7);

  float alpha = mix(0.5, 0.96, 1.0 - exp(-depth / 8.0));
  alpha = mix(alpha, 1.0, fresnel * 0.6);
  float fog = 1.0 - exp(-dist * uFogK);
  colour = mix(colour, pow(uFog, vec3(2.2)), fog);
  alpha = mix(alpha, 1.0, fog);
  float edge = smoothstep(-6.0, 0.0, rim);
  outColor = vec4(pow(colour, vec3(1.0 / 2.2)), alpha * (1.0 - edge));
}`;

const SKY_VERT = `#version 300 es
precision highp float;
out vec2 vNDC;
void main() {
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2) * 2.0 - 1.0;
  vNDC = p;
  gl_Position = vec4(p, 0.999999, 1.0);
}`;

const SKY_FRAG = `#version 300 es
precision highp float;
in vec2 vNDC;
out vec4 outColor;
uniform mat4 uInvViewProj;
uniform vec3 uCamera;
uniform vec3 uSun;
uniform vec3 uFog;
void main() {
  vec4 far = uInvViewProj * vec4(vNDC, 1.0, 1.0);
  vec3 dir = normalize(far.xyz / far.w - uCamera);
  vec3 zenith = vec3(0.30, 0.47, 0.72);
  vec3 horizon = uFog;
  float t = smoothstep(-0.02, 0.45, dir.y);
  vec3 c = mix(horizon, zenith, t);
  float sun = pow(max(dot(dir, uSun), 0.0), 600.0);
  c += vec3(1.0, 0.9, 0.7) * sun * 0.8;
  // Below the horizon, the haze the world floats in, darker underneath.
  if (dir.y < 0.0) c = mix(horizon, horizon * 0.55, smoothstep(0.0, -0.5, dir.y));
  outColor = vec4(c, 1.0);
}`;

const MODEL_VERT = `#version 300 es
precision highp float;
layout(location = 0) in vec3 aPos;
layout(location = 1) in vec3 aNrm;
layout(location = 2) in vec2 aUV;
layout(location = 3) in vec4 aInst;     // x, y, z, scale
layout(location = 4) in vec4 aQuat;     // x, y, z, w
uniform mat4 uViewProj;
uniform float uWater, uRelief;
out vec3 vNrm;
out vec2 vUV;
out vec3 vWorld;
vec3 rot(vec4 q, vec3 v) { vec3 t = 2.0 * cross(q.xyz, v); return v + q.w * t + cross(q.xyz, t); }
void main() {
  vec3 p = rot(aQuat, aPos * aInst.w);
  float y = uWater + (aInst.y - uWater) * uRelief;
  vWorld = vec3(aInst.x, y, aInst.z) + p;
  vNrm = rot(aQuat, aNrm);
  vUV = aUV;
  gl_Position = uViewProj * vec4(vWorld, 1.0);
}`;

const MODEL_FRAG = `#version 300 es
precision highp float;
in vec3 vNrm;
in vec2 vUV;
in vec3 vWorld;
out vec4 outColor;
uniform sampler2D uTex;
uniform int uHasTex;
uniform vec4 uColor;
uniform int uCutout;
uniform float uCutoff;
uniform vec2 uScale, uOffset;
uniform vec3 uCamera, uSun, uFog;
uniform float uFogK;
void main() {
  vec4 c = uColor;
  if (uHasTex == 1) c = texture(uTex, vUV * uScale + uOffset) * uColor;
  float alpha = 1.0;
  if (uCutout == 1) {
    if (c.a < uCutoff * 0.6) discard;
    alpha = smoothstep(uCutoff * 0.6, uCutoff * 1.2, c.a);
  }
  vec3 n = normalize(vNrm);
  if (!gl_FrontFacing) n = -n;
  float lambert = uCutout == 1 ? max(dot(n, uSun), 0.0) * 0.55 + 0.45 : max(dot(n, uSun), 0.0);
  vec3 sunColour = vec3(1.0, 0.95, 0.86);
  vec3 skyColour = vec3(0.55, 0.65, 0.82);
  vec3 lit = c.rgb * (sunColour * lambert * 0.95 + skyColour * (0.22 + 0.12 * max(n.y, 0.0)));
  float dist = distance(uCamera, vWorld);
  lit = mix(lit, uFog, 1.0 - exp(-dist * uFogK));
  outColor = vec4(pow(max(lit, 0.0), vec3(1.0 / 2.2)), alpha);
}`;

// ---------------------------------------------------------------- matrices

function perspective(out, fovy, aspect, near, far) {
  const f = 1 / Math.tan(fovy / 2), nf = 1 / (near - far);
  out.fill(0);
  out[0] = f / aspect; out[5] = f; out[10] = (far + near) * nf; out[11] = -1; out[14] = 2 * far * near * nf;
  return out;
}

function lookAt(out, eye, target, up) {
  let zx = eye[0] - target[0], zy = eye[1] - target[1], zz = eye[2] - target[2];
  let l = Math.hypot(zx, zy, zz) || 1; zx /= l; zy /= l; zz /= l;
  let xx = up[1] * zz - up[2] * zy, xy = up[2] * zx - up[0] * zz, xz = up[0] * zy - up[1] * zx;
  l = Math.hypot(xx, xy, xz) || 1; xx /= l; xy /= l; xz /= l;
  const yx = zy * xz - zz * xy, yy = zz * xx - zx * xz, yz = zx * xy - zy * xx;
  out[0] = xx; out[1] = yx; out[2] = zx; out[3] = 0;
  out[4] = xy; out[5] = yy; out[6] = zy; out[7] = 0;
  out[8] = xz; out[9] = yz; out[10] = zz; out[11] = 0;
  out[12] = -(xx * eye[0] + xy * eye[1] + xz * eye[2]);
  out[13] = -(yx * eye[0] + yy * eye[1] + yz * eye[2]);
  out[14] = -(zx * eye[0] + zy * eye[1] + zz * eye[2]);
  out[15] = 1;
  return out;
}

function multiply(out, a, b) {
  const o = new Float32Array(16);
  for (let i = 0; i < 4; i++) for (let j = 0; j < 4; j++) {
    o[j * 4 + i] = a[i] * b[j * 4] + a[4 + i] * b[j * 4 + 1] + a[8 + i] * b[j * 4 + 2] + a[12 + i] * b[j * 4 + 3];
  }
  out.set(o);
  return out;
}

function invert(out, m) {
  const a00 = m[0], a01 = m[1], a02 = m[2], a03 = m[3], a10 = m[4], a11 = m[5], a12 = m[6], a13 = m[7];
  const a20 = m[8], a21 = m[9], a22 = m[10], a23 = m[11], a30 = m[12], a31 = m[13], a32 = m[14], a33 = m[15];
  const b00 = a00 * a11 - a01 * a10, b01 = a00 * a12 - a02 * a10, b02 = a00 * a13 - a03 * a10, b03 = a01 * a12 - a02 * a11;
  const b04 = a01 * a13 - a03 * a11, b05 = a02 * a13 - a03 * a12, b06 = a20 * a31 - a21 * a30, b07 = a20 * a32 - a22 * a30;
  const b08 = a20 * a33 - a23 * a30, b09 = a21 * a32 - a22 * a31, b10 = a21 * a33 - a23 * a31, b11 = a22 * a33 - a23 * a32;
  let det = b00 * b11 - b01 * b10 + b02 * b09 + b03 * b08 - b04 * b07 + b05 * b06;
  if (!det) return null;
  det = 1 / det;
  out[0] = (a11 * b11 - a12 * b10 + a13 * b09) * det; out[1] = (a02 * b10 - a01 * b11 - a03 * b09) * det;
  out[2] = (a31 * b05 - a32 * b04 + a33 * b03) * det; out[3] = (a22 * b04 - a21 * b05 - a23 * b03) * det;
  out[4] = (a12 * b08 - a10 * b11 - a13 * b07) * det; out[5] = (a00 * b11 - a02 * b08 + a03 * b07) * det;
  out[6] = (a32 * b02 - a30 * b05 - a33 * b01) * det; out[7] = (a20 * b05 - a22 * b02 + a23 * b01) * det;
  out[8] = (a10 * b10 - a11 * b08 + a13 * b06) * det; out[9] = (a01 * b08 - a00 * b10 - a03 * b06) * det;
  out[10] = (a30 * b04 - a31 * b02 + a33 * b00) * det; out[11] = (a21 * b02 - a20 * b04 - a23 * b00) * det;
  out[12] = (a11 * b07 - a10 * b09 - a12 * b06) * det; out[13] = (a00 * b09 - a01 * b07 + a02 * b06) * det;
  out[14] = (a31 * b01 - a30 * b03 - a32 * b00) * det; out[15] = (a20 * b03 - a21 * b01 + a22 * b00) * det;
  return out;
}

// frustumPlanes extracts the six planes (a, b, c, d) of a view-projection,
// normals pointing in.
function frustumPlanes(m) {
  const row = i => [m[i], m[4 + i], m[8 + i], m[12 + i]];
  const r0 = row(0), r1 = row(1), r2 = row(2), r3 = row(3);
  const plane = (r, s) => [r3[0] + s * r[0], r3[1] + s * r[1], r3[2] + s * r[2], r3[3] + s * r[3]];
  return [plane(r0, 1), plane(r0, -1), plane(r1, 1), plane(r1, -1), plane(r2, 1), plane(r2, -1)];
}

function boxVisible(planes, x0, y0, z0, x1, y1, z1) {
  for (const [a, b, c, d] of planes) {
    const px = a > 0 ? x1 : x0, py = b > 0 ? y1 : y0, pz = c > 0 ? z1 : z0;
    if (a * px + b * py + c * pz + d < 0) return false;
  }
  return true;
}

// ---------------------------------------------------------------- GL bits

function compile(gl, type, src) {
  const s = gl.createShader(type);
  gl.shaderSource(s, src);
  gl.compileShader(s);
  if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) {
    const log = gl.getShaderInfoLog(s);
    gl.deleteShader(s);
    throw new Error('world3d shader: ' + log);
  }
  return s;
}

function program(gl, vert, frag) {
  const p = gl.createProgram();
  const vs = compile(gl, gl.VERTEX_SHADER, vert), fs = compile(gl, gl.FRAGMENT_SHADER, frag);
  gl.attachShader(p, vs); gl.attachShader(p, fs);
  gl.linkProgram(p);
  gl.deleteShader(vs); gl.deleteShader(fs);
  if (!gl.getProgramParameter(p, gl.LINK_STATUS)) throw new Error('world3d program: ' + gl.getProgramInfoLog(p));
  const cache = new Map();
  const u = new Proxy({}, { get: (_, name) => { if (!cache.has(name)) cache.set(name, gl.getUniformLocation(p, name)); return cache.get(name); } });
  return { p, u };
}

function texture2D(gl, internal, format, type, w, h, data, filter) {
  const t = gl.createTexture();
  gl.bindTexture(gl.TEXTURE_2D, t);
  gl.texImage2D(gl.TEXTURE_2D, 0, internal, w, h, 0, format, type, data);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, filter);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, filter);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
  return t;
}

// A Layer is one height grid on the GPU with what the draw needs of it.
class Layer {
  constructor(gl, { n, cx, cz, span, heights, biomes, mask, step }) {
    this.gl = gl;
    this.n = n; this.cx = cx; this.cz = cz; this.span = span;
    this.cell = span / n;
    this.originX = cx - span / 2 + this.cell / 2;
    this.originZ = cz + span / 2 - this.cell / 2;
    this.heights = heights; this.biomes = biomes; this.mask = mask;
    this.step = step;
    this.texH = texture2D(gl, gl.R32F, gl.RED, gl.FLOAT, n, n, heights, gl.NEAREST);
    this.texB = texture2D(gl, gl.R8UI, gl.RED_INTEGER, gl.UNSIGNED_BYTE, n, n, biomes, gl.NEAREST);
    this.texM = texture2D(gl, gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE, n, n, mask, gl.LINEAR);
    // The patches: a grid over the vertex grid, each with its height range
    // for culling.
    const verts = Math.floor((n - 1) / step) + 1;
    const per = Math.ceil((verts - 1) / PATCH);
    this.patches = [];
    for (let pj = 0; pj < per; pj++) for (let pi = 0; pi < per; pi++) {
      const i0 = pi * PATCH, j0 = pj * PATCH;
      const i1 = Math.min(i0 + PATCH, verts - 1), j1 = Math.min(j0 + PATCH, verts - 1);
      let lo = Infinity, hi = -Infinity;
      for (let j = j0; j <= j1; j++) {
        const row = Math.min(j * step, n - 1) * n;
        for (let i = i0; i <= i1; i++) {
          const h = heights[row + Math.min(i * step, n - 1)];
          if (h < lo) lo = h;
          if (h > hi) hi = h;
        }
      }
      const x0 = this.originX + i0 * step * this.cell, x1 = this.originX + Math.min(i1 * step, n - 1) * this.cell;
      const z1 = this.originZ - j0 * step * this.cell, z0 = this.originZ - Math.min(j1 * step, n - 1) * this.cell;
      this.patches.push({ i0, j0, x0, x1, z0, z1, lo, hi });
    }
    this.instances = new Float32Array(this.patches.length * 2);
    this.vbo = gl.createBuffer();
  }

  get half() { return this.span / 2; }

  contains(x, z, margin = 0) {
    return Math.abs(x - this.cx) < this.half - margin && Math.abs(z - this.cz) < this.half - margin;
  }

  // heightAt is the bilinear height at a world position, or null outside.
  heightAt(x, z) {
    const gx = (x - this.originX) / this.cell, gz = (this.originZ - z) / this.cell;
    if (gx < 0 || gz < 0 || gx > this.n - 1 || gz > this.n - 1) return null;
    const i = Math.min(Math.floor(gx), this.n - 2), j = Math.min(Math.floor(gz), this.n - 2);
    const kx = gx - i, kz = gz - j;
    const h = this.heights, n = this.n;
    const a = h[j * n + i], b = h[j * n + i + 1], c = h[(j + 1) * n + i], d = h[(j + 1) * n + i + 1];
    return (a * (1 - kx) + b * kx) * (1 - kz) + (c * (1 - kx) + d * kx) * kz;
  }

  destroy() {
    const { gl } = this;
    gl.deleteTexture(this.texH); gl.deleteTexture(this.texB); gl.deleteTexture(this.texM);
    gl.deleteBuffer(this.vbo);
  }
}

// ---------------------------------------------------------------- the view

export class World3D {
  constructor(el, { assetsBase = null, modelsBase = null, maxDPR = 1.5, relief = 2 } = {}) {
    if (!el) throw new Error('World3D: an element is required');
    this.container = el;
    if (getComputedStyle(el).position === 'static') el.style.position = 'relative';
    this.glCanvas = document.createElement('canvas');
    this.overlay = document.createElement('canvas');
    el.appendChild(this.glCanvas);
    el.appendChild(this.overlay);
    for (const c of [this.glCanvas, this.overlay]) {
      Object.assign(c.style, { position: 'absolute', left: '0', top: '0', width: '100%', height: '100%', display: 'block' });
    }
    this.glCanvas.style.zIndex = '0';
    Object.assign(this.overlay.style, { zIndex: '1', touchAction: 'none', cursor: 'grab', userSelect: 'none' });
    this.overlay.setAttribute('aria-label', 'World in 3D');
    this.ctx = this.overlay.getContext('2d');

    this.maxDPR = maxDPR;
    this.width = 0; this.height = 0; this.dpr = 1;
    this.relief = relief;
    this.water = WATER;
    this.radius = MAP_RADIUS;
    this.hOff = HEIGHT_OFFSET; this.hScale = HEIGHT_SCALE;
    // The camera: a target on the ground, a distance, a yaw (0 looks
    // north, clockwise) and a pitch down from the horizon.
    this.tx = 0; this.tz = 0; this.ty = WATER;
    this.distance = 16000; this.yaw = 0; this.pitch = 55 * Math.PI / 180;
    this.anim = null;
    this.sun = norm3([-0.45, 0.62, 0.38]);
    this.fogColour = [0.78, 0.82, 0.86];

    this.modelsBase = modelsBase == null ? new URL('../models/', import.meta.url).href : String(modelsBase).replace(/\/?$/, '/');
    this.models = null;
    this.zones = new Map();      // 'zx,zz' → {zx, zz, status, byModel: Map(name → Float32Array)}
    this.zoneInflight = 0;
    this.instancesDirty = true;
    this.visibleZoneKey = '';
    this.handle = null;
    this.unsubscribe = [];
    this.layers = [null, null, null];       // base, then LEVELS
    this.pending = [null, null, null];      // {cx, cz, promise}
    this.markers = [];
    this.filter = null;
    this.visible = [];
    this.hover = null;
    this.clickHandlers = []; this.hoverHandlers = []; this.viewHandlers = [];
    this.destroyed = false;
    this.hidden = false;
    this.dirty = true;
    this.frame = 0;
    this.error = null;
    this.time = 0;
    this.lastFrameAt = 0;

    // A decimated base on a touch device, where two million triangles are
    // a frame a second.
    this.baseStep = (navigator.maxTouchPoints > 1 && Math.min(screen.width, screen.height) < 900) ? 2 : 1;

    try {
      this._initGL();
    } catch (err) {
      this.error = err;
      console.warn(String(err));
    }
    this.glCanvas.addEventListener('webglcontextlost', this._onLost = e => { e.preventDefault(); });
    this.glCanvas.addEventListener('webglcontextrestored', this._onRestored = () => {
      try { this._initGL(); this._reupload(); this.invalidate(); } catch (err) { console.warn(String(err)); }
    });

    this.viewProj = new Float32Array(16);
    this.invViewProj = new Float32Array(16);
    this.camera = [0, 0, 0];

    this._bindPointer();
    this.ro = new ResizeObserver(() => this._resize());
    this.ro.observe(el);
    this._resize();
  }

  _initGL() {
    const gl = this.glCanvas.getContext('webgl2', { antialias: true, alpha: false, depth: true, powerPreference: 'high-performance' });
    if (!gl) throw new Error('World3D: no WebGL2');
    this.gl = gl;
    this.terrain = program(gl, TERRAIN_VERT, TERRAIN_FRAG);
    this.waterProg = program(gl, WATER_VERT, WATER_FRAG);
    this.sky = program(gl, SKY_VERT, SKY_FRAG);
    this.modelProg = program(gl, MODEL_VERT, MODEL_FRAG);
    if (this.models) this.models.destroy();
    this.models = new ModelStore(gl, this.modelsBase, () => { this.instancesDirty = true; this.invalidate(); });
    // The patch's indices: VERTS×VERTS vertices, two triangles a cell.
    const idx = new Uint16Array((VERTS - 1) * (VERTS - 1) * 6);
    let k = 0;
    for (let j = 0; j < VERTS - 1; j++) for (let i = 0; i < VERTS - 1; i++) {
      const a = j * VERTS + i, b = a + 1, c = a + VERTS, d = c + 1;
      idx[k++] = a; idx[k++] = c; idx[k++] = b;
      idx[k++] = b; idx[k++] = c; idx[k++] = d;
    }
    this.patchIndexCount = idx.length;
    this.vao = gl.createVertexArray();
    gl.bindVertexArray(this.vao);
    this.ibo = gl.createBuffer();
    gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, this.ibo);
    gl.bufferData(gl.ELEMENT_ARRAY_BUFFER, idx, gl.STATIC_DRAW);
    gl.bindVertexArray(null);
    this.emptyVao = gl.createVertexArray();
    gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
    this.aPatch = gl.getAttribLocation(this.terrain.p, 'aPatch');
  }

  // setWorld feeds the view from a World handle.
  setWorld(handle) {
    for (const off of this.unsubscribe) off();
    this.unsubscribe = [];
    for (const l of this.layers) if (l) l.destroy();
    this.layers = [null, null, null];
    this.pending = [null, null, null];
    this.zones.clear();
    this.instancesDirty = true;
    this.handle = handle;
    if (!handle) { this.invalidate(); return; }
    this.unsubscribe.push(handle.on('map', map => {
      const info = handle.info || {};
      this.water = info.water != null ? info.water : WATER;
      this.radius = info.radius || MAP_RADIUS;
      this.hOff = info.heightOffset || HEIGHT_OFFSET;
      this.hScale = info.heightScale || HEIGHT_SCALE;
      this.baseData = map;
      this._uploadBase();
      this._settleTarget();
      this.invalidate();
    }));
  }

  _uploadBase() {
    if (!this.gl || !this.baseData) return;
    const map = this.baseData;
    const n = map.size;
    const heights = new Float32Array(n * n);
    for (let i = 0; i < n * n; i++) heights[i] = map.heights[i] / this.hScale - this.hOff;
    if (this.layers[0]) this.layers[0].destroy();
    this.layers[0] = new Layer(this.gl, { n, cx: 0, cz: 0, span: 2 * this.radius, heights, biomes: map.biomes, mask: map.mask, step: this.baseStep });
  }

  _reupload() {
    const tiles = this.layers.slice(1).map(l => l && { n: l.n, cx: l.cx, cz: l.cz, span: l.span, heights: l.heights, biomes: l.biomes, mask: l.mask });
    this._uploadBase();
    tiles.forEach((t, i) => { if (t) this._setTile(i + 1, t); });
  }

  setMarkers(list) {
    const redraw = () => this.invalidate(false);
    this.markers = (list || []).map(m => {
      const out = Object.assign({}, m);
      out.priority = +m.priority || 0;
      out.color = m.color || '#ffffff';
      if (typeof m.img === 'string') {
        const entry = markerImage(m.img, redraw);
        out.imgEntry = entry; out.img = entry.img;
      } else if (m.img instanceof HTMLImageElement) {
        out.imgEntry = { img: m.img, failed: false };
      } else { out.img = null; out.imgEntry = null; }
      out.size = m.size || (out.img ? 22 : 8);
      return out;
    });
    this.hover = null;
    this.invalidate(false);
  }

  setFilter(fn) { this.filter = typeof fn === 'function' ? fn : null; this.invalidate(false); }
  onClick(fn) { this.clickHandlers.push(fn); return () => { this.clickHandlers = this.clickHandlers.filter(f => f !== fn); }; }
  onHover(fn) { this.hoverHandlers.push(fn); return () => { this.hoverHandlers = this.hoverHandlers.filter(f => f !== fn); }; }
  onView(fn) { this.viewHandlers.push(fn); return () => { this.viewHandlers = this.viewHandlers.filter(f => f !== fn); }; }

  // setRelief sets the vertical exaggeration about sea level.
  setRelief(k) {
    this.relief = Math.max(0.5, Math.min(6, +k || 1));
    this._settleTarget();
    this.invalidate();
    this._viewChanged();
  }

  // setHidden pauses drawing while the view is not shown.
  setHidden(hidden) {
    this.hidden = !!hidden;
    if (!hidden) { this._resize(); this.invalidate(); }
  }

  // view is the camera: target, distance, yaw in degrees, pitch in degrees,
  // and the metres across the view at the target, for parity with the map.
  get view() {
    return { cx: this.tx, cz: this.tz, distance: this.distance, yaw: this.yaw * 180 / Math.PI, pitch: this.pitch * 180 / Math.PI, span: this.span, zoom: this.zoom };
  }

  get aspect() { return Math.max(1e-3, this.width / Math.max(1, this.height)); }
  get span() { return 2 * this.distance * Math.tan(FOV / 2) * this.aspect; }
  // zoom as the map counts it, from the camera's distance: a tilted view
  // sees far past its target, so this is stricter than the span would say.
  get zoom() { return Math.max(1, 2 * MAP_RADIUS / (this.distance * 2.4)); }

  // lookAt moves the camera's target, and optionally its distance, yaw and
  // pitch (degrees), with a short flight unless animate is false.
  lookAt(x, z, { distance, yaw, pitch, zoom, span, animate = true } = {}) {
    if (zoom != null && distance == null) distance = this._distanceForZoom(zoom);
    if (span != null && distance == null) distance = span / (2 * Math.tan(FOV / 2) * this.aspect);
    const to = {
      tx: clampC(x), tz: clampC(z),
      distance: distance != null ? clampD(distance) : this.distance,
      yaw: yaw != null ? yaw * Math.PI / 180 : this.yaw,
      pitch: pitch != null ? clampP(pitch * Math.PI / 180) : this.pitch,
    };
    const reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (!animate || reduce || this.width === 0) {
      Object.assign(this, to);
      this._settleTarget();
      this.invalidate();
      this._viewChanged();
      return;
    }
    const from = { tx: this.tx, tz: this.tz, distance: this.distance, yaw: this.yaw, pitch: this.pitch };
    // Turn the short way round.
    while (to.yaw - from.yaw > Math.PI) to.yaw -= 2 * Math.PI;
    while (to.yaw - from.yaw < -Math.PI) to.yaw += 2 * Math.PI;
    const far = Math.hypot(to.tx - from.tx, to.tz - from.tz);
    const ms = Math.min(1400, 450 + far / 20 + Math.abs(Math.log(to.distance / from.distance)) * 180);
    this.anim = { from, to, t0: performance.now(), ms, far };
    this.invalidate();
  }

  _distanceForZoom(zoom) { return 2 * MAP_RADIUS / (Math.max(1, zoom) * 2.4); }

  // groundAt is the drawn height of the ground at x, z: the finest layer
  // that has it, exaggerated.
  groundAt(x, z) {
    for (let i = this.layers.length - 1; i >= 0; i--) {
      const l = this.layers[i];
      if (!l) continue;
      const h = l.heightAt(x, z);
      if (h != null) return this.water + (h - this.water) * this.relief;
    }
    return this.water;
  }

  // The site's own sampled height, exaggerated; the ground where there is
  // none.
  _markerY(m) {
    if (m.data && m.data.site && m.data.site.y != null) {
      const y = m.data.site.y;
      return this.water + (Math.max(y, this.water) - this.water) * this.relief;
    }
    return Math.max(this.water, this.groundAt(m.x, m.z));
  }

  _settleTarget() {
    this.ty = Math.max(this.water, this.groundAt(this.tx, this.tz));
  }

  // worldToScreen projects a ground position to CSS pixels; visible is false
  // behind the camera.
  worldToScreen(x, z, y) {
    if (y == null) y = this.groundAt(x, z);
    const m = this.viewProj;
    const cx = m[0] * x + m[4] * y + m[8] * z + m[12];
    const cy = m[1] * x + m[5] * y + m[9] * z + m[13];
    const cw = m[3] * x + m[7] * y + m[11] * z + m[15];
    if (cw <= 1e-6) return { x: 0, y: 0, visible: false, w: cw };
    return { x: (cx / cw * 0.5 + 0.5) * this.width, y: (0.5 - cy / cw * 0.5) * this.height, visible: true, w: cw };
  }

  // pickGround is the world position under a CSS pixel: the ray through it
  // against the ground, refined from the target's plane, or null when it
  // points at the sky.
  pickGround(px, py) {
    const ray = this._ray(px, py);
    if (!ray) return null;
    let y = this.ty;
    let hit = null;
    for (let k = 0; k < 6; k++) {
      if (Math.abs(ray.dir[1]) < 1e-6) return null;
      const t = (y - ray.origin[1]) / ray.dir[1];
      if (t <= 0) return null;
      hit = { x: ray.origin[0] + ray.dir[0] * t, z: ray.origin[2] + ray.dir[2] * t };
      const g = Math.max(this.water, this.groundAt(hit.x, hit.z));
      if (Math.abs(g - y) < 0.5) break;
      y = y + (g - y) * 0.7;
    }
    if (!hit || Math.hypot(hit.x, hit.z) > this.radius * 1.5) return null;
    return hit;
  }

  _ray(px, py) {
    if (!this.width || !this.height) return null;
    const ndcX = px / this.width * 2 - 1, ndcY = 1 - py / this.height * 2;
    const inv = this.invViewProj;
    const un = (x, y, z) => {
      const X = inv[0] * x + inv[4] * y + inv[8] * z + inv[12];
      const Y = inv[1] * x + inv[5] * y + inv[9] * z + inv[13];
      const Z = inv[2] * x + inv[6] * y + inv[10] * z + inv[14];
      const W = inv[3] * x + inv[7] * y + inv[11] * z + inv[15];
      return [X / W, Y / W, Z / W];
    };
    const near = un(ndcX, ndcY, -1), far = un(ndcX, ndcY, 1);
    const dir = norm3([far[0] - near[0], far[1] - near[1], far[2] - near[2]]);
    return { origin: this.camera.slice(), dir };
  }

  invalidate(gl = true) {
    if (gl) this.dirty = true;
    this.overlayDirty = true;
    if (!this.frame && !this.destroyed && !this.hidden) this.frame = requestAnimationFrame(t => this._frame(t));
  }

  _resize() {
    const r = this.container.getBoundingClientRect();
    const w = Math.max(0, Math.round(r.width)), h = Math.max(0, Math.round(r.height));
    const dpr = Math.min(window.devicePixelRatio || 1, this.maxDPR);
    if (w === this.width && h === this.height && dpr === this.dpr) return;
    this.width = w; this.height = h; this.dpr = dpr;
    this.overlay.width = Math.max(1, Math.round(w * dpr));
    this.overlay.height = Math.max(1, Math.round(h * dpr));
    this.glCanvas.width = Math.max(1, Math.round(w * dpr));
    this.glCanvas.height = Math.max(1, Math.round(h * dpr));
    this.invalidate();
    this._viewChanged();
  }

  _viewChanged() {
    if (!this.viewHandlers.length) return;
    const v = this.view;
    for (const fn of this.viewHandlers) { try { fn(v); } catch (err) { console.error(err); } }
  }

  // ------------------------------------------------------------ frames

  _frame(now) {
    this.frame = 0;
    if (this.destroyed || this.hidden || !this.width || !this.height) return;
    let again = false;
    if (this.anim) {
      const a = this.anim;
      let t = Math.min(1, (now - a.t0) / a.ms);
      const e = t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
      this.tx = a.from.tx + (a.to.tx - a.from.tx) * e;
      this.tz = a.from.tz + (a.to.tz - a.from.tz) * e;
      // Rise over a long flight, so the ground slides under rather than
      // tearing past.
      const lift = a.far > 2000 ? Math.sin(Math.PI * t) * Math.min(a.far * 0.6, 12000) : 0;
      this.distance = Math.exp(Math.log(a.from.distance) + (Math.log(a.to.distance) - Math.log(a.from.distance)) * e) + lift;
      this.yaw = a.from.yaw + (a.to.yaw - a.from.yaw) * e;
      this.pitch = a.from.pitch + (a.to.pitch - a.from.pitch) * e;
      this._settleTarget();
      if (t >= 1) { this.anim = null; this.distance = a.to.distance; }
      else again = true;
      this.dirty = true;
      this._viewChanged();
    }
    if (this.dirty) {
      this.dirty = false;
      this._updateCamera();
      this._wantTiles();
      this._wantZones();
      if (this.gl) {
        try { this._draw(now); } catch (err) { console.warn('world3d draw:', err); }
      }
      this.overlayDirty = true;
    }
    if (this.overlayDirty) {
      this.overlayDirty = false;
      this._drawOverlay();
    }
    // Water moves while something close is in view.
    if (!again && this.layers[2] && this.distance < 1500 && !this._reducedMotion()) again = true;
    if (again) this.frame = requestAnimationFrame(t => this._frame(t));
  }

  _reducedMotion() {
    return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  }

  _updateCamera() {
    const f = [Math.sin(this.yaw), 0, Math.cos(this.yaw)];
    const cp = Math.cos(this.pitch), sp = Math.sin(this.pitch);
    const eye = [this.tx - f[0] * this.distance * cp, this.ty + this.distance * sp, this.tz - f[2] * this.distance * cp];
    // Never under the ground.
    const g = this.groundAt(eye[0], eye[2]);
    const minY = Math.max(g, this.water) + Math.max(3, this.distance * 0.02);
    if (eye[1] < minY) eye[1] = minY;
    this.camera = eye;
    const near = Math.max(0.5, this.distance * 0.004), far = Math.max(40000, this.distance * 4 + 30000);
    const proj = perspective(new Float32Array(16), FOV, this.aspect, near, far);
    // The world's frame is Unity's, x east, y up, z north, which is
    // left-handed; the matrices work in a right-handed one with z south,
    // and the flip on the way in keeps east on the right.
    const view = lookAt(new Float32Array(16), [eye[0], eye[1], -eye[2]], [this.tx, this.ty, -this.tz], [0, 1, 0]);
    multiply(this.viewProj, proj, view);
    multiply(this.viewProj, this.viewProj, FLIP_Z);
    invert(this.invViewProj, this.viewProj);
    this.planes = frustumPlanes(this.viewProj);
    // Haze thickens as the camera comes down: a tenth of the sky at the
    // world's width, most of it past four times the camera's distance.
    this.fogK = 1 / (this.distance * 9 + 14000);
  }

  _draw(now) {
    const { gl } = this;
    const w = this.glCanvas.width, h = this.glCanvas.height;
    gl.viewport(0, 0, w, h);
    gl.disable(gl.BLEND);
    gl.enable(gl.DEPTH_TEST);
    gl.depthFunc(gl.LEQUAL);
    gl.clearColor(this.fogColour[0], this.fogColour[1], this.fogColour[2], 1);
    gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
    this.time = now / 1000;

    // The sky, behind everything.
    gl.useProgram(this.sky.p);
    gl.bindVertexArray(this.emptyVao);
    gl.depthMask(false);
    gl.uniformMatrix4fv(this.sky.u.uInvViewProj, false, this.invViewProj);
    gl.uniform3fv(this.sky.u.uCamera, this.camera);
    gl.uniform3fv(this.sky.u.uSun, this.sun);
    gl.uniform3fv(this.sky.u.uFog, this.fogColour);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
    gl.depthMask(true);

    const base = this.layers[0];
    if (!base) return;

    // The land, coarse to fine.
    gl.useProgram(this.terrain.p);
    gl.bindVertexArray(this.vao);
    gl.enable(gl.CULL_FACE);
    gl.cullFace(gl.BACK);
    gl.frontFace(gl.CCW);
    const u = this.terrain.u;
    gl.uniformMatrix4fv(u.uViewProj, false, this.viewProj);
    gl.uniform1f(u.uWater, this.water);
    gl.uniform1f(u.uRelief, this.relief);
    gl.uniform3fv(u.uCamera, this.camera);
    gl.uniform3fv(u.uSun, this.sun);
    gl.uniform3fv(u.uFog, this.fogColour);
    gl.uniform1f(u.uFogK, this.fogK);
    gl.uniform1f(u.uDetail, this.layers[2] ? 1 : 0.4);
    gl.uniform1i(u.uH, 0); gl.uniform1i(u.uB, 1); gl.uniform1i(u.uM, 2); gl.uniform1i(u.uParentH, 3);
    for (let i = 0; i < this.layers.length; i++) {
      const l = this.layers[i];
      if (!l) continue;
      const parent = i > 0 ? this._parentOf(i) : null;
      const child = this._childOf(i);
      this._drawLayer(l, parent, child);
    }
    gl.disable(gl.CULL_FACE);

    // What stands on the land.
    this._drawModels();

    // The sea, blended over the land.
    gl.useProgram(this.waterProg.p);
    gl.bindVertexArray(this.emptyVao);
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
    gl.depthMask(false);
    const wu = this.waterProg.u;
    gl.uniformMatrix4fv(wu.uViewProj, false, this.viewProj);
    gl.uniform1f(wu.uWater, this.water);
    gl.uniform1f(wu.uRadius, this.radius);
    gl.uniform1f(wu.uRelief, this.relief);
    gl.uniform3fv(wu.uCamera, this.camera);
    gl.uniform3fv(wu.uSun, this.sun);
    gl.uniform3fv(wu.uFog, this.fogColour);
    gl.uniform1f(wu.uFogK, this.fogK);
    gl.uniform1f(wu.uTime, this._reducedMotion() ? 0 : this.time);
    const present = this.layers.filter(Boolean);
    gl.uniform1i(wu.uLayers, present.length);
    present.forEach((l, i) => {
      gl.activeTexture(gl.TEXTURE0 + i);
      gl.bindTexture(gl.TEXTURE_2D, l.texH);
      gl.uniform1i(wu['uH' + i], i);
      gl.uniform1i(wu['uN' + i], l.n);
      gl.uniform2f(wu['uO' + i], l.originX, l.originZ);
      gl.uniform1f(wu['uC' + i], l.cell);
      if (i > 0) { gl.uniform2f(wu['uCentre' + i], l.cx, l.cz); gl.uniform1f(wu['uHalf' + i], l.half); }
    });
    gl.drawArrays(gl.TRIANGLES, 0, 3);
    gl.depthMask(true);
    gl.disable(gl.BLEND);
    gl.bindVertexArray(null);
  }

  _parentOf(i) {
    for (let j = i - 1; j >= 0; j--) if (this.layers[j]) return this.layers[j];
    return null;
  }

  _childOf(i) {
    for (let j = i + 1; j < this.layers.length; j++) if (this.layers[j]) return this.layers[j];
    return null;
  }

  _drawLayer(l, parent, child) {
    const { gl } = this;
    const u = this.terrain.u;
    gl.activeTexture(gl.TEXTURE0); gl.bindTexture(gl.TEXTURE_2D, l.texH);
    gl.activeTexture(gl.TEXTURE1); gl.bindTexture(gl.TEXTURE_2D, l.texB);
    gl.activeTexture(gl.TEXTURE2); gl.bindTexture(gl.TEXTURE_2D, l.texM);
    gl.uniform1i(u.uN, l.n);
    gl.uniform1i(u.uStep, l.step);
    gl.uniform2f(u.uOrigin, l.originX, l.originZ);
    gl.uniform1f(u.uCell, l.cell);
    gl.uniform1f(u.uSkirt, Math.max(8, l.cell * 2.5) * this.relief);
    gl.uniform2f(u.uCentre, l.cx, l.cz);
    gl.uniform1f(u.uHalf, l.half);
    gl.uniform1i(u.uHasParent, parent ? 1 : 0);
    if (parent) {
      gl.activeTexture(gl.TEXTURE3); gl.bindTexture(gl.TEXTURE_2D, parent.texH);
      gl.uniform1i(u.uParentN, parent.n);
      gl.uniform2f(u.uParentOrigin, parent.originX, parent.originZ);
      gl.uniform1f(u.uParentCell, parent.cell);
    }
    gl.uniform1i(u.uHasChild, child ? 1 : 0);
    if (child) {
      gl.uniform2f(u.uChildCentre, child.cx, child.cz);
      gl.uniform1f(u.uChildHalf, child.half);
    }
    // The patches in view, and not wholly under the child.
    let count = 0;
    const inst = l.instances;
    const skirt = Math.max(8, l.cell * 2.5) * this.relief + (child ? 80 * this.relief : 0);
    for (const p of l.patches) {
      if (child && p.x0 > child.cx - child.half && p.x1 < child.cx + child.half && p.z0 > child.cz - child.half && p.z1 < child.cz + child.half) continue;
      const y0 = this.water + (p.lo - this.water) * this.relief - skirt, y1 = this.water + (p.hi - this.water) * this.relief + 1;
      if (!boxVisible(this.planes, p.x0 - l.cell, y0, p.z0 - l.cell, p.x1 + l.cell, y1, p.z1 + l.cell)) continue;
      inst[2 * count] = p.i0; inst[2 * count + 1] = p.j0;
      count++;
    }
    if (!count) return;
    gl.bindBuffer(gl.ARRAY_BUFFER, l.vbo);
    gl.bufferData(gl.ARRAY_BUFFER, inst.subarray(0, 2 * count), gl.DYNAMIC_DRAW);
    gl.enableVertexAttribArray(this.aPatch);
    gl.vertexAttribPointer(this.aPatch, 2, gl.FLOAT, false, 0, 0);
    gl.vertexAttribDivisor(this.aPatch, 1);
    gl.drawElementsInstanced(gl.TRIANGLES, this.patchIndexCount, gl.UNSIGNED_SHORT, 0, count);
  }

  // ------------------------------------------------------------ tiles

  // _wantTiles asks the engine for the close-up levels the camera has come
  // near, and recentres the ones it has drifted from.
  _wantTiles() {
    if (!this.handle || !this.layers[0]) return;
    for (let i = 0; i < LEVELS.length; i++) {
      const lv = LEVELS[i], slot = i + 1;
      const have = this.layers[slot], pend = this.pending[slot];
      if (this.distance > lv.show) {
        if (have && this.distance > lv.show * 1.6) { have.destroy(); this.layers[slot] = null; this.dirty = true; }
        continue;
      }
      const cx = this.tx, cz = this.tz;
      const drift = l => Math.max(Math.abs(l.cx - cx), Math.abs(l.cz - cz)) / lv.span;
      if (have && drift(have) < lv.recentre) continue;
      if (pend && drift(pend) < lv.recentre * 0.6) continue;
      const req = { cx, cz, span: lv.span };
      this.pending[slot] = req;
      this.handle.tile(cx, cz, lv.span, lv.n).then(t => {
        if (this.destroyed || this.pending[slot] !== req) return;
        this.pending[slot] = null;
        if (this.distance > lv.show * 1.6) return;
        this._setTile(slot, t);
        this._settleTarget();
        this.invalidate();
      }).catch(err => {
        if (this.pending[slot] === req) this.pending[slot] = null;
        console.warn('world3d tile:', err && err.message || err);
      });
    }
  }

  _setTile(slot, t) {
    if (!this.gl) return;
    if (this.layers[slot]) this.layers[slot].destroy();
    this.layers[slot] = new Layer(this.gl, { n: t.n, cx: t.cx, cz: t.cz, span: t.span, heights: t.heights, biomes: t.biomes, mask: t.mask, step: 1 });
  }

  // ------------------------------------------------------------ zones

  // _wantZones asks for the zones around the target the camera has come
  // close enough to see standing things in, nearest first, a few at a
  // time, and forgets the ones it has left behind.
  _wantZones() {
    if (!this.handle || !this.layers[0] || !this.models) return;
    if (this.distance > SCENE_SHOW) {
      if (this.zones.size) { this.zones.clear(); this.instancesDirty = true; }
      return;
    }
    const R = ZONE_RADIUS(this.distance);
    const zx0 = Math.floor((this.tx + ZONE / 2) / ZONE), zz0 = Math.floor((this.tz + ZONE / 2) / ZONE);
    const reach = Math.ceil(R / ZONE);
    // Forget what is far.
    for (const [key, z] of this.zones) {
      const d = Math.hypot(z.zx * ZONE - this.tx, z.zz * ZONE - this.tz);
      if (d > R * 1.8 + ZONE && z.status !== 'loading') { this.zones.delete(key); this.instancesDirty = true; }
    }
    const wanted = [];
    for (let zz = zz0 - reach; zz <= zz0 + reach; zz++) for (let zx = zx0 - reach; zx <= zx0 + reach; zx++) {
      const d = Math.hypot(zx * ZONE - this.tx, zz * ZONE - this.tz);
      if (d > R + ZONE * 0.7) continue;
      if (Math.hypot(zx * ZONE, zz * ZONE) > WORLD_RADIUS + ZONE) continue;
      const key = zx + ',' + zz;
      if (this.zones.has(key)) continue;
      wanted.push({ zx, zz, d, key });
    }
    wanted.sort((a, b) => a.d - b.d);
    for (const w of wanted) {
      if (this.zoneInflight >= ZONE_INFLIGHT) break;
      const zone = { zx: w.zx, zz: w.zz, status: 'loading', byModel: new Map(), count: 0 };
      this.zones.set(w.key, zone);
      this.zoneInflight++;
      const handle = this.handle;
      Promise.all([handle.zone(w.zx, w.zz), this.models.ready]).then(([z]) => {
        this.zoneInflight--;
        if (this.destroyed || this.handle !== handle || this.zones.get(w.key) !== zone) return;
        this._fillZone(zone, z);
        this.instancesDirty = true;
        this.invalidate();
      }).catch(err => {
        this.zoneInflight--;
        zone.status = 'failed';
        console.warn('world3d zone:', err && err.message || err);
        this.invalidate();
      });
    }
  }

  // _fillZone sorts a zone's places by model, keeping the ones the pack
  // can draw: x, y, z, scale, then the quaternion, per instance.
  _fillZone(zone, z) {
    const counts = new Map();
    const n = z.ids.length;
    for (let i = 0; i < n; i++) {
      const name = z.names[z.ids[i]];
      if (!this.models.has(name)) continue;
      counts.set(name, (counts.get(name) || 0) + 1);
    }
    const fill = new Map();
    for (const [name, c] of counts) { zone.byModel.set(name, new Float32Array(c * 8)); fill.set(name, 0); }
    for (let i = 0; i < n; i++) {
      const name = z.names[z.ids[i]];
      const arr = zone.byModel.get(name);
      if (!arr) continue;
      const k = fill.get(name);
      const o = i * 8;
      arr[k] = z.data[o]; arr[k + 1] = z.data[o + 1]; arr[k + 2] = z.data[o + 2]; arr[k + 3] = z.data[o + 7];
      arr[k + 4] = z.data[o + 3]; arr[k + 5] = z.data[o + 4]; arr[k + 6] = z.data[o + 5]; arr[k + 7] = z.data[o + 6];
      fill.set(name, k + 8);
      zone.count++;
    }
    zone.status = 'ready';
    for (const name of zone.byModel.keys()) this.models.get(name);
  }

  // _visibleZones is the ready zones in the frustum, as a list and a key.
  _visibleZones() {
    const out = [];
    let key = '';
    const y0 = this.water - 60 * this.relief, y1 = this.water + 500 * this.relief;
    for (const z of this.zones.values()) {
      if (z.status !== 'ready' || !z.count) continue;
      const x0 = z.zx * ZONE - ZONE / 2 - 20, x1 = x0 + ZONE + 40;
      const z0 = z.zz * ZONE - ZONE / 2 - 20, z1 = z0 + ZONE + 40;
      if (!boxVisible(this.planes, x0, y0, z0, x1, y1, z1)) continue;
      out.push(z);
      key += z.zx + ',' + z.zz + ';';
    }
    return { list: out, key };
  }

  _drawModels() {
    const { gl } = this;
    if (!this.models || !this.zones.size) return;
    const vis = this._visibleZones();
    if (!vis.list.length) return;
    if (this.instancesDirty || vis.key !== this.visibleZoneKey) {
      this.visibleZoneKey = vis.key;
      this.instancesDirty = false;
      // Merge the visible zones' instances per model and upload them.
      const merged = new Map();
      for (const z of vis.list) for (const [name, arr] of z.byModel) {
        let list = merged.get(name);
        if (!list) merged.set(name, list = []);
        list.push(arr);
      }
      for (const m of this.models.models.values()) if (m && m.vao) m.instances = 0;
      for (const [name, arrays] of merged) {
        const m = this.models.get(name);
        if (!m) continue;
        let total = 0;
        for (const a of arrays) total += a.length;
        const all = new Float32Array(total);
        let at = 0;
        for (const a of arrays) { all.set(a, at); at += a.length; }
        gl.bindBuffer(gl.ARRAY_BUFFER, m.ibo);
        gl.bufferData(gl.ARRAY_BUFFER, all, gl.DYNAMIC_DRAW);
        m.instances = total / 8;
      }
    }
    gl.useProgram(this.modelProg.p);
    const u = this.modelProg.u;
    gl.uniformMatrix4fv(u.uViewProj, false, this.viewProj);
    gl.uniform1f(u.uWater, this.water);
    gl.uniform1f(u.uRelief, this.relief);
    gl.uniform3fv(u.uCamera, this.camera);
    gl.uniform3fv(u.uSun, this.sun);
    gl.uniform3fv(u.uFog, this.fogColour);
    gl.uniform1f(u.uFogK, this.fogK);
    gl.uniform1i(u.uTex, 0);
    gl.activeTexture(gl.TEXTURE0);
    gl.disable(gl.CULL_FACE);
    let coverage = false;
    for (const m of this.models.models.values()) {
      if (!m || !m.vao || !m.instances) continue;
      gl.bindVertexArray(m.vao);
      for (const r of m.runs) {
        const mat = r.material;
        if (mat.cutout !== coverage) {
          coverage = mat.cutout;
          if (coverage) gl.enable(gl.SAMPLE_ALPHA_TO_COVERAGE); else gl.disable(gl.SAMPLE_ALPHA_TO_COVERAGE);
        }
        if (mat.tex) {
          gl.bindTexture(gl.TEXTURE_2D, mat.tex.tex);
          gl.uniform1i(u.uHasTex, 1);
          gl.uniform4f(u.uColor, mat.color[0], mat.color[1], mat.color[2], mat.color[3]);
        } else {
          gl.uniform1i(u.uHasTex, 0);
          const c = mat.name && this.models.materials[mat.name] ? mat.color : mat.tone;
          gl.uniform4f(u.uColor, c[0], c[1], c[2], 1);
        }
        gl.uniform1i(u.uCutout, mat.cutout ? 1 : 0);
        gl.uniform1f(u.uCutoff, mat.cutoff);
        gl.uniform2f(u.uScale, mat.scale[0], mat.scale[1]);
        gl.uniform2f(u.uOffset, mat.offset[0], mat.offset[1]);
        gl.drawElementsInstanced(gl.TRIANGLES, r.count, gl.UNSIGNED_INT, r.first * 4, m.instances);
      }
    }
    if (coverage) gl.disable(gl.SAMPLE_ALPHA_TO_COVERAGE);
    gl.bindVertexArray(null);
  }

  // ------------------------------------------------------------ overlay

  _drawOverlay() {
    const ctx = this.ctx;
    const W = this.width, H = this.height;
    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    if (this.error) {
      ctx.fillStyle = '#e8d8b0';
      ctx.font = '14px system-ui, sans-serif';
      ctx.textAlign = 'center';
      ctx.fillText('The 3D view needs WebGL2: ' + (this.error.message || this.error), W / 2, H / 2);
      return;
    }
    if (!this.layers[0]) return;
    const margin = 40;
    const visible = [];
    const cam = this.camera;
    const reach = this.distance * 3.5 + 1500;
    for (const m of this.markers) {
      if (this.filter && !this.filter(m)) continue;
      const y = this._markerY(m);
      const d = Math.hypot(m.x - cam[0], y - cam[1], m.z - cam[2]);
      if (d > reach) continue;
      const p = this.worldToScreen(m.x, m.z, y);
      if (!p.visible) continue;
      if (p.x < -margin || p.y < -margin || p.x > W + margin || p.y > H + margin) continue;
      // Nearer markers larger, within reason.
      const scale = Math.max(0.5, Math.min(1.25, (this.distance * 1.1) / d));
      const fade = 1 - Math.max(0, (d - this.distance * 2) / (reach - this.distance * 2));
      visible.push({ m, sx: p.x, sy: p.y - m.size * scale / 2, r: m.size * scale / 2, d, alpha: Math.max(0.25, fade) });
    }
    let drawn = visible;
    if (visible.length > DECLUTTER_ABOVE) drawn = declutter(visible);
    this.visible = drawn;
    // Far first, so the near ones end on top; the priorities within a
    // distance band.
    drawn.sort((a, b) => b.d - a.d || a.m.priority - b.m.priority);
    let hovered = null;
    for (const v of drawn) {
      if (this.hover && v.m === this.hover.m) { hovered = v; continue; }
      ctx.globalAlpha = v.alpha;
      this._pin(ctx, v);
      drawMarkerDisc(ctx, v, false);
    }
    ctx.globalAlpha = 1;
    if (hovered) {
      this._pin(ctx, hovered);
      drawMarkerDisc(ctx, hovered, true);
      drawMarkerLabel(ctx, hovered, this.width);
    }
  }

  // A short stem from the disc to the ground, so a marker reads as
  // standing where it is.
  _pin(ctx, v) {
    const r = v.r;
    ctx.beginPath();
    ctx.moveTo(v.sx, v.sy + r);
    ctx.lineTo(v.sx, v.sy + r + Math.max(4, r * 0.6));
    ctx.lineWidth = 1.5;
    ctx.strokeStyle = 'rgba(0,0,0,0.6)';
    ctx.stroke();
    ctx.beginPath();
    ctx.arc(v.sx, v.sy + r + Math.max(4, r * 0.6), 1.5, 0, Math.PI * 2);
    ctx.fillStyle = v.m.color;
    ctx.fill();
  }

  markerAt(px, py) {
    let best = null, bestD = Infinity;
    for (const v of this.visible) {
      const d = Math.hypot(v.sx - px, v.sy - py);
      const reach = Math.max(v.r, 5) + 2;
      if (d <= reach && d < bestD) { best = v; bestD = d; }
    }
    return best;
  }

  // ------------------------------------------------------------ pointer

  _bindPointer() {
    const el = this.overlay;
    const pointers = new Map();
    let drag = null;     // {x, y, sx, sy, moved, mode: 'pan'|'orbit', ground}
    let pinch = null;    // {d, angle, midY, distance, yaw, pitch}
    let clickTimer = 0;
    const pos = e => { const r = el.getBoundingClientRect(); return { x: e.clientX - r.left, y: e.clientY - r.top }; };
    this._listeners = [];
    const on = (type, fn, opts) => { el.addEventListener(type, fn, opts); this._listeners.push([type, fn, opts]); };

    on('pointerdown', e => {
      if (e.pointerType === 'mouse' && e.button !== 0 && e.button !== 2 && e.button !== 1) return;
      el.setPointerCapture(e.pointerId);
      const p = pos(e);
      pointers.set(e.pointerId, p);
      this.anim = null;
      if (pointers.size === 1) {
        const orbit = e.button === 2 || e.button === 1 || e.shiftKey || e.ctrlKey;
        drag = { x: p.x, y: p.y, sx: p.x, sy: p.y, moved: false, mode: orbit ? 'orbit' : 'pan', ground: this.pickGround(p.x, p.y) };
        el.style.cursor = orbit ? 'move' : 'grabbing';
      } else if (pointers.size === 2) {
        const [a, b] = Array.from(pointers.values());
        pinch = { d: Math.hypot(a.x - b.x, a.y - b.y), angle: Math.atan2(b.y - a.y, b.x - a.x), midY: (a.y + b.y) / 2, distance: this.distance, yaw: this.yaw, pitch: this.pitch };
        drag = null;
      }
    });

    on('pointermove', e => {
      const p = pos(e);
      if (pointers.has(e.pointerId)) pointers.set(e.pointerId, p);
      if (pinch && pointers.size >= 2) {
        const [a, b] = Array.from(pointers.values());
        const d = Math.hypot(a.x - b.x, a.y - b.y);
        const angle = Math.atan2(b.y - a.y, b.x - a.x);
        const midY = (a.y + b.y) / 2;
        this.distance = clampD(pinch.distance * pinch.d / Math.max(1, d));
        let da = angle - pinch.angle;
        while (da > Math.PI) da -= 2 * Math.PI;
        while (da < -Math.PI) da += 2 * Math.PI;
        this.yaw = pinch.yaw - da;
        this.pitch = clampP(pinch.pitch + (midY - pinch.midY) * 0.004);
        this.invalidate();
        this._viewChanged();
        return;
      }
      if (drag && pointers.has(e.pointerId)) {
        const dx = p.x - drag.x, dy = p.y - drag.y;
        if (!drag.moved && Math.hypot(p.x - drag.sx, p.y - drag.sy) > CLICK_SLOP) drag.moved = true;
        if (drag.moved) {
          if (drag.mode === 'orbit') {
            this.yaw += dx * 0.005;
            this.pitch = clampP(this.pitch + dy * 0.005);
          } else {
            // Keep the ground point that was grabbed under the pointer.
            const now = this._groundOnPlane(p.x, p.y, drag.ground ? this.groundAt(drag.ground.x, drag.ground.z) : this.ty);
            const was = drag.ground || this._groundOnPlane(drag.x, drag.y, this.ty);
            if (now && was) {
              this.tx = clampC(this.tx + (was.x - now.x));
              this.tz = clampC(this.tz + (was.z - now.z));
            }
          }
          drag.x = p.x; drag.y = p.y;
          this._settleTarget();
          this.invalidate();
          this._viewChanged();
        }
        return;
      }
      const hit = this.markerAt(p.x, p.y);
      if ((hit && hit.m) !== (this.hover && this.hover.m)) {
        this.hover = hit;
        el.style.cursor = hit ? 'pointer' : 'grab';
        this.invalidate(false);
        for (const fn of this.hoverHandlers) { try { fn(hit ? hit.m : null); } catch (err) { console.error(err); } }
      }
    });

    const end = e => {
      const p = pos(e);
      const wasDrag = drag;
      pointers.delete(e.pointerId);
      if (pointers.size < 2) pinch = null;
      if (pointers.size === 0) {
        drag = null;
        el.style.cursor = this.hover ? 'pointer' : 'grab';
        if (wasDrag && !wasDrag.moved && e.type === 'pointerup' && e.button === 0) {
          const hit = this.markerAt(p.x, p.y);
          const g = hit ? { x: hit.m.x, z: hit.m.z } : this.pickGround(p.x, p.y);
          if (!g) return;
          const ev = { x: g.x, z: g.z, marker: hit ? hit.m : null, screen: p };
          clearTimeout(clickTimer);
          clickTimer = setTimeout(() => {
            clickTimer = 0;
            for (const fn of this.clickHandlers) { try { fn(ev); } catch (err) { console.error(err); } }
          }, DBLCLICK_WAIT);
        }
      } else if (pointers.size === 1) {
        const [q] = Array.from(pointers.values());
        drag = { x: q.x, y: q.y, sx: q.x, sy: q.y, moved: true, mode: 'pan', ground: this.pickGround(q.x, q.y) };
      }
    };
    on('pointerup', end);
    on('pointercancel', end);
    on('lostpointercapture', e => { if (pointers.has(e.pointerId)) end(e); });

    on('dblclick', e => {
      e.preventDefault();
      clearTimeout(clickTimer);
      clickTimer = 0;
      const p = pos(e);
      const g = this.pickGround(p.x, p.y);
      if (g) this.lookAt(g.x, g.z, { distance: this.distance / 2 });
    });

    on('wheel', e => {
      e.preventDefault();
      const p = pos(e);
      const k = e.deltaMode === 1 ? 20 : e.deltaMode === 2 ? 400 : 1;
      const factor = Math.exp(e.deltaY * k * 0.0015);
      const g = this.pickGround(p.x, p.y);
      const before = this.distance;
      this.distance = clampD(this.distance * factor);
      const ratio = this.distance / before;
      if (g && ratio < 1) {
        // Zoom towards what is under the pointer.
        this.tx = clampC(this.tx + (g.x - this.tx) * (1 - ratio));
        this.tz = clampC(this.tz + (g.z - this.tz) * (1 - ratio));
      }
      this.anim = null;
      this._settleTarget();
      this.invalidate();
      this._viewChanged();
    }, { passive: false });

    on('pointerleave', () => {
      if (this.hover) {
        this.hover = null;
        this.invalidate(false);
        for (const fn of this.hoverHandlers) { try { fn(null); } catch (err) { console.error(err); } }
      }
    });
    on('contextmenu', e => e.preventDefault());

    on('keydown', e => {
      const step = this.span * 0.08;
      const f = [Math.sin(this.yaw), Math.cos(this.yaw)], r = [Math.cos(this.yaw), -Math.sin(this.yaw)];
      let used = true;
      switch (e.key) {
        case 'ArrowUp': this.tx += f[0] * step; this.tz += f[1] * step; break;
        case 'ArrowDown': this.tx -= f[0] * step; this.tz -= f[1] * step; break;
        case 'ArrowLeft': if (e.shiftKey) this.yaw -= 0.1; else { this.tx -= r[0] * step; this.tz -= r[1] * step; } break;
        case 'ArrowRight': if (e.shiftKey) this.yaw += 0.1; else { this.tx += r[0] * step; this.tz += r[1] * step; } break;
        case '+': case '=': this.distance = clampD(this.distance / 1.3); break;
        case '-': case '_': this.distance = clampD(this.distance * 1.3); break;
        default: used = false;
      }
      if (!used) return;
      e.preventDefault();
      this.tx = clampC(this.tx); this.tz = clampC(this.tz);
      this._settleTarget();
      this.invalidate();
      this._viewChanged();
    });
    el.tabIndex = 0;
  }

  _groundOnPlane(px, py, y) {
    const ray = this._ray(px, py);
    if (!ray || Math.abs(ray.dir[1]) < 1e-6) return null;
    const t = (y - ray.origin[1]) / ray.dir[1];
    if (t <= 0) return null;
    return { x: ray.origin[0] + ray.dir[0] * t, z: ray.origin[2] + ray.dir[2] * t };
  }

  destroy() {
    if (this.destroyed) return;
    this.destroyed = true;
    if (this.frame) cancelAnimationFrame(this.frame);
    this.ro.disconnect();
    for (const off of this.unsubscribe) off();
    for (const [type, fn, opts] of this._listeners || []) this.overlay.removeEventListener(type, fn, opts);
    this.glCanvas.removeEventListener('webglcontextlost', this._onLost);
    this.glCanvas.removeEventListener('webglcontextrestored', this._onRestored);
    for (const l of this.layers) if (l) l.destroy();
    if (this.models) this.models.destroy();
    if (this.gl) {
      const gl = this.gl;
      gl.deleteProgram(this.terrain.p); gl.deleteProgram(this.waterProg.p); gl.deleteProgram(this.sky.p); gl.deleteProgram(this.modelProg.p);
      gl.deleteBuffer(this.ibo);
      const lose = gl.getExtension('WEBGL_lose_context');
      if (lose) lose.loseContext();
    }
    this.overlay.remove();
    this.glCanvas.remove();
  }
}

function norm3(v) { const l = Math.hypot(v[0], v[1], v[2]) || 1; return [v[0] / l, v[1] / l, v[2] / l]; }
function clampC(v) { return Math.max(-MAP_RADIUS, Math.min(MAP_RADIUS, +v || 0)); }
function clampD(v) { return Math.max(MIN_DIST, Math.min(MAX_DIST, +v || MIN_DIST)); }
function clampP(v) { return Math.max(MIN_PITCH, Math.min(MAX_PITCH, v)); }

export default World3D;
