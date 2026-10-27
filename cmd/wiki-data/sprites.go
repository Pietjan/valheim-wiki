package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/unity/texture"
)

// The sprites are packed into atlas pages, so writing one is a decode of
// its page and a crop: valheim-forge's extractor, which valheim-npc
// checked against a Python extractor to the pixel.

// renderData is where one sprite sits in its atlas page.
type renderData struct {
	texture  int64
	x, y     float64 // in texture pixels, y up; fractional in the atlas
	w, h     float64
	settings uint32
}

// writeSprites writes every wanted sprite: sprite ref -> the file names
// to write it under (without .png). It reports which names were written.
func (x *X) writeSprites(wanted map[assets.Ref][]string, dir string) (map[string]bool, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	env := x.Env
	written := map[string]bool{}
	for _, cab := range env.Order {
		f := env.Files[cab]
		atlas := map[string]renderData{}
		for _, id := range f.Order {
			o := f.Objects[id]
			if o.ClassID != 687 && o.ClassID != 687078895 {
				continue
			}
			t := env.Tree(cab, id)
			for _, e := range assets.AsList(t["m_RenderDataMap"]) {
				em := assets.AsMap(e)
				atlas[keyOf(assets.AsMap(em["first"]))] = readRenderData(assets.AsMap(em["second"]))
			}
		}
		pages := map[int64]*image.NRGBA{}
		page := func(id int64) (*image.NRGBA, error) {
			if img, ok := pages[id]; ok {
				return img, nil
			}
			t := env.Tree(cab, id)
			if t == nil {
				return nil, fmt.Errorf("no texture %d", id)
			}
			img, err := texture.Decode(t, env.Resolver())
			if err != nil {
				return nil, err
			}
			pages[id] = img
			return img, nil
		}
		for _, id := range f.Order {
			o := f.Objects[id]
			if o.ClassID != 213 {
				continue
			}
			names := wanted[assets.Ref{Cab: cab, ID: id}]
			if len(names) == 0 {
				continue
			}
			t := env.Tree(cab, id)
			if t == nil {
				continue
			}
			rd := readRenderData(assets.AsMap(t["m_RD"]))
			if rd.texture == 0 {
				rd = atlas[keyOf(assets.AsMap(t["m_RenderDataKey"]))]
			}
			if rd.texture == 0 {
				continue
			}
			img, err := page(rd.texture)
			if err != nil {
				return written, fmt.Errorf("sprite %v: %w", names, err)
			}
			crop := cropSprite(img, rd)
			sort.Strings(names)
			for _, name := range names {
				if err := writePNG(filepath.Join(dir, name+".png"), crop); err != nil {
					return written, err
				}
				written[name] = true
			}
		}
	}
	return written, nil
}

func writePNG(path string, img image.Image) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	return png.Encode(fh, img)
}

func readRenderData(rd map[string]any) renderData {
	tex := assets.AsMap(rd["texture"])
	r := assets.AsMap(rd["textureRect"])
	return renderData{
		texture:  assets.AsInt64(tex["m_PathID"]),
		x:        assets.AsFloat(r["x"]),
		y:        assets.AsFloat(r["y"]),
		w:        assets.AsFloat(r["width"]),
		h:        assets.AsFloat(r["height"]),
		settings: assets.AsUint32(rd["settingsRaw"]),
	}
}

// keyOf is a render data key as a string: the GUID's four words and the id.
func keyOf(k map[string]any) string {
	g := assets.AsMap(k["first"])
	return fmt.Sprintf("%v-%v-%v-%v-%v", g["data[0]"], g["data[1]"], g["data[2]"], g["data[3]"], k["second"])
}

// cropSprite cuts the sprite out of its page. The rect is in texture
// coordinates with y up and fractional edges; each edge rounds to the
// nearest pixel, and the page is an image with y down. The packing
// rotation in settingsRaw (bits 2-5) is undone.
func cropSprite(page *image.NRGBA, rd renderData) *image.NRGBA {
	h := page.Rect.Dy()
	x0, x1 := int(math.Round(rd.x)), int(math.Round(rd.x+rd.w))
	y0, y1 := int(math.Round(rd.y)), int(math.Round(rd.y+rd.h))
	r := image.Rect(x0, h-y1, x1, h-y0)
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Rect, page, r.Min, draw.Src)
	switch (rd.settings >> 2) & 0xf {
	case 1:
		out = flipH(out)
	case 2:
		out = flipV(out)
	case 3:
		out = flipH(flipV(out))
	case 4:
		out = rotate90(out)
	}
	return out
}

func flipH(img *image.NRGBA) *image.NRGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out := image.NewNRGBA(img.Rect)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetNRGBA(w-1-x, y, img.NRGBAAt(x, y))
		}
	}
	return out
}

func flipV(img *image.NRGBA) *image.NRGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out := image.NewNRGBA(img.Rect)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetNRGBA(x, h-1-y, img.NRGBAAt(x, y))
		}
	}
	return out
}

func rotate90(img *image.NRGBA) *image.NRGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetNRGBA(y, w-1-x, img.NRGBAAt(x, y))
		}
	}
	return out
}
