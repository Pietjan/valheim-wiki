package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/render"
	"github.com/pietjan/valheim-wiki/internal/vmsh"
)

// camps are the locations a dungeon generator builds from rooms at
// placement time, so no site mesh exists for them. Each is stood in for
// by rooms from its generator's set, assembled from the dungeon tables:
// the room's pieces placed where the room prefab places them. The first
// room stands at the origin; the others keep their offset from it as the
// room palette lays them out, which stacks Hildir's tower floor on floor.
var camps = map[string][]string{
	"GoblinCamp2":           {"gobvill_hut03"},
	"GoblinCamp2_1":         {"gobvill_hut03"},
	"WoodFarm1":             {"woodfarm_farmhouse1"},
	"WoodVillage1":          {"meadowsvillage_greathall"},
	"WoodVillage2":          {"meadowsvillage_greathall"},
	"Hildir_plainsfortress": {"plainsfortress_Hildir_Start", "plainsfortress_Hildir_Floor0", "plainsfortress_Hildir_Floor1", "plainsfortress_Hildir_Floor2", "plainsfortress_Hildir_Endcap"},
	"FortressRuins":         {"FortressRuins_new01"},
	"AshlandRuins":          {"CharredRuins_new01"},
	"NorthVillage":          {"northvillage_house1"},
}

// room is one row of rooms.tsv with its views.tsv pieces.
type room struct {
	name  string
	pos   [3]float64
	rot   [4]float64
	views []view
}

// view is one piece prefab a room places: its transform under the room,
// and for SnapToGround pieces the height above the ground they settle at.
type view struct {
	prefab   string
	pos      [3]float64
	rot      [4]float64
	scale    [3]float64
	snap     bool
	snapOffs float64
}

// campJobs is one job per generator-built camp whose rooms the tables
// hold.
func campJobs(meshes, dungeon string, mats *render.Manifest) ([]job, error) {
	rooms, err := loadRooms(dungeon)
	if err != nil {
		return nil, err
	}
	var jobs []job
	for name, set := range camps {
		var picked []*room
		for _, rn := range set {
			r, ok := rooms[rn]
			if !ok {
				return nil, fmt.Errorf("%s: room %s is not in %s", name, rn, dungeon)
			}
			picked = append(picked, r)
		}
		jobs = append(jobs, job{kind: "locations", name: name, bytes: int64(len(picked)) << 20, build: func() (*render.Scene, error) {
			return assemble(meshes, mats, picked)
		}})
	}
	return jobs, nil
}

// assemble places the rooms' pieces and the rooms' own geometry. Pieces
// the game drops onto the ground sit at their snap offset over y=0, the
// flat ground the stand-in is drawn on; pieces with no mesh (spawners,
// which are invisible in the game too) are left out.
func assemble(meshes string, mats *render.Manifest, rooms []*room) (*render.Scene, error) {
	s := &render.Scene{}
	lib := vmsh.NewLibrary(meshes)
	origin := rooms[0].pos
	placed := 0
	for _, r := range rooms {
		at := render.Transform{Pos: sub(r.pos, origin), Rot: r.rot, Scale: [3]float64{1, 1, 1}}
		if m, ok := lib.Get(r.name + "~room"); ok {
			s.Add(m, mats, at)
			placed++
		}
		for _, v := range r.views {
			m, ok := lib.Get(v.prefab)
			if !ok {
				continue
			}
			xf := render.Transform{Pos: add(at.Pos, rot(at.Rot, v.pos)), Rot: render.Mul(at.Rot, v.rot), Scale: v.scale}
			if v.snap {
				xf.Pos[1] = v.snapOffs
			}
			s.Add(m, mats, xf)
			placed++
		}
	}
	if placed == 0 {
		return nil, fmt.Errorf("no piece of %s has a mesh", rooms[0].name)
	}
	return s, nil
}

// loadRooms reads rooms.tsv and views.tsv.
func loadRooms(dir string) (map[string]*room, error) {
	rooms := map[string]*room{}
	err := eachRow(filepath.Join(dir, "rooms.tsv"), func(f map[string]string) {
		rooms[f["name"]] = &room{name: f["name"], pos: vec3(f["rootPos"]), rot: quat(f["rootRot"])}
	})
	if err != nil {
		return nil, err
	}
	err = eachRow(filepath.Join(dir, "views.tsv"), func(f map[string]string) {
		r, ok := rooms[f["room"]]
		if !ok || f["activeSelf"] == "0" {
			return
		}
		v := view{prefab: f["prefab"], pos: vec3(f["roomPos"]), rot: quat(f["roomRot"]), scale: vec3(f["scale"])}
		if s := f["snapOffset"]; s != "" {
			v.snap, v.snapOffs = true, num(s)
		}
		r.views = append(r.views, v)
	})
	return rooms, err
}

// eachRow reads a tab-separated table whose first line names the columns
// after a '#'.
func eachRow(path string, fn func(map[string]string)) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cols []string
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if cols == nil {
				cols = strings.Split(strings.TrimSpace(strings.TrimPrefix(line, "#")), "\t")
			}
			continue
		}
		f := strings.Split(line, "\t")
		row := map[string]string{}
		for i, c := range cols {
			if i < len(f) {
				row[c] = f[i]
			}
		}
		fn(row)
	}
	return nil
}

func vec3(s string) [3]float64 {
	var v [3]float64
	for i, p := range strings.SplitN(s, ",", 3) {
		v[i], _ = strconv.ParseFloat(p, 64)
	}
	return v
}

func quat(s string) [4]float64 {
	q := [4]float64{0, 0, 0, 1}
	f := strings.Split(s, ",")
	if len(f) == 4 {
		for i := range q {
			q[i], _ = strconv.ParseFloat(f[i], 64)
		}
	}
	return q
}

func add(a, b [3]float64) [3]float64 { return [3]float64{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func sub(a, b [3]float64) [3]float64 { return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }

// rot is the vector under the quaternion, q v q*.
func rot(q [4]float64, v [3]float64) [3]float64 {
	x, y, z, w := q[0], q[1], q[2], q[3]
	tx := 2 * (y*v[2] - z*v[1])
	ty := 2 * (z*v[0] - x*v[2])
	tz := 2 * (x*v[1] - y*v[0])
	return [3]float64{
		v[0] + w*tx + (y*tz - z*ty),
		v[1] + w*ty + (z*tx - x*tz),
		v[2] + w*tz + (x*ty - y*tx),
	}
}
