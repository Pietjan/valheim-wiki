package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Material is how one of a mesh's material runs is painted.
type Material struct {
	Name string
	// Albedo is the base texture, nil for a flat colour.
	Albedo *Texture
	// Color multiplies the albedo in linear light, or is the flat colour
	// when there is no texture.
	Color [3]float32
	// Cutout materials alpha-test against Cutoff and draw both faces: a
	// leaf card is seen from either side.
	Cutout   bool
	Cutoff   float32
	TwoSided bool
	// Unlit materials are the game's glow shaders: drawn as their colour,
	// without the sun.
	Unlit bool
	// Skip marks what the game does not draw as a surface: teleport quads,
	// particle sheets, beams of light, blockers.
	Skip bool
	// Scale and Offset tile the uv before sampling, as Unity's
	// mainTextureScale and mainTextureOffset do.
	Scale, Offset [2]float32
}

// manifestEntry is one row of materials.json as cmd/unity-meshes wrote it.
type manifestEntry struct {
	Texture string     `json:"texture"`
	Scale   [2]float32 `json:"scale"`
	Offset  [2]float32 `json:"offset"`
	Color   [4]float32 `json:"color"`
	Cutoff  float32    `json:"cutoff"`
	Cutout  bool       `json:"cutout"`
	Shader  string     `json:"shader"`
}

// Manifest is materials.json plus the textures beside it, read on demand
// and shared by every render.
type Manifest struct {
	dir     string
	entries map[string]manifestEntry

	mu       sync.Mutex
	textures map[string]*textureSlot
	mats     map[string]*Material
}

type textureSlot struct {
	once sync.Once
	tex  *Texture
}

// LoadManifest reads <meshes>/materials.json.
func LoadManifest(meshDir string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(meshDir, "materials.json"))
	if err != nil {
		return nil, err
	}
	m := &Manifest{dir: meshDir, textures: map[string]*textureSlot{}, mats: map[string]*Material{}}
	if err := json.Unmarshal(b, &m.entries); err != nil {
		return nil, fmt.Errorf("materials.json: %w", err)
	}
	return m, nil
}

// The scene's name-based tones for a material without an entry, sRGB
// hex as scene.js's MATERIAL_TONES lists them, in order of precedence.
var tones = []struct {
	re  *regexp.Regexp
	hex uint32
}{
	{regexp.MustCompile(`(?i)ashlandstree|ashlandsbranch|ashlandslog|charred|ember|burnt`), 0x3a3330},
	{regexp.MustCompile(`(?i)leaf|leaves|needle|foliage|branch|shrub|bush|fern|grass|vine|ivy|moss`), 0x4f8f4a},
	{regexp.MustCompile(`(?i)straw|thatch|\breed|\bhay\b`), 0xc4a34a},
	{regexp.MustCompile(`(?i)bark|trunk|\bstem|wood|plank|timber|log|stump|beam|stake`), 0x7a5a3a},
	{regexp.MustCompile(`(?i)metal|iron|bronze|copper|\btin\b|silver|steel|flametal`), 0x8f8f96},
	{regexp.MustCompile(`(?i)rock|stone|cliff|marble|granite|boulder|gravel|slate`), 0x6e6a63},
	{regexp.MustCompile(`(?i)\bice\b|\bice[0-9_]|snow|frost`), 0x9fc4d6},
	{regexp.MustCompile(`(?i)cloth|banner|linen|\bfur\b|hide|leather|carpet|\brug\b`), 0xa87f5c},
}

// siteTone is the scene's colour for a material no rule names: a warm stone.
const siteTone = 0x9a8f80

// skipShaders are shaders that draw no lit surface: particles, beams,
// masks, and the decals that would only fight the floor under them.
var skipShaders = regexp.MustCompile(`^(Unlit/BeaconBeam|Custom/LitParticles|Custom/WaterMask|Custom/InteriorSide|Custom/Decal|Sprites/Default|Particles/|Legacy Shaders/Particles|Lux Lit Particles)`)

// Material is the paint for a run's material name. The extractor splits a
// conifer's material into name#foliage and name#trunk; the entry is under
// the name before the '#'.
func (m *Manifest) Material(name string) *Material {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mat, ok := m.mats[name]; ok {
		return mat
	}
	mat := m.build(name)
	m.mats[name] = mat
	return mat
}

func (m *Manifest) build(name string) *Material {
	mat := &Material{Name: name, Scale: [2]float32{1, 1}, Cutoff: 0.5}
	key := name
	if i := strings.IndexByte(key, '#'); i >= 0 {
		key = key[:i]
	}
	if strings.HasSuffix(key, "_portal") {
		mat.Skip = true
		return mat
	}
	e, ok := m.entries[key]
	if !ok {
		hex := uint32(siteTone)
		for _, t := range tones {
			if t.re.MatchString(key) {
				hex = t.hex
				break
			}
		}
		mat.Color = [3]float32{srgbToLinear[hex>>16&255], srgbToLinear[hex>>8&255], srgbToLinear[hex&255]}
		return mat
	}
	if skipShaders.MatchString(e.Shader) {
		mat.Skip = true
		return mat
	}
	mat.Color = [3]float32{e.Color[0], e.Color[1], e.Color[2]}
	mat.Scale, mat.Offset = e.Scale, e.Offset
	mat.Cutout, mat.Cutoff = e.Cutout, e.Cutoff
	mat.TwoSided = e.Cutout || e.Shader == "Standard TwoSided"
	mat.Unlit = strings.HasPrefix(e.Shader, "Unlit/")
	if e.Texture != "" {
		mat.Albedo = m.texture(e.Texture)
	} else if e.Color[3] < 0.5 {
		// A flat see-through colour: crystal windows, blocker walls. The
		// game blends them; the icon reads the same without.
		mat.Skip = true
	}
	return mat
}

// texture reads one PNG once, whichever render asks first; a file that
// fails to read paints as the material's colour.
func (m *Manifest) texture(file string) *Texture {
	slot, ok := m.textures[file]
	if !ok {
		slot = &textureSlot{}
		m.textures[file] = slot
	}
	m.mu.Unlock()
	slot.once.Do(func() {
		t, err := LoadTexture(filepath.Join(m.dir, "textures", file))
		if err != nil {
			fmt.Fprintf(os.Stderr, "wiki-icons: %v\n", err)
			return
		}
		slot.tex = t
	})
	m.mu.Lock()
	return slot.tex
}
