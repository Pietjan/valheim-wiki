package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// X is the extraction context every extractor gets: the loaded bundles,
// the English strings, an index of root prefabs, and the DB being filled.
type X struct {
	Env     *assets.Env
	DB      *model.DB
	Strings map[string]string // localization key (without $) -> English
	// Roots is every fatherless GameObject by name: prefab name -> where.
	// A name held by several files keeps the first loaded (the gameplay
	// bundle loads before the soft-referenced location bundles).
	Roots map[string]assets.Ref
	// RootsAll keeps every root of a name, in load order.
	RootsAll map[string][]assets.Ref
	// Owners maps every soft-referenced asset path to its bundle.
	SoftPaths map[string]string
	// Held is bundle -> asset paths, from manifest_extended.
	Held    map[string][]string
	SoftRef string
	IconDir string
	// Sprites wanted by the icon pass: kind -> name -> sprite ref.
	sprites map[string]map[string]assets.Ref
	// names overrides from names.tsv: prefab -> (token, english).
	names map[string]nameRow
}

type nameRow struct{ token, english string }

//go:embed names.tsv
var namesTSV string

// Bundles the wiki needs beyond what the manifest paths pick: the system
// prefabs live in d59cfac next to _GameMain, found by path below.
func wantBundle(p string) bool {
	switch {
	case strings.HasSuffix(p, ".prefab") && strings.Contains(p, "/Items/"),
		strings.HasSuffix(p, ".asset") && strings.Contains(p, "/Recipes/"),
		strings.HasSuffix(p, ".prefab") && strings.Contains(p, "/Pieces/"),
		strings.HasSuffix(p, ".prefab") && strings.Contains(p, "/Characters/"),
		strings.HasSuffix(p, ".asset") && strings.Contains(p, "/StatusEffects/"),
		strings.Contains(p, "/_icons/"),
		strings.HasSuffix(p, "/_GameMain.prefab"),
		strings.HasSuffix(p, "/_ZoneCtrl.prefab"),
		strings.Contains(p, "/SpawnLists/"),
		strings.Contains(p, "/LocationLists/"),
		strings.Contains(p, "/AltBiomeLists/"),
		strings.Contains(p, "/RoomLists/"),
		strings.HasSuffix(p, "UIAtlas.spriteatlas"),
		strings.Contains(p, "/Props/"),
		strings.Contains(p, "/Ships/"),
		strings.HasSuffix(p, "/Player.prefab"):
		return true
	}
	return false
}

// open loads the bundles and the strings.
func open(softRef, dataDir string, listOnly bool) (*X, error) {
	held, err := assets.Manifest(softRef)
	if err != nil {
		return nil, err
	}
	soft, err := softManifest(softRef)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for b, paths := range held {
		for _, p := range paths {
			if wantBundle(p) {
				want[b] = true
			}
		}
	}
	var first, later []string
	for b := range want {
		first = append(first, b)
	}
	sort.Strings(first)
	// The soft-referenced location and room prefabs: one small bundle
	// each, loaded after the gameplay bundles so their duplicates of a
	// name never win.
	seen := map[string]bool{}
	for p, b := range soft {
		if (strings.Contains(p, "/Locations/") || strings.Contains(p, "/Rooms/")) && !want[b] && !seen[b] {
			seen[b] = true
			later = append(later, b)
		}
	}
	sort.Strings(later)
	fmt.Printf("==> loading %d gameplay bundle(s) %v and %d location/room bundle(s)\n", len(first), first, len(later))
	if listOnly {
		return nil, nil
	}
	var paths []string
	for _, b := range append(first, later...) {
		paths = append(paths, filepath.Join(softRef, "Bundles", b))
	}
	debug.SetGCPercent(40)
	env, err := assets.Load(paths...)
	if err != nil {
		return nil, err
	}
	debug.FreeOSMemory()
	strs, err := localization(filepath.Join(dataDir, "resources.assets"))
	if err != nil {
		return nil, err
	}
	x := &X{
		Env: env, DB: model.New(), Strings: strs, Held: held, SoftPaths: soft, SoftRef: softRef,
		Roots: map[string]assets.Ref{}, RootsAll: map[string][]assets.Ref{},
		sprites: map[string]map[string]assets.Ref{}, names: map[string]nameRow{},
	}
	x.indexRoots()
	x.readNames()
	return x, nil
}

// softManifest reads the "asset locations:" section of SoftRef/manifest:
// path in bundle -> bundle.
func softManifest(softRef string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(softRef, "manifest"))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	bundle := ""
	for _, line := range strings.Split(strings.TrimPrefix(string(b), "\ufeff"), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "  bundle: ") {
			bundle = strings.TrimPrefix(line, "  bundle: ")
		} else if strings.HasPrefix(line, "  path in bundle: ") && bundle != "" {
			out[strings.TrimPrefix(line, "  path in bundle: ")] = bundle
		}
	}
	return out, nil
}

// indexRoots finds every fatherless GameObject.
func (x *X) indexRoots() {
	for _, cab := range x.Env.Order {
		f := x.Env.Files[cab]
		for _, id := range f.Order {
			o := f.Objects[id]
			if o.ClassID != 4 { // Transform
				continue
			}
			t := x.Env.Tree(cab, id)
			if t == nil {
				continue
			}
			if assets.AsInt64(assets.AsMap(t["m_Father"])["m_PathID"]) != 0 {
				continue
			}
			g, ref := x.Env.TreeOf(cab, t["m_GameObject"])
			if g == nil {
				continue
			}
			name := assets.AsString(g["m_Name"])
			if name == "" {
				continue
			}
			if _, ok := x.Roots[name]; !ok {
				x.Roots[name] = ref
			}
			x.RootsAll[name] = append(x.RootsAll[name], ref)
		}
	}
}

// readNames loads the hand-kept name table.
func (x *X) readNames() {
	sc := bufio.NewScanner(strings.NewReader(namesTSV))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		x.names[f[0]] = nameRow{f[1], f[2]}
	}
}

// Text localizes a string: every $token becomes English.
func (x *X) Text(s string) string { return strings.TrimSpace(localize(x.Strings, s)) }

// NameOf is the English name for a prefab: the hand-kept table first,
// then the token given, then the prefab name spaced out.
func (x *X) NameOf(prefab, token string) string {
	if r, ok := x.names[prefab]; ok && r.english != "" {
		return r.english
	}
	if token != "" {
		if t := x.Text(token); t != "" && !strings.HasPrefix(t, "$") {
			return t
		}
	}
	return prettyName(prefab)
}

// prettyName turns "Pickable_SeedCarrot" into "Pickable Seed Carrot".
func prettyName(s string) string {
	var b strings.Builder
	prev := rune(0)
	for i, r := range s {
		switch {
		case r == '_':
			b.WriteByte(' ')
		case i > 0 && r >= 'A' && r <= 'Z' && prev >= 'a' && prev <= 'z':
			b.WriteByte(' ')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
		prev = r
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Tree is the decoded object behind a reference.
func (x *X) Tree(r assets.Ref) map[string]any { return x.Env.Tree(r.Cab, r.ID) }

// Deref resolves a PPtr read in cab.
func (x *X) Deref(cab string, ref any) (assets.Ref, bool) { return x.Env.Deref(cab, ref) }

// GameObjectName is the m_Name of the GameObject a PPtr points at, or of
// the GameObject owning the component it points at.
func (x *X) GameObjectName(cab string, ref any) string {
	t, r := x.Env.TreeOf(cab, ref)
	if t == nil {
		return ""
	}
	if n, ok := t["m_Name"]; ok && x.Env.Object(r.Cab, r.ID).ClassID == 1 {
		return assets.AsString(n)
	}
	if g, ok := t["m_GameObject"]; ok {
		return x.Env.Name(r.Cab, g)
	}
	return assets.AsString(t["m_Name"])
}

// Component is one component on a GameObject.
type Component struct {
	Ref     assets.Ref
	ClassID int32
	Tree    map[string]any
}

// Components lists a GameObject's components with their decoded trees.
func (x *X) Components(g assets.Ref) []Component {
	t := x.Tree(g)
	if t == nil {
		return nil
	}
	var out []Component
	for _, c := range assets.AsList(t["m_Component"]) {
		m := assets.AsMap(c)
		ptr, ok := m["component"]
		if !ok {
			ptr = c
		}
		r, ok := x.Deref(g.Cab, ptr)
		if !ok {
			continue
		}
		o := x.Env.Object(r.Cab, r.ID)
		if o == nil {
			continue
		}
		ct := x.Tree(r)
		if ct == nil {
			continue
		}
		out = append(out, Component{r, o.ClassID, ct})
	}
	return out
}

// Has is whether a tree has every one of the fields.
func Has(t map[string]any, fields ...string) bool {
	for _, f := range fields {
		if _, ok := t[f]; !ok {
			return false
		}
	}
	return true
}

// Find is the first MonoBehaviour on a GameObject with all the fields.
func (x *X) Find(g assets.Ref, fields ...string) (Component, bool) {
	for _, c := range x.Components(g) {
		if c.ClassID == 114 && Has(c.Tree, fields...) {
			return c, true
		}
	}
	return Component{}, false
}

// Children are a GameObject's child GameObjects, in transform order.
func (x *X) Children(g assets.Ref) []assets.Ref {
	for _, c := range x.Components(g) {
		if c.ClassID != 4 && c.ClassID != 224 { // Transform, RectTransform
			continue
		}
		var out []assets.Ref
		for _, ch := range assets.AsList(c.Tree["m_Children"]) {
			ct, _ := x.Env.TreeOf(c.Ref.Cab, ch)
			if ct == nil {
				continue
			}
			if cg, ok := x.Deref(c.Ref.Cab, ct["m_GameObject"]); ok {
				out = append(out, cg)
			}
		}
		return out
	}
	return nil
}

// Active is a GameObject's m_IsActive.
func (x *X) Active(g assets.Ref) bool {
	t := x.Tree(g)
	return t != nil && assets.AsBool(t["m_IsActive"])
}

// Name is a GameObject's m_Name.
func (x *X) Name(g assets.Ref) string { return assets.AsString(x.Tree(g)["m_Name"]) }

// Walk visits a GameObject and every descendant, depth first; fn returns
// false to skip a subtree. Inactive objects are visited (the caller
// decides; a RandomSpawn child starts inactive).
func (x *X) Walk(g assets.Ref, fn func(depth int, g assets.Ref) bool) {
	var rec func(assets.Ref, int)
	rec = func(g assets.Ref, d int) {
		if !fn(d, g) {
			return
		}
		for _, c := range x.Children(g) {
			rec(c, d+1)
		}
	}
	rec(g, 0)
}

// EachMono calls fn for every MonoBehaviour in every loaded file whose
// top-level fields include all of fields. Trees are decoded on demand.
func (x *X) EachMono(fields []string, fn func(cab string, id int64, t map[string]any)) {
	for _, cab := range x.Env.Order {
		f := x.Env.Files[cab]
		for _, id := range f.Order {
			o := f.Objects[id]
			if o.ClassID != 114 {
				continue
			}
			top := assets.TopFields(f, o)
			ok := true
			for _, fl := range fields {
				if !top[fl] {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			if t := x.Env.Tree(cab, id); t != nil {
				fn(cab, id, t)
			}
		}
	}
}

// Owner is the prefab-root name a component's GameObject belongs to and
// the GameObject itself.
func (x *X) Owner(cab string, t map[string]any) (root string, g assets.Ref) {
	g, ok := x.Deref(cab, t["m_GameObject"])
	if !ok {
		return "", g
	}
	return x.RootName(g), g
}

// RootName walks the transform chain up to the fatherless GameObject.
func (x *X) RootName(g assets.Ref) string {
	for i := 0; i < 64; i++ {
		var father assets.Ref
		found := false
		for _, c := range x.Components(g) {
			if c.ClassID == 4 || c.ClassID == 224 {
				ft, fr := x.Env.TreeOf(c.Ref.Cab, c.Tree["m_Father"])
				if ft == nil {
					return x.Name(g)
				}
				if fg, ok := x.Deref(fr.Cab, ft["m_GameObject"]); ok {
					father, found = fg, true
				}
				break
			}
		}
		if !found {
			return x.Name(g)
		}
		g = father
	}
	return x.Name(g)
}

// F, I, B, S read typed fields with defaults for missing ones.
func F(t map[string]any, k string) float64 { return assets.AsFloat(t[k]) }
func I(t map[string]any, k string) int     { return int(assets.AsInt64(t[k])) }
func B(t map[string]any, k string) bool    { return assets.AsBool(t[k]) }
func S(t map[string]any, k string) string  { return assets.AsString(t[k]) }
func L(t map[string]any, k string) []any   { return assets.AsList(t[k]) }
func M(t map[string]any, k string) map[string]any {
	return assets.AsMap(t[k])
}

// Strs is a list of strings.
func Strs(v any) []string {
	var out []string
	for _, s := range assets.AsList(v) {
		if str := assets.AsString(s); str != "" {
			out = append(out, str)
		}
	}
	return out
}

// Damage reads a HitData.DamageTypes struct, non-zero entries only.
func Damage(m map[string]any) model.Damage {
	out := model.Damage{}
	for _, k := range model.DamageTypes {
		if v := F(m, "m_"+k); v != 0 {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// DamageMods reads a HitData.DamageModifiers struct (one enum per type)
// keeping only the entries that are not Normal.
func DamageMods(m map[string]any) []model.DamageMod {
	var out []model.DamageMod
	for _, k := range model.DamageTypes[1:] {
		v := I(m, "m_"+k)
		if v > 0 && v < len(model.DamageModifierNames) {
			out = append(out, model.DamageMod{Type: k, Modifier: model.DamageModifierNames[v]})
		}
	}
	return out
}

// DamageModList reads a List<HitData.DamageModPair>.
func DamageModList(v any) []model.DamageMod {
	var out []model.DamageMod
	for _, e := range assets.AsList(v) {
		m := assets.AsMap(e)
		typ := damageTypeName(int64(I(m, "m_type")))
		mod := I(m, "m_modifier")
		if typ == "" || mod <= 0 || mod >= len(model.DamageModifierNames) {
			continue
		}
		out = append(out, model.DamageMod{Type: typ, Modifier: model.DamageModifierNames[mod]})
	}
	return out
}

func damageTypeName(bits int64) string {
	switch bits {
	case 1:
		return "blunt"
	case 2:
		return "slash"
	case 4:
		return "pierce"
	case 8:
		return "chop"
	case 0x10:
		return "pickaxe"
	case 0x20:
		return "fire"
	case 0x40:
		return "frost"
	case 0x80:
		return "lightning"
	case 0x100:
		return "poison"
	case 0x200:
		return "spirit"
	case 0x400:
		return "damage"
	case 0x1F:
		return "physical"
	case 0x3E0, 0xE0:
		return "elemental"
	}
	return ""
}

// DropTable reads a DropTable struct in cab.
func (x *X) DropTable(cab string, m map[string]any) *model.DropTable {
	if m == nil {
		return nil
	}
	t := &model.DropTable{Min: I(m, "m_dropMin"), Max: I(m, "m_dropMax"), Chance: F(m, "m_dropChance"), OneOfEach: B(m, "m_oneOfEach")}
	for _, d := range L(m, "m_drops") {
		dm := assets.AsMap(d)
		item := x.GameObjectName(cab, dm["m_item"])
		if item == "" {
			continue
		}
		t.Drops = append(t.Drops, model.DropEntry{Item: item, Min: I(dm, "m_stackMin"), Max: I(dm, "m_stackMax"), Weight: F(dm, "m_weight"), DontScale: B(dm, "m_dontScale")})
	}
	if len(t.Drops) == 0 {
		return nil
	}
	return t
}

// Requirements reads a Piece.Requirement[].
func (x *X) Requirements(cab string, v any) []model.Requirement {
	var out []model.Requirement
	for _, r := range assets.AsList(v) {
		m := assets.AsMap(r)
		item := x.GameObjectName(cab, m["m_resItem"])
		if item == "" {
			continue
		}
		out = append(out, model.Requirement{Item: item, Amount: I(m, "m_amount"), PerLevel: I(m, "m_amountPerLevel"), Recover: B(m, "m_recover"), Upgrader: B(m, "m_upgraderResource")})
	}
	return out
}

// Biomes expands a Heightmap.Biome mask.
func Biomes(mask int) []string { return model.BiomesOf(mask) }

// BiomeArea names Heightmap.BiomeArea.
func BiomeArea(v int) string {
	switch v {
	case 1:
		return "Edge"
	case 2:
		return "Median"
	case 3:
		return "Everything"
	}
	return ""
}

// WantSprite asks the icon pass to write a sprite as
// assets/icons/<kind>/<name>.png.
func (x *X) WantSprite(kind, name string, ref assets.Ref) {
	if ref.ID == 0 || name == "" {
		return
	}
	if x.sprites[kind] == nil {
		x.sprites[kind] = map[string]assets.Ref{}
	}
	x.sprites[kind][name] = ref
}

// unityVersion is the engine version the bundles were built with.
func (x *X) unityVersion() string {
	for _, cab := range x.Env.Order {
		return x.Env.Files[cab].UnityVersion
	}
	return ""
}

// probe dumps every component on the named prefabs or assets.
func (x *X) probe(names map[string]bool, out string) error {
	fh, err := os.Create(out)
	if err != nil {
		return err
	}
	defer fh.Close()
	enc := json.NewEncoder(fh)
	enc.SetIndent("", " ")
	for name := range names {
		if g, ok := x.Roots[name]; ok {
			fmt.Fprintf(fh, "### prefab %s (%s %d)\n", name, g.Cab, g.ID)
			x.Walk(g, func(d int, g assets.Ref) bool {
				fmt.Fprintf(fh, "%s- %s (active %v)\n", strings.Repeat("  ", d), x.Name(g), x.Active(g))
				for _, c := range x.Components(g) {
					if c.ClassID != 114 {
						continue
					}
					fmt.Fprintf(fh, "%s  component %d:\n", strings.Repeat("  ", d), c.Ref.ID)
					enc.Encode(c.Tree)
				}
				return true
			})
			continue
		}
		// An asset (Recipe, StatusEffect) by m_Name.
		found := false
		for _, cab := range x.Env.Order {
			f := x.Env.Files[cab]
			for _, id := range f.Order {
				if f.Objects[id].ClassID != 114 {
					continue
				}
				t := x.Env.Tree(cab, id)
				if t != nil && S(t, "m_Name") == name {
					fmt.Fprintf(fh, "### asset %s (%s %d)\n", name, cab, id)
					enc.Encode(t)
					found = true
				}
			}
		}
		if !found {
			fmt.Fprintf(fh, "### %s: not found\n", name)
		}
	}
	return nil
}
