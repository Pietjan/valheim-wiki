// Command wiki-icons renders the wiki's location and creature icons from
// the game's models: the VMSH meshes valheim-npc extracts from the asset
// bundles (-meshes), drawn by a software rasterizer into round PNGs.
//
// Every '<X>~site.bin' (a location prefab, flattened) becomes
// locations/<X>.png; the generator-built camps that have no site mesh
// are stood in for by one of their rooms assembled from the dungeon
// tables. Every creature prefab that names.tsv knows as an enemy or a
// fish, plus the traders, becomes creatures/<X>.png and a transparent
// creatures/<X>.flat.png portrait. All of it is lit like the game's clear
// day, from a three-quarter orthographic view.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pietjan/valheim-wiki/internal/render"
	"github.com/pietjan/valheim-wiki/internal/vmsh"
)

// Icon sizes and the supersampling behind them: a disc is rendered at
// 4× and box-filtered down; a portrait shares the creature's render.
const (
	discSize     = 256
	portraitSize = 512
	supersample  = 4
	exposure     = 1.6
)

// The camera and the sun for each kind of icon. Yaw 45 looks from the
// +x +z diagonal (a creature faces +z, so this shows its face and flank);
// buildings are seen from higher up than creatures. The key light stands
// to the camera's upper left so that the faces turned to the viewer are
// the ones the sun paints.
var (
	siteView = render.Options{Yaw: 45, Pitch: 32, Fit: 0.80, SunAzimuth: 35, SunElevation: 50,
		Size: discSize * supersample, ShadowMap: 1536}
	creatureView = render.Options{Yaw: 45, Pitch: 15, Fit: 0.80, SunAzimuth: 35, SunElevation: 50,
		Size: portraitSize * supersample, ShadowMap: 1024}
)

// job is one prefab to draw.
type job struct {
	kind  string // "locations" or "creatures"
	name  string
	bytes int64 // the mesh's size, to start the big ones first
	build func() (*render.Scene, error)
}

func main() {
	meshes := flag.String("meshes", "", "valheim-npc's meshes directory (VMSH .bin files, materials.json, textures/)")
	out := flag.String("out", "assets/icons", "output directory; icons go in locations/ and creatures/ under it")
	only := flag.String("only", "", "comma-separated prefab names to render, default all")
	names := flag.String("names", "cmd/wiki-data/names.tsv", "the wiki's names table, which says which prefabs are creatures")
	sites := flag.String("sites", "../valheim-world/internal/world/sites.tsv", "valheim-world's location table, for the ground each location digs to (optional)")
	dungeon := flag.String("dungeon", "../valheim-world/internal/world/dungeon", "valheim-world's dungeon tables, for the camps built from rooms (optional)")
	workers := flag.Int("workers", runtime.GOMAXPROCS(0), "parallel renders")
	flag.Parse()
	if *meshes == "" {
		flag.Usage()
		os.Exit(2)
	}
	mats, err := render.LoadManifest(*meshes)
	if err != nil {
		fail(err)
	}
	floors, err := loadFloors(*sites)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wiki-icons: %v; every location is drawn down to y=0\n", err)
	}

	var jobs []job
	siteJobs, err := siteJobs(*meshes, mats, floors)
	if err != nil {
		fail(err)
	}
	jobs = append(jobs, siteJobs...)
	campJobs, err := campJobs(*meshes, *dungeon, mats)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wiki-icons: %v; the generator-built camps get no icon\n", err)
	}
	jobs = append(jobs, campJobs...)
	creatureJobs, err := creatureJobs(*meshes, *names, mats)
	if err != nil {
		fail(err)
	}
	jobs = append(jobs, creatureJobs...)

	if *only != "" {
		want := map[string]bool{}
		for _, n := range strings.Split(*only, ",") {
			want[strings.TrimSpace(n)] = true
		}
		var kept []job
		for _, j := range jobs {
			if want[j.name] {
				kept = append(kept, j)
			}
		}
		jobs = kept
	}
	for _, kind := range []string{"locations", "creatures"} {
		if err := os.MkdirAll(filepath.Join(*out, kind), 0o755); err != nil {
			fail(err)
		}
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].bytes > jobs[j].bytes })

	start := time.Now()
	queue := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := 0
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				t := time.Now()
				tris, err := renderJob(j, *out)
				mu.Lock()
				if err != nil {
					failed++
					fmt.Fprintf(os.Stderr, "wiki-icons: %s/%s: %v\n", j.kind, j.name, err)
				} else {
					fmt.Printf("%s/%s.png\t%d tris\t%d ms\n", j.kind, j.name, tris, time.Since(t).Milliseconds())
				}
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		queue <- j
	}
	close(queue)
	wg.Wait()
	fmt.Printf("%d icons, %d failed, %s\n", len(jobs)-failed, failed, time.Since(start).Round(time.Millisecond))
	if failed > 0 {
		os.Exit(1)
	}
}

// renderJob draws one prefab and writes its PNGs.
func renderJob(j job, out string) (tris int, err error) {
	s, err := j.build()
	if err != nil {
		return 0, err
	}
	s.Finish()
	if s.Triangles() == 0 {
		return 0, fmt.Errorf("nothing to draw")
	}
	switch j.kind {
	case "locations":
		img := render.Render(s, siteView)
		disc := img.Resolve(supersample, exposure)
		err = render.WritePNG(filepath.Join(out, j.kind, j.name+".png"), disc.Disc())
	case "creatures":
		img := render.Render(s, creatureView)
		disc := img.Resolve(supersample*portraitSize/discSize, exposure)
		if err = render.WritePNG(filepath.Join(out, j.kind, j.name+".png"), disc.Disc()); err != nil {
			break
		}
		flat := img.Resolve(supersample, exposure)
		err = render.WritePNG(filepath.Join(out, j.kind, j.name+".flat.png"), flat.Flat())
	}
	return s.Triangles(), err
}

// siteJobs is one job per '<X>~site.bin', the Dev* test scenes left out.
func siteJobs(meshes string, mats *render.Manifest, floors map[string]float64) ([]job, error) {
	files, err := filepath.Glob(filepath.Join(meshes, "*~site.bin"))
	if err != nil {
		return nil, err
	}
	var jobs []job
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), "~site.bin")
		if strings.HasPrefix(name, "Dev") {
			continue
		}
		st, err := os.Stat(f)
		if err != nil {
			return nil, err
		}
		floor := floors[name]
		jobs = append(jobs, job{kind: "locations", name: name, bytes: st.Size(), build: func() (*render.Scene, error) {
			m, err := vmsh.Load(f)
			if err != nil {
				return nil, err
			}
			s := &render.Scene{Floor: floor}
			s.Add(m, mats, render.Identity)
			return s, nil
		}})
	}
	return jobs, nil
}

// creatureJobs is one job per creature prefab with a mesh: what names.tsv
// calls an enemy ($enemy_*) or a fish ($animal_fish*), and the people
// one meets (the traders, the Bog Witch, Odin's ravens) when their mesh
// exists. Ragdolls are the corpses, not the creatures.
func creatureJobs(meshes, names string, mats *render.Manifest) ([]job, error) {
	b, err := os.ReadFile(names)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{"Haldor": true, "Hildir": true, "BogWitch": true, "Munin": true, "Hugin": true}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			continue
		}
		if strings.HasPrefix(f[1], "$enemy_") || strings.HasPrefix(f[1], "$animal_fish") {
			want[f[0]] = true
		}
	}
	var jobs []job
	for name := range want {
		if strings.HasSuffix(name, "_ragdoll") {
			continue
		}
		path := filepath.Join(meshes, name+".bin")
		st, err := os.Stat(path)
		if err != nil {
			continue
		}
		jobs = append(jobs, job{kind: "creatures", name: name, bytes: st.Size(), build: func() (*render.Scene, error) {
			m, err := vmsh.Load(path)
			if err != nil {
				return nil, err
			}
			s := &render.Scene{}
			s.Add(m, mats, render.Identity)
			return s, nil
		}})
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].name < jobs[j].name })
	return jobs, nil
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "wiki-icons: %v\n", err)
	os.Exit(1)
}
