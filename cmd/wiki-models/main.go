// Command wiki-models packs the models the 3D view draws: the meshes of
// everything the zone build can stand — the vegetation rules' prefabs and
// the locations' whole-site models — from valheim-npc's mesh extraction,
// with the materials they paint with and their textures shrunk for the
// web.
//
//	wiki-models -meshes ../valheim-npc/meshes -out web/static/models
//
// Writes <out>/<prefab>.bin (the VMSH file as extracted for a vegetation
// prefab; a site packed as VMSQ, see sites.go), <out>/textures/
// (each texture used, downscaled to -max a side; PNG where a material
// cuts it out, with its transparent texels bled so the filtering does not
// fringe, JPEG otherwise), <out>/materials.json (the entries those meshes
// use, with the texture file names rewritten) and <out>/index.json (the
// models packed, with their sizes).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/scene"
	"github.com/pietjan/valheim-wiki/internal/vmsh"
	"github.com/pietjan/valheim-wiki/internal/worldgen"
)

type material struct {
	Texture    string     `json:"texture,omitempty"`
	Scale      [2]float64 `json:"scale,omitempty"`
	Offset     [2]float64 `json:"offset,omitempty"`
	Color      [4]float64 `json:"color"`
	Cutoff     float64    `json:"cutoff,omitempty"`
	Cutout     bool       `json:"cutout,omitempty"`
	Shader     string     `json:"shader,omitempty"`
	Glossiness float64    `json:"glossiness,omitempty"`
}

// entry is a materials.json entry as the extractor writes it; only what
// the viewer reads is kept.
type entry struct {
	Texture    string     `json:"texture"`
	Scale      [2]float64 `json:"scale"`
	Offset     [2]float64 `json:"offset"`
	Color      [4]float64 `json:"color"`
	Cutoff     float64    `json:"cutoff"`
	Cutout     bool       `json:"cutout"`
	Shader     string     `json:"shader"`
	Glossiness float64    `json:"glossiness"`
}

func main() {
	meshes := flag.String("meshes", "../valheim-npc/meshes", "valheim-npc's meshes directory")
	out := flag.String("out", "web/static/models", "where to write")
	maxSide := flag.Int("max", 512, "a texture's longest side, pixels")
	sites := flag.Bool("sites", true, "include the locations' whole-site models, packed")
	flag.Parse()

	manifest := map[string]entry{}
	raw, err := os.ReadFile(filepath.Join(*meshes, "materials.json"))
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		log.Fatal("materials.json: ", err)
	}

	var names []string
	names = append(names, scene.VegetationNames()...)
	if *sites {
		seen := map[string]bool{}
		for _, r := range worldgen.DefaultLocationRules() {
			if !seen[r.Prefab] && r.Quantity > 0 {
				seen[r.Prefab] = true
				names = append(names, r.Prefab+scene.SiteSuffix)
			}
		}
	}
	sort.Strings(names)

	if err := os.MkdirAll(filepath.Join(*out, "textures"), 0o755); err != nil {
		log.Fatal(err)
	}
	index := map[string]int64{}
	used := map[string]*material{} // material name → entry written
	textures := map[string]bool{}  // texture file → needs alpha (cutout)
	var total int64
	missing, over := 0, 0
	for _, name := range names {
		src := filepath.Join(*meshes, name+".bin")
		m, err := vmsh.Load(src)
		if err != nil {
			missing++
			continue
		}
		if len(m.Idx) == 0 {
			continue
		}
		var data []byte
		if strings.HasSuffix(name, scene.SiteSuffix) {
			if data = packSite(m); data == nil {
				over++
				log.Printf("%s: over the cap at every grid, left out", name)
				continue
			}
		} else if data, err = os.ReadFile(src); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(*out, name+".bin"), data, 0o644); err != nil {
			log.Fatal(err)
		}
		index[name] = int64(len(data))
		total += int64(len(data))
		for _, r := range m.Runs {
			base := strings.SplitN(r.Material, "#", 2)[0]
			if _, ok := used[base]; ok {
				continue
			}
			e, ok := manifest[base]
			if !ok {
				continue
			}
			mat := &material{Scale: e.Scale, Offset: e.Offset, Color: e.Color, Cutoff: e.Cutoff, Cutout: e.Cutout, Shader: e.Shader, Glossiness: e.Glossiness}
			if e.Texture != "" {
				mat.Texture = e.Texture
				if e.Cutout {
					textures[e.Texture] = true
				} else if _, ok := textures[e.Texture]; !ok {
					textures[e.Texture] = false
				}
			}
			used[base] = mat
		}
	}

	// The textures, each once, named by the file the viewer fetches.
	outName := map[string]string{}
	var texBytes int64
	for file, alpha := range textures {
		img, err := decode(filepath.Join(*meshes, "textures", file))
		if err != nil {
			log.Printf("texture %s: %v", file, err)
			continue
		}
		if alpha {
			bleed(img, 8)
		}
		img = shrink(img, *maxSide)
		stem := strings.TrimSuffix(file, filepath.Ext(file))
		var dst string
		if alpha {
			dst = stem + ".png"
		} else {
			dst = stem + ".jpg"
		}
		f, err := os.Create(filepath.Join(*out, "textures", dst))
		if err != nil {
			log.Fatal(err)
		}
		if alpha {
			err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(f, img)
		} else {
			err = jpeg.Encode(f, img, &jpeg.Options{Quality: 82})
		}
		f.Close()
		if err != nil {
			log.Fatal(err)
		}
		st, _ := os.Stat(filepath.Join(*out, "textures", dst))
		texBytes += st.Size()
		outName[file] = dst
	}
	for _, mat := range used {
		if mat.Texture != "" {
			mat.Texture = outName[mat.Texture]
		}
	}

	write := func(name string, v any) {
		b, err := json.MarshalIndent(v, "", " ")
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(*out, name), b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	write("materials.json", used)
	write("index.json", map[string]any{"models": index, "textures": len(outName)})
	fmt.Printf("%d models (%.1f MB), %d textures (%.1f MB), %d materials; %d prefabs without a mesh, %d sites over the cap\n",
		len(index), float64(total)/1e6, len(outName), float64(texBytes)/1e6, len(used), missing, over)
}

func decode(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	return out, nil
}

// bleed copies colour from opaque texels into transparent neighbours,
// one ring at a time for `rings` rings, so filtering and mipmapping do
// not drag the artist's leftover colour into a leaf's edge.
func bleed(img *image.NRGBA, rings int) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	filled := make([]bool, w*h)
	for i := 0; i < w*h; i++ {
		filled[i] = img.Pix[4*i+3] > 0
	}
	for ring := 0; ring < rings; ring++ {
		next := make([]bool, w*h)
		copy(next, filled)
		changed := false
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := y*w + x
				if filled[i] {
					continue
				}
				var r, g, b, n int
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						xx, yy := x+dx, y+dy
						if xx < 0 || yy < 0 || xx >= w || yy >= h {
							continue
						}
						j := yy*w + xx
						if !filled[j] {
							continue
						}
						r += int(img.Pix[4*j])
						g += int(img.Pix[4*j+1])
						b += int(img.Pix[4*j+2])
						n++
					}
				}
				if n == 0 {
					continue
				}
				img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2] = uint8(r/n), uint8(g/n), uint8(b/n)
				next[i] = true
				changed = true
			}
		}
		filled = next
		if !changed {
			break
		}
	}
}

// shrink downscales by an integer factor with a box filter until the
// longest side is within max. Alpha is averaged too, and the colour is
// weighted by it so transparent texels do not darken an edge.
func shrink(img *image.NRGBA, max int) *image.NRGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	f := 1
	for (w+f-1)/f > max || (h+f-1)/f > max {
		f++
	}
	if f == 1 {
		return img
	}
	nw, nh := (w+f-1)/f, (h+f-1)/f
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			var r, g, b, a, n, wsum float64
			for dy := 0; dy < f; dy++ {
				for dx := 0; dx < f; dx++ {
					sx, sy := x*f+dx, y*f+dy
					if sx >= w || sy >= h {
						continue
					}
					i := 4 * (sy*w + sx)
					al := float64(img.Pix[i+3])
					wt := al + 1 // a whisper of weight for fully transparent texels, which are bled
					r += float64(img.Pix[i]) * wt
					g += float64(img.Pix[i+1]) * wt
					b += float64(img.Pix[i+2]) * wt
					wsum += wt
					a += al
					n++
				}
			}
			if n == 0 {
				continue
			}
			o := 4 * (y*nw + x)
			out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = uint8(r/wsum), uint8(g/wsum), uint8(b/wsum), uint8(a/n)
		}
	}
	return out
}
