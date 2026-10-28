// The map: the world painted in WebGL2 with the game's own map shader, and
// round site markers drawn over it.
//
//   import { WorldMap } from './worldmap.js';
//   const map = new WorldMap(container, { assetsBase });   // or a canvas
//   map.setWorld(World.open(seed));                        // feeds info, map, sectors as they arrive
//   map.setMarkers([{x, z, img, size, kind, color, label, priority, data}]);
//   map.onClick(({x, z, marker}) => ...);
//   map.centerOn(x, z, 8);
//
// The painter is valheim-world's: one full-screen triangle whose fragment
// shader reads the raster the engine streamed — height, biome and mask
// planes, row zero north, twelve or twenty-four metres a texel — and paints
// each pixel the way the game's Custom/mapshader does, with the textures
// under assetsBase + 'map/' (background, water, forest, mountain, clouds,
// lava, space) and the constants in minimap.json. Without those textures a
// procedural stand-in draws the same data. The alt-biome sector plane, when
// it arrives, is outlined over the land.
//
// Over the GL canvas sits a 2D canvas for the markers: images clipped round
// with a two-pixel ring in the marker's colour, dots where there is no
// image, decluttered by priority when more than four hundred are in view,
// the hovered one lifted with its label, and the sector names written out
// past zoom 3. Pan by dragging, zoom with the wheel, a pinch or a
// double-click; a click that did not drag reports the world position and
// the marker under it.

const MAP_RADIUS = 12288;
const WORLD_RADIUS = 10500;
const HEIGHT_OFFSET = 512, HEIGHT_SCALE = 16;

// The shaders, unchanged from valheim-world/cmd/valheim-world/static/worldmap.js.

const VERT = `#version 300 es
void main() {
  // One triangle over the whole canvas; the fragment reads gl_FragCoord.
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`;

// What both shaders share: the uniforms for the grid and the view, and the
// reads of the grid. The grid's row zero is north; the game's texture and
// its uv run south to north, so a uv is flipped on the way in and everything
// written against the game's texture keeps its sign.
const COMMON = `#version 300 es
precision highp float;
precision highp int;
precision highp usampler2D;
precision highp sampler2D;

uniform usampler2D uHeight;
uniform usampler2D uBiome;
uniform sampler2D uMask;
uniform vec2 uCentre;   // world x, z at the middle of the canvas
uniform vec2 uSpan;     // metres across the canvas, x and z
uniform vec2 uPx;       // canvas size in pixels
uniform float uRadius;  // the grid's half-width in metres
uniform float uWater;
uniform float uHOff, uHScale;
uniform int uSize;
uniform vec3 uPalette[16];

out vec4 outColor;

// The world's edge, where the generator stops: worldmap.WorldRadius.
const float WORLD = 10500.0;

// uv over the grid, as the game's map texture has it: x east, y north.
vec2 uvOf(vec2 w) { return (w + uRadius) / (2.0 * uRadius); }

// The grid's texel for a uv; row zero is north.
vec2 texelOf(vec2 uv) { return vec2(uv.x, 1.0 - uv.y) * float(uSize) - 0.5; }

float hAt(ivec2 t) {
  t = clamp(t, ivec2(0), ivec2(uSize - 1));
  return float(texelFetch(uHeight, t, 0).r) / uHScale - uHOff;
}

int bAt(ivec2 t) {
  t = clamp(t, ivec2(0), ivec2(uSize - 1));
  return int(texelFetch(uBiome, t, 0).r);
}

// Height between samples, bilinear as the game's RHalf texture is sampled.
float height(vec2 uv) {
  vec2 tc = texelOf(uv);
  vec2 f = floor(tc), w = tc - f;
  ivec2 i = ivec2(f);
  float a = hAt(i), b = hAt(i + ivec2(1, 0)), c = hAt(i + ivec2(0, 1)), d = hAt(i + ivec2(1, 1));
  return mix(mix(a, b, w.x), mix(c, d, w.x), w.y);
}

vec3 colourOf(int b) { return uPalette[clamp(b, 0, 15)]; }

// The biome colour between samples: the four neighbours' colours blended,
// as the game's RGB24 map texture is when sampled bilinear.
vec3 biomeColour(vec2 uv) {
  vec2 tc = texelOf(uv);
  vec2 f = floor(tc), w = tc - f;
  ivec2 i = ivec2(f);
  vec3 a = colourOf(bAt(i)), b = colourOf(bAt(i + ivec2(1, 0)));
  vec3 c = colourOf(bAt(i + ivec2(0, 1))), d = colourOf(bAt(i + ivec2(1, 1)));
  return mix(mix(a, b, w.x), mix(c, d, w.x), w.y);
}

vec4 mask(vec2 uv) { return texture(uMask, vec2(uv.x, 1.0 - uv.y)); }

// The alt-biome sectors: a plane of ids on the game's 2048-point biome grid,
// zero where a sector carries nothing. Nearest point, as the game's
// GetBiomeSector reads it, and uSecSize is zero until the plane arrives.
uniform usampler2D uSector;
uniform int uSecSize;

int sAt(vec2 w) {
  vec2 uv = uvOf(w);
  ivec2 t = ivec2(floor(vec2(uv.x, 1.0 - uv.y) * float(uSecSize)));
  t = clamp(t, ivec2(0), ivec2(uSecSize - 1));
  return int(texelFetch(uSector, t, 0).r);
}

// altSectors marks the sectors that carry alt biomes over the painted land:
// a line along the edge a pixel either side at every zoom, and a faint lift
// inside so the region reads as one from far out. Linear, before encode.
vec3 altSectors(vec3 c, vec2 w, float mpp) {
  if (uSecSize == 0) return c;
  int s = sAt(w);
  int l = sAt(w - vec2(mpp, 0.0)), r = sAt(w + vec2(mpp, 0.0));
  int d = sAt(w - vec2(0.0, mpp)), u = sAt(w + vec2(0.0, mpp));
  if (s != l || s != r || s != d || s != u) return vec3(0.08, 0.75, 0.70);
  if (s != 0) return mix(c, vec3(1.0), 0.06);
  return c;
}
`;

// The stand-in look, for a checkout without the game's textures. World
// coordinates throughout: everything drawn — trees, ripples, the coast — is a
// function of metres, so zooming in reveals it rather than enlarging it.
const FALLBACK_FRAG = COMMON + `
// Biome indices, as worldmap.BiomeIndex lays them out: the flag's bit plus one.
const int MEADOWS = 1, SWAMP = 2, MOUNTAIN = 3, BLACKFOREST = 4, PLAINS = 5,
          ASHLANDS = 6, DEEPNORTH = 7, OCEAN = 9, MISTLANDS = 10;

float cell() { return 2.0 * uRadius / float(uSize); }

// Height with the weights eased, which keeps the slope continuous across a
// cell edge for the shading: plain bilinear is creased between samples, and
// at 72x every crease is a ridge.
float heightSmooth(vec2 uv) {
  vec2 tc = texelOf(uv);
  vec2 f = floor(tc), w = tc - f;
  w = w * w * (3.0 - 2.0 * w);
  ivec2 i = ivec2(f);
  float a = hAt(i), b = hAt(i + ivec2(1, 0)), c = hAt(i + ivec2(0, 1)), d = hAt(i + ivec2(1, 1));
  return mix(mix(a, b, w.x), mix(c, d, w.x), w.y);
}

// Value noise over metres. The hash takes cell indices, which stay small
// enough for float32 to keep apart out to the world's edge.
float hash(vec2 p) {
  p = fract(p * vec2(0.1031, 0.1030));
  p += dot(p, p.yx + 33.33);
  return fract((p.x + p.y) * p.x);
}

float vnoise(vec2 p) {
  vec2 i = floor(p), f = fract(p);
  f = f * f * (3.0 - 2.0 * f);
  return mix(mix(hash(i), hash(i + vec2(1, 0)), f.x),
             mix(hash(i + vec2(0, 1)), hash(i + vec2(1, 1)), f.x), f.y);
}

// trees is the stipple: one dot per pitch-metre cell, jittered, drawn with
// probability density, radius r. mpp is metres per screen pixel, which does
// two jobs — the dot's edge is one pixel soft, and once a dot is smaller than
// a pixel the stipple fades to its own average cover, so zooming out darkens
// a forest evenly instead of into sparkle. Returns cover in [0, 1] and the
// dot's own shading in the second channel, for a lit side.
vec2 trees(vec2 w, float pitch, float density, float r, float mpp) {
  vec2 g = w / pitch;
  vec2 i = floor(g);
  float best = 1e9, lit = 0.0;
  for (int dy = -1; dy <= 1; dy++) for (int dx = -1; dx <= 1; dx++) {
    vec2 c = i + vec2(dx, dy);
    if (hash(c + 7.0) > density) continue;
    vec2 p = c + 0.15 + 0.7 * vec2(hash(c + 1.0), hash(c + 2.0));
    vec2 d = (g - p) * pitch;
    float dist = length(d);
    if (dist < best) { best = dist; lit = clamp(0.5 - 0.5 * (d.x + d.y) / max(r, 0.01), 0.0, 1.0); }
  }
  float aa = max(mpp, 0.15);
  float disc = 1.0 - smoothstep(r - aa, r + aa, best);
  float cover = density * 3.1416 * r * r / (pitch * pitch);
  float fade = smoothstep(r * 0.5, r * 2.0, mpp);
  return vec2(mix(disc, cover, fade), lit);
}

// land is the ground of one biome at one point: its colour with trees, rock
// and snow over it. Called for each of the cell's four neighbouring biomes
// and blended, so a border between two is a soft edge rather than a step.
vec3 land(int b, vec2 w, vec4 m, float h, float slope, float mpp) {
  vec3 base = colourOf(b);
  float above = h - uWater;

  // Texture at two scales, so a plain is not a flat fill: the finer one
  // only once it is bigger than a pixel.
  float grain = (vnoise(w / 28.0) - 0.5) * 0.14 + (vnoise(w / 4.5) - 0.5) * 0.10 * (1.0 - smoothstep(1.0, 3.0, mpp));
  base *= 1.0 + grain;

  // Where trees stand is the mask's red, which is the game's own rule; the
  // mist's green stands in for the mistlands' sparse wood.
  vec2 t = vec2(0.0);
  vec3 treeColour = base * 0.5;
  if (b == MEADOWS) {
    t = trees(w, 5.0, 0.6 * m.r, 1.6, mpp);
    treeColour = vec3(0.24, 0.36, 0.15);
  } else if (b == BLACKFOREST) {
    t = trees(w, 4.5, 0.9 * m.r, 2.1, mpp);
    treeColour = vec3(0.13, 0.20, 0.11);
  } else if (b == PLAINS) {
    t = trees(w, 9.0, 0.3 * m.r, 2.0, mpp);
    treeColour = vec3(0.40, 0.36, 0.18);
  } else if (b == MISTLANDS) {
    t = trees(w, 8.0, 0.35 * m.g, 1.8, mpp);
    treeColour = vec3(0.16, 0.15, 0.20);
  } else if (b == SWAMP) {
    t = trees(w, 7.0, 0.35, 1.6, mpp);
    treeColour = vec3(0.22, 0.20, 0.13);
  }
  vec3 lit = treeColour * (0.8 + 0.5 * t.y);
  vec3 colour = mix(base, lit, t.x);

  if (b == MOUNTAIN || b == DEEPNORTH) {
    // Snow on the gentle ground, rock where it is steep, in patches.
    vec3 rock = vec3(0.42, 0.41, 0.42);
    float bare = smoothstep(0.25, 0.6, slope) * (0.6 + 0.4 * vnoise(w / 30.0));
    bare = max(bare, smoothstep(0.5, 0.78, vnoise(w / 45.0) * 0.7 + vnoise(w / 12.0) * 0.3) * 0.8);
    colour = mix(base, rock, bare);
  }

  // The shore: a metre of lighter ground, then the coast's own line below.
  return mix(colour * 1.08, colour, smoothstep(0.0, 1.5, above));
}

// shade is the colour of one point of the world, mpp metres to a pixel.
vec3 shade(vec2 w, float mpp) {
  vec3 void_ = colourOf(0);
  float rim = length(w) - WORLD;
  if (rim > 2.0 * mpp) return void_;

  vec2 uv = uvOf(w);
  float h = heightSmooth(uv);
  vec4 m = mask(uv);

  // The slope, from the height half a texel either side. Steps of a cell
  // rather than a pixel: below that the eased bilinear says nothing new, and
  // the shading of a hill should not change with the zoom.
  float du = 0.5 / float(uSize);
  float dhdx = (heightSmooth(uv + vec2(du, 0.0)) - heightSmooth(uv - vec2(du, 0.0))) / cell();
  float dhdz = (heightSmooth(uv + vec2(0.0, du)) - heightSmooth(uv - vec2(0.0, du))) / cell();
  vec3 n = normalize(vec3(-dhdx, 1.0, -dhdz));
  vec3 light = normalize(vec3(-0.55, 0.7, 0.45));
  float lambert = max(dot(n, light), 0.0);
  float slope = length(vec2(dhdx, dhdz));

  vec3 colour;
  if (h < uWater) {
    // Sea: by depth, from the shelf to the deep, with a faint reading of the
    // bed's shape and a hatch of ripples that is there when a pixel is a
    // metre and gone by the time it is three. The game's ocean colour is
    // white, which is no sea to look at; a blue stands in.
    vec3 sea = colourOf(OCEAN);
    if (sea.r > 0.9 && sea.g > 0.9 && sea.b > 0.9) sea = vec3(0.18, 0.31, 0.48);
    float depth = uWater - h;
    colour = mix(sea * 1.45, sea * 0.55, clamp(depth / 40.0, 0.0, 1.0));
    colour *= 0.9 + 0.2 * lambert;
    float wave = sin((w.x * 0.9 + w.y * 0.3 + 9.0 * vnoise(w / 30.0)) * 1.25);
    float dash = smoothstep(0.35, 0.65, vnoise(vec2(w.x / 9.0, w.y / 3.0)));
    float bands = smoothstep(0.7, 0.95, wave) * dash * 0.07;
    colour += bands * (1.0 - smoothstep(1.0, 3.0, mpp));
    // The shallows, where a boat can still ground.
    colour = mix(colour, colour * 1.12, 1.0 - smoothstep(0.0, 2.5, depth));
  } else {
    // The four biomes about the point, blended by the eased distance to
    // each, and the work done once where they agree — which is nearly
    // everywhere.
    vec2 tc = texelOf(uv);
    vec2 f = floor(tc), wt = tc - f;
    wt = wt * wt * (3.0 - 2.0 * wt);
    ivec2 i = ivec2(f);
    int a = bAt(i), b = bAt(i + ivec2(1, 0)), c = bAt(i + ivec2(0, 1)), d = bAt(i + ivec2(1, 1));
    if (a == b && a == c && a == d) {
      colour = land(a, w, m, h, slope, mpp);
    } else {
      colour = mix(mix(land(a, w, m, h, slope, mpp), land(b, w, m, h, slope, mpp), wt.x),
                   mix(land(c, w, m, h, slope, mpp), land(d, w, m, h, slope, mpp), wt.x), wt.y);
    }

    // Relief: lit from the north-west, which is what makes a ridge a ridge.
    colour *= 0.62 + 0.55 * lambert;
    colour *= 1.0 + 0.06 * clamp((h - uWater) / 200.0, 0.0, 1.0);
  }

  // The coastline, drawn where the height crosses the waterline: the metres
  // to the crossing are the height over the slope, and a pixel and a half of
  // them is dark. Read as a line at any zoom.
  float toShore = abs(h - uWater) / max(slope, 1e-3);
  float line = 1.0 - smoothstep(0.8 * mpp, 1.8 * mpp, toShore);
  colour = mix(colour, colour * 0.45, line * 0.8);

  // The rim of the world, a pixel soft.
  return mix(colour, void_, smoothstep(-2.0 * mpp, 2.0 * mpp, rim));
}

// The stand-in works in the display's own space; nothing to encode.
vec3 encode(vec3 c) { return c; }
`;

// The game's own look: Custom/mapshader, ported from the GLSL in the Linux
// client's bundle, with its textures and constants from minimap.json. The
// names are the shader's, so a line here can be read against a line there.
//
// What is left out: the fog of war, since everything here is explored, the
// shared-map fade that goes with it, and the drifting clouds, which hide the
// ground the map is for. What is different: _Time stands still, and the
// pixel is averaged over a lattice when it covers more than one texel, which
// the game does not do and which only removes sparkle.
//
// The game renders in linear colour: its colour textures are sRGB and
// decoded as they are read, its material colours are decoded on the way to
// the shader, the arithmetic is linear, and the frame is encoded on the way
// out. The same here — the textures are uploaded as sRGB so the hardware
// decodes them, the constants are decoded in bindGameAssets, and encode is
// the last thing done to a pixel. Without this everything is a stop and a
// half too bright.
const GAME_FRAG = COMMON + `
uniform sampler2D _BackgroundTex, _FogLayerTex, _WaterTex, _lavaTex, _MountainTex, _CloudTex, _ForestTex, _SpaceTex;
uniform vec4 _ForestColor, _WaterColor, _WaterColorDeep, _WaterColorAshlands, _WaterColorAshlandsDeep;
uniform float _normalWidth, _normalIntensity;
uniform vec3 _lightColor, _ambientLightColor, _lavaColor1, _lavaColor2, _SunDir;
uniform vec4 _SunColor, _AmbientColor;
uniform float _zoom;    // the fraction of the map texture across the canvas
uniform float uTime;    // seconds; the game's _Time.x is this over twenty
uniform vec2 _mapCenterXZ;

float smooth01(float t) { t = clamp(t, 0.0, 1.0); return t * t * (3.0 - 2.0 * t); }

// DUtils.BlendOverlay, as the shader has it inlined.
float overlay(float a, float b) {
  return a <= 0.5 ? 2.0 * a * b : 1.0 - 2.0 * (1.0 - a) * (1.0 - b);
}

vec2 rotateAbout(vec2 p, float angle) {
  p -= 0.5;
  float c = cos(angle), s = sin(angle);
  return vec2(c * p.x + s * p.y, c * p.y - s * p.x) + 0.5;
}

vec4 shade4(vec2 w, vec2 fragCoord) {
  // The game snaps the uv to a grid of 7000 across the texture — _zoom
  // times a _pixelSize of 200 over _zoom, times 35 — so every read lands
  // on a three-and-a-half-metre cell. It is the map's pixel look.
  vec2 uv = uvOf(w);
  uv = trunc(uv * 7000.0 + 0.5) / 7000.0;

  float T = uTime / 20.0;
  vec4 fogLayer = texture(_FogLayerTex, uv * 5.0);
  vec4 m = mask(uv);
  float h = height(uv);

  // The normal, from the height a normalWidth to the west and to the south,
  // flat under the water.
  float hx = height(uv - vec2(_normalWidth, 0.0));
  float hy = height(uv - vec2(0.0, _normalWidth));
  vec3 n = normalize(vec3(hx - h, _normalIntensity, hy - h));
  if (h < 29.5) n = vec3(0.0, 1.0, 0.0);
  vec3 sun = normalize(_SunDir);
  vec3 light = max(dot(n, sun), 0.0) * _SunColor.rgb * _lightColor + _AmbientColor.rgb * _ambientLightColor;

  vec4 colour;
  if (h < 29.5) {
    vec4 bg = texture(_BackgroundTex, uv * 5.0);
    float t1 = clamp((h - 9.5) * 0.05, 0.0, 1.0);
    float t2 = clamp((h - 29.0) * -0.0714285746, 0.0, 1.0);
    vec4 ash = mix(_WaterColorAshlandsDeep, _WaterColorAshlands, t1);
    vec4 water = mix(_WaterColorDeep, _WaterColor, t1);
    float a = smooth01(m.b * 20.0);
    vec4 paper = mix(fogLayer, bg, 0.5);
    vec4 tint = mix(water, ash, a);
    vec4 base = paper * tint;
    float wobble = sin(uv.x * uv.y * 4000.0 + T * 20.0) * 0.01;
    vec4 waves = texture(_WaterTex, uv * 80.0 + vec2(T * 0.1, wobble));
    float k = (1.0 - t2) * waves.a;
    colour = mix(base, waves, k);
  } else {
    vec4 bg = texture(_BackgroundTex, uv * 40.0);
    vec4 main_ = vec4(biomeColour(uv), 1.0);
    float hi = max(bg.g, max(bg.r, bg.b));
    float lo = min(bg.g, min(bg.r, bg.b));
    if (hi - lo >= 1e-4) bg.rgb = vec3(hi);
    vec4 ground = main_ * bg;
    colour = ground * 1.5;

    // Lava, where the Ashlands mask says: three reads of the lava mask, two
    // of them turning with time, overlaid.
    float t = clamp((h - 30.5) * 0.2, 0.0, 1.0);
    vec4 lava0 = texture(_lavaTex, uv * 40.0);
    vec4 lava1 = texture(_lavaTex, rotateAbout(uv, uTime * 0.0005) * 80.0);
    vec4 lava2 = texture(_lavaTex, rotateAbout(uv, uTime * -0.00087) * 60.0);
    float glow = overlay(lava0.x, overlay(lava1.x, lava2.x));
    glow = pow(glow, 2.5);
    vec3 lavaColour = mix(_lavaColor1, _lavaColor2, glow);
    float where = overlay(t * m.b, lava0.x);
    colour.rgb = mix(colour.rgb, lavaColour, where);
  }

  // Mountains: the rock texture above seventy metres, snow above eighty on
  // the flat, whitening whatever is not already bright.
  vec4 mountain = texture(_MountainTex, uv * 70.0);
  float t70 = clamp((h - 70.0) * 0.04, 0.0, 1.0);
  float t80 = clamp((h - 80.0) * 0.05, 0.0, 1.0);
  colour = mix(colour, mountain, mountain.a * t70);
  float flat_ = clamp((n.y - 0.14) * 6.25, 0.0, 1.0);
  float lum = clamp(dot(colour.rgb, vec3(1.0)) * 1.5, 0.0, 1.0);
  float snow = max(t80 * flat_ - lum, 0.0);
  colour.rgb = mix(colour.rgb, vec3(0.5), snow);
  colour.rgb *= light;

  // The coast: dark within a few metres of the waterline, wider the further
  // out the map is.
  float coastWidth = clamp(_zoom * 50.0, 2.0, 10.0);
  float coast = 1.0 - min(abs(h - 29.5) / coastWidth, 1.0);
  colour = mix(colour, vec4(0.02, 0.01, 0.01, 1.0), coast);

  // The mist, from two drifting reads of the cloud texture.
  vec2 cloudUV1 = uv * 15.0 + 0.01 * vec2(sin(uv.y * 850.0 + T * 5.0), cos(uv.x * 600.0 + T * 4.7706151));
  vec2 cloudUV2 = uv * 20.0 + 0.01 * vec2(sin(uv.y * 750.0 + T * 5.0), cos(uv.x * 300.0 + T * 3.2706151));
  vec4 cloud1 = texture(_CloudTex, cloudUV1);
  vec4 cloud2 = texture(_CloudTex, cloudUV2);
  vec3 daylight = _SunColor.rgb + _AmbientColor.rgb;
  vec4 mist1 = vec4(daylight * vec3(0.7, 0.5, 1.0) * cloud1.a, cloud1.a * cloud1.a);
  vec4 mist2 = vec4(daylight * vec3(1.2, 0.7, 0.7), cloud2.a);
  colour = mix(colour, mist1, m.g);
  colour = mix(colour, mist2, m.g * cloud2.a);

  // The forest, lit at four fifths.
  vec4 forest = texture(_ForestTex, uv * 150.0) * _ForestColor;
  forest.rgb += 0.8 * (forest.rgb * light - forest.rgb);
  colour = mix(colour, forest, m.r * forest.a);

  // The game draws its drifting clouds here, a read of _CloudTex at uv * 7
  // less _CloudOffset mixed in by its alpha. Left out on purpose: a cloud
  // over the place being watched is only in the way of a map whose job is
  // reading terrain. The mist above stays, being the Mistlands' own mark.

  // Beyond the world, the stars: screen-space, sliding with the map's
  // centre.
  float dist = length(uv - 0.5);
  float s = smooth01((dist - 0.42) * 99.999794);
  vec4 space = texture(_SpaceTex, fragCoord * 0.0007 - _mapCenterXZ * 1e-5);
  return mix(colour, space, s);
}

vec3 shade(vec2 w, float mpp) { return shade4(w, gl_FragCoord.xy).rgb; }

// Linear to sRGB, the frame's encoding.
vec3 encode(vec3 c) {
  c = clamp(c, 0.0, 1.0);
  return mix(c * 12.92, 1.055 * pow(c, vec3(1.0 / 2.4)) - 0.055, step(0.0031308, c));
}
`;

// The pixel: one read of the world, or the average of a lattice of them
// when a pixel covers several cells and one read of it would be a lottery —
// the world view speckled with whichever cell each pixel happened to land
// on.
const MAIN = `
void main() {
  vec2 frag = gl_FragCoord.xy / uPx;
  vec2 w = uCentre + (frag - 0.5) * uSpan;
  vec2 mppv = uSpan / uPx;
  float mpp = mppv.x;
  float cellM = 2.0 * uRadius / float(uSize);
  int n = clamp(int(ceil(mpp / cellM)), 1, 4);
  if (n == 1) { outColor = vec4(encode(altSectors(shade(w, mpp), w, mpp)), 1.0); return; }
  vec3 sum = vec3(0.0);
  for (int y = 0; y < n; y++) for (int x = 0; x < n; x++) {
    vec2 o = (vec2(x, y) + 0.5) / float(n) - 0.5;
    sum += shade(w + o * mppv, mpp);
  }
  outColor = vec4(encode(altSectors(sum / float(n * n), w, mpp)), 1.0);
}`;

// toLinear is Unity's GammaToLinearSpace: an sRGB component decoded. Above
// one it keeps going, which is what the HDR lava colour needs.
export function toLinear(c) {
  if (c <= 0.04045) return c / 12.92;
  return Math.pow((c + 0.055) / 1.055, 2.4);
}

function compile(gl, type, src) {
  const s = gl.createShader(type);
  gl.shaderSource(s, src);
  gl.compileShader(s);
  if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) {
    const log = gl.getShaderInfoLog(s);
    gl.deleteShader(s);
    throw new Error('worldmap shader: ' + log);
  }
  return s;
}

// loadAssets fetches the game's textures and constants from under base
// (…/map/minimap.json and the textures it names), once per base, resolving
// to null when they are not there or do not load — the fallback look's cue.
const assetCache = new Map();
export function loadAssets(base) {
  let p = assetCache.get(base);
  if (!p) {
    p = fetch(base + 'map/minimap.json').then(r => {
      if (!r.ok) return null;
      return r.json();
    }).then(async json => {
      if (!json || !json.textures) return null;
      const images = {};
      await Promise.all(Object.entries(json.textures).map(([name, file]) => new Promise((resolve, reject) => {
        const img = new Image();
        img.onload = () => { images[name] = img; resolve(); };
        img.onerror = () => reject(new Error(`map texture ${file} did not load`));
        img.src = base + 'map/' + file;
      })));
      return { json, images };
    }).catch(err => {
      console.warn('worldmap: game textures unavailable, using the stand-in look:', err && err.message || err);
      return null;
    });
    assetCache.set(base, p);
  }
  return p;
}

// Texture units, fixed: the planes first, then the game's textures.
const UNIT = { uHeight: 0, uBiome: 1, uMask: 2, uSector: 3 };
const GAME_TEXTURES = [
  // name in minimap.json, sampler, mipmaps, sRGB
  ['background', '_BackgroundTex', false, true],
  ['foglayer', '_FogLayerTex', false, true],
  ['water', '_WaterTex', false, true],
  ['lava', '_lavaTex', true, false],
  ['mountain', '_MountainTex', false, true],
  ['clouds', '_CloudTex', true, true],
  ['forest', '_ForestTex', false, true],
  ['space', '_SpaceTex', true, true],
];

// Painter owns the GL canvas: the program for the look it has (the game's
// with assets, the stand-in without), the plane textures, and the draw.
class Painter {
  constructor(canvas) {
    this.canvas = canvas;
    const gl = canvas.getContext('webgl2', { antialias: false, alpha: false, preserveDrawingBuffer: false, powerPreference: 'high-performance' });
    if (!gl) throw new Error('worldmap: no WebGL2');
    this.gl = gl;
    this.prog = null;
    this.u = null;
    this.assets = null;
    this.map = null;
    this.info = null;
    this.sectors = null;
    this.showSectors = true;
    this.textures = {};
    gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
  }

  // build compiles the program for the look and binds everything the painter
  // already holds; called once the assets are known and again after a
  // context loss.
  build(assets) {
    const { gl } = this;
    this.assets = assets || null;
    if (this.prog) gl.deleteProgram(this.prog);
    for (const t of Object.values(this.textures)) gl.deleteTexture(t);
    this.textures = {};
    const prog = gl.createProgram();
    const vs = compile(gl, gl.VERTEX_SHADER, VERT);
    const fs = compile(gl, gl.FRAGMENT_SHADER, (this.assets ? GAME_FRAG : FALLBACK_FRAG) + MAIN);
    gl.attachShader(prog, vs);
    gl.attachShader(prog, fs);
    gl.linkProgram(prog);
    gl.deleteShader(vs);
    gl.deleteShader(fs);
    if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) {
      throw new Error('worldmap program: ' + gl.getProgramInfoLog(prog));
    }
    this.prog = prog;
    gl.useProgram(prog);
    const cache = new Map();
    this.u = new Proxy({}, { get: (_, name) => {
      if (!cache.has(name)) cache.set(name, gl.getUniformLocation(prog, name));
      return cache.get(name);
    } });
    gl.uniform1i(this.u.uSecSize, 0);
    if (this.assets) this.bindGameAssets();
    if (this.map) this.uploadMap();
    if (this.sectors) this.uploadSectors();
  }

  get ready() { return !!(this.prog && this.map); }

  // texture uploads one plane to a unit and points the sampler at it.
  texture(name, unit, internal, format, type, data, filter, w, h) {
    const { gl } = this;
    if (this.textures[name]) gl.deleteTexture(this.textures[name]);
    const t = gl.createTexture();
    this.textures[name] = t;
    gl.activeTexture(gl.TEXTURE0 + unit);
    gl.bindTexture(gl.TEXTURE_2D, t);
    gl.texImage2D(gl.TEXTURE_2D, 0, internal, w, h, 0, format, type, data);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, filter);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, filter);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.uniform1i(this.u[name], unit);
    return t;
  }

  setMap(map, info) {
    this.map = map;
    this.info = info || this.info;
    if (this.prog) this.uploadMap();
  }

  uploadMap() {
    const { gl, map } = this;
    const info = this.info || {};
    gl.useProgram(this.prog);
    const { size } = map;
    // Integer textures for what is read exactly; the mask is normalised, and
    // the hardware filters it the way the game's own mask texture is filtered.
    this.texture('uHeight', UNIT.uHeight, gl.R16UI, gl.RED_INTEGER, gl.UNSIGNED_SHORT, map.heights, gl.NEAREST, size, size);
    this.texture('uBiome', UNIT.uBiome, gl.R8UI, gl.RED_INTEGER, gl.UNSIGNED_BYTE, map.biomes, gl.NEAREST, size, size);
    this.texture('uMask', UNIT.uMask, gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE, map.mask, gl.LINEAR, size, size);
    gl.uniform1f(this.u.uRadius, info.radius || MAP_RADIUS);
    gl.uniform1f(this.u.uWater, info.water != null ? info.water : 30);
    gl.uniform1f(this.u.uHOff, info.heightOffset || HEIGHT_OFFSET);
    gl.uniform1f(this.u.uHScale, info.heightScale || HEIGHT_SCALE);
    gl.uniform1i(this.u.uSize, size);

    const palette = new Float32Array(48);
    if (info.palette && info.palette.length >= 48) {
      for (let i = 0; i < 48; i++) palette[i] = info.palette[i] / 255;
    } else {
      // worldmap.Palette, should the info not carry it.
      const p = [[20, 22, 26], [124, 158, 74], [94, 84, 61], [226, 231, 236], [59, 82, 51], [186, 168, 96], [130, 62, 47], [198, 214, 226], [20, 22, 26], [46, 79, 122], [74, 70, 88]];
      p.forEach((c, i) => palette.set(c.map(v => v / 255), 3 * i));
    }
    if (this.assets) {
      // The game's colours by biome index, over the engine's, decoded: the
      // game writes them into an sRGB texture and reads them back linear.
      const index = { Meadows: 1, Swamp: 2, Mountain: 3, BlackForest: 4, Plains: 5, AshLands: 6, DeepNorth: 7, Ocean: 9, Mistlands: 10 };
      for (const [name, rgb] of Object.entries(this.assets.json.palette || {})) {
        const i = index[name];
        if (i !== undefined) palette.set(rgb.map(toLinear), 3 * i);
      }
    }
    gl.uniform3fv(this.u.uPalette, palette);
  }

  setSectors(sectors) {
    this.sectors = sectors;
    if (this.prog) this.uploadSectors();
  }

  uploadSectors() {
    const { gl, sectors } = this;
    gl.useProgram(this.prog);
    this.texture('uSector', UNIT.uSector, gl.R16UI, gl.RED_INTEGER, gl.UNSIGNED_SHORT, sectors.ids, gl.NEAREST, sectors.size, sectors.size);
    gl.uniform1i(this.u.uSecSize, this.showSectors ? sectors.size : 0);
  }

  setShowSectors(on) {
    this.showSectors = !!on;
    if (this.prog && this.sectors) {
      this.gl.useProgram(this.prog);
      this.gl.uniform1i(this.u.uSecSize, this.showSectors ? this.sectors.size : 0);
    }
  }

  // bindGameAssets uploads the game's textures and sets its constants. The
  // filtering follows each texture's own settings: repeat everywhere,
  // bilinear, with mips only where the game built them.
  bindGameAssets() {
    const { gl } = this;
    const { json, images } = this.assets;
    GAME_TEXTURES.forEach(([name, slot, mips, srgb], i) => {
      const unit = 4 + i;
      const t = gl.createTexture();
      this.textures[slot] = t;
      gl.activeTexture(gl.TEXTURE0 + unit);
      gl.bindTexture(gl.TEXTURE_2D, t);
      gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, true);
      gl.texImage2D(gl.TEXTURE_2D, 0, srgb ? gl.SRGB8_ALPHA8 : gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE, images[name]);
      gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, false);
      if (mips) gl.generateMipmap(gl.TEXTURE_2D);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, mips ? gl.LINEAR_MIPMAP_LINEAR : gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT);
      gl.uniform1i(this.u[slot], unit);
    });

    const c = json.material.colors, f = json.material.floats;
    const v4 = (name, rgb) => { rgb = rgb.map(toLinear); gl.uniform4f(this.u[name], rgb[0], rgb[1], rgb[2], 1); };
    const v3 = (name, rgb) => { rgb = rgb.map(toLinear); gl.uniform3f(this.u[name], rgb[0], rgb[1], rgb[2]); };
    v4('_ForestColor', c.ForestColor);
    v4('_WaterColor', c.WaterColor);
    v4('_WaterColorDeep', c.WaterColorDeep);
    v4('_WaterColorAshlands', c.WaterColorAshlands);
    v4('_WaterColorAshlandsDeep', c.WaterColorAshlandsDeep);
    v3('_lightColor', c.lightColor);
    v3('_ambientLightColor', c.ambientLightColor);
    v3('_lavaColor1', c.lavaColor1);
    v3('_lavaColor2', c.lavaColor2);
    gl.uniform1f(this.u._normalWidth, f.normalWidth);
    gl.uniform1f(this.u._normalIntensity, f.normalIntensity);
    gl.uniform3f(this.u._SunDir, json.sun.dir[0], json.sun.dir[1], json.sun.dir[2]);
    v4('_SunColor', json.sun.color);
    v4('_AmbientColor', json.sun.ambient);
    gl.uniform1f(this.u.uTime, 0);
  }

  // draw paints the view — world x, z at the middle, metres across the
  // width — at w×h device pixels.
  draw(view, w, h) {
    const { gl, canvas } = this;
    if (!this.ready) return;
    w = Math.max(1, Math.round(w));
    h = Math.max(1, Math.round(h));
    if (canvas.width !== w || canvas.height !== h) {
      canvas.width = w;
      canvas.height = h;
    }
    gl.viewport(0, 0, w, h);
    gl.useProgram(this.prog);
    gl.uniform2f(this.u.uCentre, view.cx, view.cz);
    gl.uniform2f(this.u.uSpan, view.span, view.span * h / w);
    gl.uniform2f(this.u.uPx, w, h);
    if (this.assets) {
      const R = (this.info && this.info.radius) || MAP_RADIUS;
      gl.uniform1f(this.u._zoom, view.span / (2 * R));
      gl.uniform2f(this.u._mapCenterXZ, view.cx, view.cz);
    }
    gl.drawArrays(gl.TRIANGLES, 0, 3);
  }

  clear() {
    const { gl } = this;
    gl.clearColor(0.043, 0.051, 0.071, 1);
    gl.clear(gl.COLOR_BUFFER_BIT);
  }

  destroy() {
    const { gl } = this;
    for (const t of Object.values(this.textures)) gl.deleteTexture(t);
    if (this.prog) gl.deleteProgram(this.prog);
    const lose = gl.getExtension('WEBGL_lose_context');
    if (lose) lose.loseContext();
  }
}

// Marker images by URL, loaded once; a load redraws every map waiting on it.
const imageCache = new Map();
function markerImage(src, onload) {
  let entry = imageCache.get(src);
  if (!entry) {
    const img = new Image();
    entry = { img, waiting: new Set(), failed: false };
    img.onload = () => { for (const fn of entry.waiting) fn(); entry.waiting.clear(); };
    img.onerror = () => { entry.failed = true; for (const fn of entry.waiting) fn(); entry.waiting.clear(); };
    img.src = src;
    imageCache.set(src, entry);
  }
  if (!entry.img.complete && onload) entry.waiting.add(onload);
  return entry;
}

const MIN_ZOOM = 1, MAX_ZOOM = 72;
const DECLUTTER_ABOVE = 400;
const CLICK_SLOP = 4;       // CSS px of drag that still counts as a click
const DBLCLICK_WAIT = 260;  // ms a click waits for a second one

export class WorldMap {
  // el is a canvas (the GL canvas; the overlay goes beside it in its
  // parent, which is made position: relative) or a container to fill.
  constructor(el, { assetsBase = null, cx = 0, cz = 0, zoom = 1, sectorLabels = true, sectorOutlines = true, maxDPR = 2 } = {}) {
    if (!el) throw new Error('WorldMap: an element is required');
    if (el.tagName === 'CANVAS') {
      this.glCanvas = el;
      this.container = el.parentElement;
      this.ownsCanvases = false;
    } else {
      this.container = el;
      this.glCanvas = document.createElement('canvas');
      this.container.appendChild(this.glCanvas);
      this.ownsCanvases = true;
    }
    if (getComputedStyle(this.container).position === 'static') this.container.style.position = 'relative';
    this.overlay = document.createElement('canvas');
    this.glCanvas.insertAdjacentElement('afterend', this.overlay);
    for (const c of [this.glCanvas, this.overlay]) {
      Object.assign(c.style, { position: 'absolute', left: '0', top: '0', width: '100%', height: '100%', display: 'block' });
    }
    this.glCanvas.style.zIndex = '0';
    Object.assign(this.overlay.style, { zIndex: '1', touchAction: 'none', cursor: 'grab', userSelect: 'none' });
    this.overlay.setAttribute('aria-label', 'World map');

    this.assetsBase = assetsBase == null ? new URL('../', import.meta.url).href : String(assetsBase).replace(/\/?$/, '/');
    this.maxDPR = maxDPR;
    this.cx = cx; this.cz = cz; this.zoom = clampZoom(zoom);
    this.width = 0; this.height = 0; this.dpr = 1;
    this.markers = [];
    this.filter = null;
    this.visible = [];        // the markers drawn last frame, with screen positions
    this.hover = null;
    this.clickHandlers = [];
    this.hoverHandlers = [];
    this.viewHandlers = [];
    this.sectorLabels = sectorLabels;
    this.handle = null;
    this.sectorList = null;
    this.unsubscribe = [];
    this.destroyed = false;
    this.glDirty = true;
    this.overlayDirty = true;
    this.frame = 0;
    this.lastPaintMs = 0;

    this.painter = null;
    this.painterError = null;
    try {
      this.painter = new Painter(this.glCanvas);
      this.painter.showSectors = sectorOutlines;
    } catch (err) {
      this.painterError = err;
      console.warn(String(err));
    }
    this.assets = loadAssets(this.assetsBase);
    this.assets.then(assets => {
      if (this.destroyed || !this.painter) return;
      try {
        this.painter.build(assets);
      } catch (err) {
        // The game shader failed on this GPU: the stand-in.
        console.warn('worldmap:', err.message || err);
        if (assets) {
          try { this.painter.build(null); } catch (err2) { this.painterError = err2; console.warn(String(err2)); }
        } else {
          this.painterError = err;
        }
      }
      this.invalidate(true);
    });

    this.glCanvas.addEventListener('webglcontextlost', this._onLost = e => { e.preventDefault(); });
    this.glCanvas.addEventListener('webglcontextrestored', this._onRestored = () => {
      if (!this.painter) return;
      this.assets.then(assets => { try { this.painter.build(assets); } catch (err) { console.warn(String(err)); } this.invalidate(true); });
    });

    this.ctx = this.overlay.getContext('2d');
    this._bindPointer();
    this.ro = new ResizeObserver(() => this._resize());
    this.ro.observe(this.container);
    this._resize();
  }

  // setWorld feeds the map from a World handle: its info, map and sectors
  // as they arrive (or at once, when they already have).
  setWorld(handle) {
    for (const off of this.unsubscribe) off();
    this.unsubscribe = [];
    this.handle = handle;
    this.sectorList = null;
    if (!handle) { this.invalidate(true); return; }
    this.unsubscribe.push(handle.on('map', map => {
      if (this.painter) this.painter.setMap(map, handle.info);
      this.invalidate(true);
    }));
    this.unsubscribe.push(handle.on('sectors', sectors => {
      this.sectorList = sectors.list;
      if (this.painter) this.painter.setSectors(sectors);
      this.invalidate(true);
    }));
  }

  // setMap and setSectors feed the painter directly, for a page with its
  // own source of planes.
  setMap(map, info) { if (this.painter) this.painter.setMap(map, info); this.invalidate(true); }
  setSectors(sectors) { this.sectorList = sectors.list || null; if (this.painter) this.painter.setSectors(sectors); this.invalidate(true); }

  setSectorOutlines(on) { if (this.painter) this.painter.setShowSectors(on); this.invalidate(true); }
  setSectorLabels(on) { this.sectorLabels = !!on; this.invalidate(false); }

  // setMarkers replaces the markers: {x, z, img (Image, URL or null), size
  // (px), kind, color, label, priority, data}.
  setMarkers(list) {
    const redraw = () => this.invalidate(false);
    this.markers = (list || []).map(m => {
      const out = Object.assign({}, m);
      out.priority = +m.priority || 0;
      out.color = m.color || '#ffffff';
      if (typeof m.img === 'string') {
        const entry = markerImage(m.img, redraw);
        out.imgEntry = entry;
        out.img = entry.img;
      } else if (m.img instanceof HTMLImageElement) {
        out.imgEntry = { img: m.img, failed: false };
      } else {
        out.img = null;
        out.imgEntry = null;
      }
      out.size = m.size || (out.img ? 22 : 8);
      return out;
    });
    this.hover = null;
    this.invalidate(false);
  }

  // setFilter narrows the markers drawn to those fn(marker) accepts.
  setFilter(fn) { this.filter = typeof fn === 'function' ? fn : null; this.invalidate(false); }

  onClick(fn) { this.clickHandlers.push(fn); return () => { this.clickHandlers = this.clickHandlers.filter(f => f !== fn); }; }
  onHover(fn) { this.hoverHandlers.push(fn); return () => { this.hoverHandlers = this.hoverHandlers.filter(f => f !== fn); }; }
  onView(fn) { this.viewHandlers.push(fn); return () => { this.viewHandlers = this.viewHandlers.filter(f => f !== fn); }; }

  // centerOn moves the view to x, z, at zoom when given.
  centerOn(x, z, zoom) {
    this.cx = +x || 0;
    this.cz = +z || 0;
    if (zoom != null) this.zoom = clampZoom(zoom);
    this._clampView();
    this.invalidate(true);
    this._viewChanged();
  }

  setZoom(zoom) { this.centerOn(this.cx, this.cz, zoom); }

  // view is the current window: centre, zoom, metres across the width.
  get view() { return { cx: this.cx, cz: this.cz, zoom: this.zoom, span: this.span, mpp: this.mpp }; }

  // mpp is metres per CSS pixel: at zoom 1 the shorter side spans the grid.
  get mpp() { return 2 * MAP_RADIUS / (this.zoom * Math.max(1, Math.min(this.width, this.height))); }
  get span() { return this.mpp * Math.max(1, this.width); }

  // worldToScreen and screenToWorld, in CSS pixels of the canvas.
  worldToScreen(x, z) {
    const mpp = this.mpp;
    return { x: this.width / 2 + (x - this.cx) / mpp, y: this.height / 2 - (z - this.cz) / mpp };
  }

  screenToWorld(px, py) {
    const mpp = this.mpp;
    return { x: this.cx + (px - this.width / 2) * mpp, z: this.cz - (py - this.height / 2) * mpp };
  }

  // invalidate schedules a frame: the GL paint too when the view or the
  // planes changed, the overlay alone otherwise.
  invalidate(gl) {
    if (gl) this.glDirty = true;
    this.overlayDirty = true;
    if (!this.frame && !this.destroyed) this.frame = requestAnimationFrame(() => this._frame());
  }

  _frame() {
    this.frame = 0;
    if (this.destroyed) return;
    if (this.width === 0 || this.height === 0) return;
    if (this.glDirty) {
      this.glDirty = false;
      const t0 = performance.now();
      if (this.painter && this.painter.ready) {
        try {
          this.painter.draw({ cx: this.cx, cz: this.cz, span: this.span }, this.width * this.dpr, this.height * this.dpr);
        } catch (err) {
          console.warn('worldmap draw:', err);
        }
      } else if (this.painter && this.painter.prog) {
        this.painter.clear();
      }
      this.lastPaintMs = performance.now() - t0;
    }
    if (this.overlayDirty) {
      this.overlayDirty = false;
      this._drawOverlay();
    }
  }

  _resize() {
    const r = this.container.getBoundingClientRect();
    const w = Math.max(0, Math.round(r.width)), h = Math.max(0, Math.round(r.height));
    const dpr = Math.min(window.devicePixelRatio || 1, this.maxDPR);
    if (w === this.width && h === this.height && dpr === this.dpr) return;
    this.width = w; this.height = h; this.dpr = dpr;
    this.overlay.width = Math.max(1, Math.round(w * dpr));
    this.overlay.height = Math.max(1, Math.round(h * dpr));
    this._clampView();
    this.invalidate(true);
    this._viewChanged();
  }

  _clampView() {
    // The centre stays within the grid; the void around the world is
    // allowed, a page of it is not.
    const lim = MAP_RADIUS;
    this.cx = Math.max(-lim, Math.min(lim, this.cx));
    this.cz = Math.max(-lim, Math.min(lim, this.cz));
  }

  _viewChanged() {
    if (!this.viewHandlers.length) return;
    const v = this.view;
    for (const fn of this.viewHandlers) { try { fn(v); } catch (err) { console.error(err); } }
  }

  // The overlay: markers, the hovered marker's label, the sector names.
  _drawOverlay() {
    const ctx = this.ctx;
    const W = this.width, H = this.height;
    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);

    if (this.painterError && !(this.painter && this.painter.ready)) {
      ctx.fillStyle = '#e8d8b0';
      ctx.font = '14px system-ui, sans-serif';
      ctx.textAlign = 'center';
      ctx.fillText('The map needs WebGL2: ' + (this.painterError.message || this.painterError), W / 2, H / 2);
    }

    // The sector names, once a label is readable: past zoom 3.
    if (this.sectorLabels && this.sectorList && this.zoom > 3) {
      ctx.font = '600 12px system-ui, sans-serif';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.lineWidth = 3;
      ctx.lineJoin = 'round';
      ctx.strokeStyle = 'rgba(0, 0, 0, 0.75)';
      ctx.fillStyle = 'rgb(140, 235, 225)';
      for (const s of this.sectorList) {
        const p = this.worldToScreen(s.x, s.z);
        if (p.x < -200 || p.y < -20 || p.x > W + 200 || p.y > H + 20) continue;
        const text = (s.names || []).join(', ');
        ctx.strokeText(text, p.x, p.y);
        ctx.fillText(text, p.x, p.y);
      }
    }

    // The markers in view, then decluttered by priority when there are too
    // many to read.
    const margin = 40;
    const mpp = this.mpp;
    const visible = [];
    for (const m of this.markers) {
      if (this.filter && !this.filter(m)) continue;
      const sx = W / 2 + (m.x - this.cx) / mpp, sy = H / 2 - (m.z - this.cz) / mpp;
      if (sx < -margin || sy < -margin || sx > W + margin || sy > H + margin) continue;
      visible.push({ m, sx, sy, r: m.size / 2 });
    }
    let drawn = visible;
    if (visible.length > DECLUTTER_ABOVE) {
      drawn = declutter(visible);
    }
    this.visible = drawn;

    // Lowest priority first, so the important ones end on top.
    drawn.sort((a, b) => a.m.priority - b.m.priority);
    let hovered = null;
    for (const v of drawn) {
      if (this.hover && v.m === this.hover.m) { hovered = v; continue; }
      this._drawMarker(ctx, v, false);
    }
    if (hovered) {
      this._drawMarker(ctx, hovered, true);
      this._drawLabel(ctx, hovered);
    }
  }

  _drawMarker(ctx, v, lifted) {
    const { m, sx, sy } = v;
    const r = v.r * (lifted ? 1.25 : 1);
    const img = m.imgEntry && !m.imgEntry.failed && m.img && m.img.complete && m.img.naturalWidth > 0 ? m.img : null;
    if (img) {
      ctx.save();
      ctx.beginPath();
      ctx.arc(sx, sy, r, 0, Math.PI * 2);
      ctx.closePath();
      ctx.clip();
      ctx.drawImage(img, sx - r, sy - r, 2 * r, 2 * r);
      ctx.restore();
      ctx.beginPath();
      ctx.arc(sx, sy, r, 0, Math.PI * 2);
      ctx.lineWidth = lifted ? 3 : 2;
      ctx.strokeStyle = lifted ? '#ffffff' : m.color;
      ctx.stroke();
      if (lifted) {
        ctx.beginPath();
        ctx.arc(sx, sy, r + 3, 0, Math.PI * 2);
        ctx.lineWidth = 2;
        ctx.strokeStyle = m.color;
        ctx.stroke();
      }
    } else {
      // A dot: the colour inside a dark ring, the marker's own ring when lifted.
      const rd = Math.max(2.5, Math.min(r, 7));
      ctx.beginPath();
      ctx.arc(sx, sy, rd, 0, Math.PI * 2);
      ctx.fillStyle = m.color;
      ctx.fill();
      ctx.lineWidth = lifted ? 2 : 1;
      ctx.strokeStyle = lifted ? '#ffffff' : 'rgba(0, 0, 0, 0.75)';
      ctx.stroke();
    }
  }

  _drawLabel(ctx, v) {
    const label = v.m.label || v.m.kind || '';
    if (!label) return;
    ctx.font = '13px system-ui, sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'bottom';
    const w = ctx.measureText(label).width + 12;
    const h = 20;
    let x = v.sx, y = v.sy - v.r * 1.25 - 8;
    if (y - h < 2) y = v.sy + v.r * 1.25 + 8 + h;
    x = Math.max(w / 2 + 2, Math.min(this.width - w / 2 - 2, x));
    ctx.fillStyle = 'rgba(12, 12, 16, 0.88)';
    roundRect(ctx, x - w / 2, y - h, w, h, 4);
    ctx.fill();
    ctx.strokeStyle = v.m.color;
    ctx.lineWidth = 1;
    roundRect(ctx, x - w / 2 + 0.5, y - h + 0.5, w - 1, h - 1, 4);
    ctx.stroke();
    ctx.fillStyle = '#f2e9d6';
    ctx.fillText(label, x, y - 4);
  }

  // The marker under a CSS-pixel position, nearest first, or null.
  markerAt(px, py) {
    let best = null, bestD = Infinity;
    for (const v of this.visible) {
      const d = Math.hypot(v.sx - px, v.sy - py);
      const reach = Math.max(v.r, 5) + 2;
      if (d <= reach && (d < bestD || (best && v.m.priority > best.m.priority && d <= reach))) { best = v; bestD = d; }
    }
    return best;
  }

  _bindPointer() {
    const el = this.overlay;
    const pointers = new Map();
    let drag = null;        // {x, y, moved}
    let pinch = null;       // {d, cx, cz, zoom, mid}
    let clickTimer = 0;
    const pos = e => {
      const r = el.getBoundingClientRect();
      return { x: e.clientX - r.left, y: e.clientY - r.top };
    };

    this._listeners = [];
    const on = (type, fn, opts) => { el.addEventListener(type, fn, opts); this._listeners.push([type, fn, opts]); };

    on('pointerdown', e => {
      if (e.button !== 0 && e.pointerType === 'mouse') return;
      el.setPointerCapture(e.pointerId);
      const p = pos(e);
      pointers.set(e.pointerId, p);
      if (pointers.size === 1) {
        drag = { x: p.x, y: p.y, sx: p.x, sy: p.y, moved: false };
        el.style.cursor = 'grabbing';
      } else if (pointers.size === 2) {
        const [a, b] = Array.from(pointers.values());
        pinch = { d: Math.hypot(a.x - b.x, a.y - b.y), zoom: this.zoom, mid: { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 } };
        pinch.world = this.screenToWorld(pinch.mid.x, pinch.mid.y);
        drag = null;
      }
    });

    on('pointermove', e => {
      const p = pos(e);
      if (pointers.has(e.pointerId)) pointers.set(e.pointerId, p);
      if (pinch && pointers.size >= 2) {
        const [a, b] = Array.from(pointers.values());
        const d = Math.hypot(a.x - b.x, a.y - b.y);
        const mid = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
        this.zoom = clampZoom(pinch.zoom * (d / Math.max(1, pinch.d)));
        // Keep the world point that was under the pinch's middle under it.
        const now = this.screenToWorld(mid.x, mid.y);
        this.cx += pinch.world.x - now.x;
        this.cz += pinch.world.z - now.z;
        this._clampView();
        this.invalidate(true);
        this._viewChanged();
        return;
      }
      if (drag && pointers.has(e.pointerId)) {
        const dx = p.x - drag.x, dy = p.y - drag.y;
        if (!drag.moved && Math.hypot(p.x - drag.sx, p.y - drag.sy) > CLICK_SLOP) drag.moved = true;
        if (drag.moved) {
          const mpp = this.mpp;
          this.cx -= dx * mpp;
          this.cz += dy * mpp;
          drag.x = p.x; drag.y = p.y;
          this._clampView();
          this.invalidate(true);
          this._viewChanged();
        }
        return;
      }
      // Hover.
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
        if (wasDrag && !wasDrag.moved && e.type === 'pointerup') {
          // A click, unless a second one follows and makes it a double.
          const hit = this.markerAt(p.x, p.y);
          const w = this.screenToWorld(p.x, p.y);
          const ev = { x: w.x, z: w.z, marker: hit ? hit.m : null, screen: p };
          clearTimeout(clickTimer);
          clickTimer = setTimeout(() => {
            clickTimer = 0;
            for (const fn of this.clickHandlers) { try { fn(ev); } catch (err) { console.error(err); } }
          }, DBLCLICK_WAIT);
        }
      } else if (pointers.size === 1) {
        const [q] = Array.from(pointers.values());
        drag = { x: q.x, y: q.y, sx: q.x, sy: q.y, moved: true };
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
      this._zoomAbout(p.x, p.y, this.zoom * 2);
    });

    on('wheel', e => {
      e.preventDefault();
      const p = pos(e);
      const k = e.deltaMode === 1 ? 20 : e.deltaMode === 2 ? 400 : 1;
      this._zoomAbout(p.x, p.y, this.zoom * Math.exp(-e.deltaY * k * 0.0015));
    }, { passive: false });

    on('pointerleave', () => {
      if (this.hover) {
        this.hover = null;
        this.invalidate(false);
        for (const fn of this.hoverHandlers) { try { fn(null); } catch (err) { console.error(err); } }
      }
    });
    on('contextmenu', e => e.preventDefault());
  }

  // _zoomAbout changes the zoom with the world point under px, py held
  // still on screen.
  _zoomAbout(px, py, zoom) {
    const before = this.screenToWorld(px, py);
    this.zoom = clampZoom(zoom);
    const after = this.screenToWorld(px, py);
    this.cx += before.x - after.x;
    this.cz += before.z - after.z;
    this._clampView();
    this.invalidate(true);
    this._viewChanged();
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
    if (this.painter) this.painter.destroy();
    this.overlay.remove();
    if (this.ownsCanvases) this.glCanvas.remove();
    this.markers = [];
    this.visible = [];
  }
}

function clampZoom(z) { return Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, +z || MIN_ZOOM)); }

function roundRect(ctx, x, y, w, h, r) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}

// declutter keeps, by priority, the markers that do not sit on one already
// kept: a hash grid of cells the size of a marker, a kept marker claiming
// the cells its disc touches.
function declutter(visible) {
  const sorted = visible.slice().sort((a, b) => b.m.priority - a.m.priority || b.r - a.r);
  const cell = 10;
  const taken = new Map();   // cell key → kept entries in it
  const kept = [];
  for (const v of sorted) {
    const pad = Math.max(v.r, 4) * 0.9;
    const x0 = Math.floor((v.sx - pad) / cell), x1 = Math.floor((v.sx + pad) / cell);
    const y0 = Math.floor((v.sy - pad) / cell), y1 = Math.floor((v.sy + pad) / cell);
    let free = true;
    for (let y = y0; y <= y1 && free; y++) for (let x = x0; x <= x1 && free; x++) {
      const list = taken.get(y * 65536 + x);
      if (!list) continue;
      for (const k of list) {
        const minD = (Math.max(v.r, 4) + Math.max(k.r, 4)) * 0.9;
        if (Math.hypot(k.sx - v.sx, k.sy - v.sy) < minD) { free = false; break; }
      }
    }
    if (!free) continue;
    kept.push(v);
    for (let y = y0; y <= y1; y++) for (let x = x0; x <= x1; x++) {
      const key = y * 65536 + x;
      let list = taken.get(key);
      if (!list) taken.set(key, list = []);
      list.push(v);
    }
  }
  return kept;
}

export default WorldMap;
