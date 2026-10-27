// Package assets loads Valheim's asset bundles together, so that a
// reference from one serialised file into another resolves: a recipe's
// item, an item's icon, a station's name.
package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/unity/serialized"
	"github.com/pietjan/valheim-wiki/internal/unity/unityfs"
)

// Env is every loaded serialised file by its CAB name, with the resource
// files beside them.
type Env struct {
	Files map[string]*serialized.File
	Res   map[string][]byte
	// Order is the CAB names in load order, for a deterministic walk.
	Order     []string
	externals map[string][]string
	trees     map[key]map[string]any
	missing   map[key]bool
}

type key struct {
	cab string
	id  int64
}

// Ref is a resolved PPtr: the file and the object in it.
type Ref struct {
	Cab string
	ID  int64
}

// Load reads bundle files into one environment.
func Load(paths ...string) (*Env, error) {
	e := &Env{
		Files:     map[string]*serialized.File{},
		Res:       map[string][]byte{},
		externals: map[string][]string{},
		trees:     map[key]map[string]any{},
		missing:   map[key]bool{},
	}
	for _, p := range paths {
		a, err := unityfs.Open(p)
		if err != nil {
			return nil, err
		}
		for i := range a.Nodes {
			n := &a.Nodes[i]
			if n.Flags&4 == 0 {
				// .resS holds texture and mesh data; .resource holds
				// audio and video, which nothing here reads.
				if !strings.HasSuffix(n.Path, ".resource") {
					e.Res[n.Path] = n.Data
				}
				continue
			}
			f, err := serialized.Parse(n.Data)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", p, n.Path, err)
			}
			e.Files[n.Path] = f
			e.Order = append(e.Order, n.Path)
			ext := make([]string, len(f.Externals))
			for j, x := range f.Externals {
				ext[j] = x.Path[strings.LastIndex(x.Path, "/")+1:]
			}
			e.externals[n.Path] = ext
		}
	}
	return e, nil
}

// Manifest is bundle name -> the asset paths it holds, from the SoftRef
// manifest: a flat text file with a BOM and CRLF endings.
func Manifest(softRef string) (map[string][]string, error) {
	b, err := os.ReadFile(filepath.Join(softRef, "manifest_extended"))
	if err != nil {
		return nil, err
	}
	held := map[string][]string{}
	bundle := ""
	for _, line := range strings.Split(strings.TrimPrefix(string(b), "\ufeff"), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "  bundle: ") {
			bundle = strings.TrimPrefix(line, "  bundle: ")
		} else if strings.HasPrefix(line, "  path in bundle: ") && bundle != "" {
			held[bundle] = append(held[bundle], strings.TrimPrefix(line, "  path in bundle: "))
		}
	}
	if len(held) == 0 {
		return nil, fmt.Errorf("no bundles listed in the manifest; its format has changed")
	}
	return held, nil
}

// Tree decodes one object, cached. Nil when it does not decode.
func (e *Env) Tree(cab string, id int64) map[string]any {
	k := key{cab, id}
	if t, ok := e.trees[k]; ok {
		return t
	}
	if e.missing[k] {
		return nil
	}
	f := e.Files[cab]
	if f == nil {
		e.missing[k] = true
		return nil
	}
	o := f.Objects[id]
	if o == nil {
		e.missing[k] = true
		return nil
	}
	t, err := o.Tree()
	if err != nil {
		e.missing[k] = true
		return nil
	}
	e.trees[k] = t
	return t
}

// Object is the object table entry, or nil.
func (e *Env) Object(cab string, id int64) *serialized.Object {
	f := e.Files[cab]
	if f == nil {
		return nil
	}
	return f.Objects[id]
}

// Deref resolves a PPtr read out of a tree in cab: {m_FileID, m_PathID}.
func (e *Env) Deref(cab string, ref any) (Ref, bool) {
	m, ok := ref.(map[string]any)
	if !ok {
		return Ref{}, false
	}
	id := AsInt64(m["m_PathID"])
	if id == 0 {
		return Ref{}, false
	}
	fid := int(AsInt32(m["m_FileID"]))
	if fid == 0 {
		return Ref{cab, id}, true
	}
	ext := e.externals[cab]
	if fid > len(ext) {
		return Ref{}, false
	}
	return Ref{ext[fid-1], id}, true
}

// TreeOf is Deref followed by Tree.
func (e *Env) TreeOf(cab string, ref any) (map[string]any, Ref) {
	r, ok := e.Deref(cab, ref)
	if !ok {
		return nil, Ref{}
	}
	return e.Tree(r.Cab, r.ID), r
}

// Name is a referenced object's m_Name, or "".
func (e *Env) Name(cab string, ref any) string {
	t, _ := e.TreeOf(cab, ref)
	return AsString(t["m_Name"])
}

// TopFields is the names at the first level of an object's type tree:
// which component it is, without loading the MonoScript.
func TopFields(f *serialized.File, o *serialized.Object) map[string]bool {
	out := map[string]bool{}
	if o.TypeIndex < 0 || int(o.TypeIndex) >= len(f.Types) {
		return out
	}
	for _, n := range f.Types[o.TypeIndex].Nodes {
		if n.Level == 1 {
			out[n.Name] = true
		}
	}
	return out
}

// Resolver hands back bytes from the resource files, for the texture
// decoder.
func (e *Env) Resolver() func(path string, off uint64, size uint32) ([]byte, error) {
	return func(path string, off uint64, size uint32) ([]byte, error) {
		name := path[strings.LastIndex(path, "/")+1:]
		data, ok := e.Res[name]
		if !ok {
			return nil, fmt.Errorf("no %s loaded", name)
		}
		if off+uint64(size) > uint64(len(data)) {
			return nil, fmt.Errorf("%s: %d+%d beyond %d bytes", name, off, size, len(data))
		}
		return data[off : off+uint64(size)], nil
	}
}

// Typed accessors for tree values, which the type tree hands back as the
// Go type nearest the serialised one.

func AsList(v any) []any         { l, _ := v.([]any); return l }
func AsMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
func AsString(v any) string      { s, _ := v.(string); return s }

func AsInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case uint64:
		return int64(x)
	case int32:
		return int64(x)
	case uint32:
		return int64(x)
	case int16:
		return int64(x)
	case uint16:
		return int64(x)
	case int8:
		return int64(x)
	case uint8:
		return int64(x)
	case bool:
		if x {
			return 1
		}
	}
	return 0
}

func AsInt32(v any) int32 { return int32(AsInt64(v)) }

func AsUint32(v any) uint32 {
	switch x := v.(type) {
	case uint32:
		return x
	case int32:
		return uint32(x)
	}
	return uint32(AsInt64(v))
}

func AsBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	}
	return AsInt64(v) != 0
}

func AsFloat(v any) float64 {
	switch x := v.(type) {
	case float32:
		return float64(x)
	case float64:
		return x
	}
	return float64(AsInt64(v))
}
