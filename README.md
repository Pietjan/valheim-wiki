# Vegvísir — a Valheim wiki that knows your world

A static Valheim wiki generated from the game's own files, with a seed
viewer that runs in the browser: type your world's seed and every page
gains a *Your world* panel saying where the nearest troll cave is, how
far the trader stands from spawn, which biomes border your meadows.
Without a seed it is a complete wiki; with one it is a wiki of your
world.

Published at https://pietjan.github.io/valheim-wiki/ on every push to
`main`.

## What is in it

Everything is read out of the client's Unity asset bundles by
`cmd/wiki-data` (game 1.0.20): 1,500 items with stats per quality level
and recipes, 10 crafting stations with what each level unlocks, 55
smelters, ovens, fermenters and the like, 689 building pieces across the
hammer, hoe, cultivator and feast menus, 173 creatures with health per
star, attacks, resistances, drops, taming and spawn rules, 31 raids, 97
status effects with their numbers, 24 skills, 3 traders, 232 location
rules with what each prefab holds (chests and their loot tables,
spawners, pickables, runestones, altars), 358 dungeon rooms, 475
pickables, trees, ores and destructibles with their drop tables, the
vegetation rules that place them, 9 biomes with their weather, and 32
alt-biome variants.

The icons are the game's sprites; the round location and creature icons
are rendered from the game's models by `cmd/wiki-icons`, a software
rasterizer.

The seed viewer is `cmd/world-wasm`: the generator port from
[valheim-world](https://github.com/pietjan/valheim-world) compiled to
WebAssembly, run in a Web Worker and cached per seed in IndexedDB. The
biome map, the rivers and every placement rule are reproduced; a world
takes about half a minute the first time.

## Building it

```bash
make run        # build the seed engine and the site, serve at http://localhost:8080
make build      # build for GitHub Pages (base path /valheim-wiki/) into public/
make test
```

Only Go is needed to build the site: `data/` and `assets/icons/` are
committed. Regenerating them needs the game and the sibling projects:

```bash
make data       # the client at /mnt/c/Program Files (x86)/Steam/... → data/, assets/icons/
make icons      # valheim-npc's extracted meshes → assets/icons/{locations,creatures}
```

`make probe names=Troll,Crypt2` dumps every component on a prefab with
its hierarchy, which is how a new reader gets written.

## How it is put together

```
cmd/wiki-data     bundles → data/*.json + the game's sprites; one extractor file per domain
cmd/wiki-icons    VMSH models → round icons (internal/render is the rasterizer)
cmd/world-wasm    the seed engine for the browser (engine/ is plain Go, tested natively)
cmd/wiki          the static site generator (internal/site: index, pages, templates)
internal/model    the data contract: one JSON file per field of model.DB
internal/worldgen, worldmap, stablehash, unityrandom, scalar, vmsh   copied from valheim-world
internal/unity, internal/assets    Unity bundle readers copied from valheim-npc and valheim-forge
web/templates, web/static          html/template files, CSS, the JS modules, fonts, UI art, map textures
```

`docs/architecture.md` has the contracts: the data model, the worker
protocol, the URL scheme.

## The art

The item and map sprites, the UI ornaments, the map textures and the
models the icons are rendered from are Iron Gate's; the fonts are Averia
Serif Libre (OFL) and Norse. This is an unofficial fan site.
