package worldmap

import (
	"encoding/json"
	"fmt"
	"image/color"

	"github.com/pietjan/valheim-wiki/internal/worldgen"
)

// Assets is what the game's map is drawn with, read out of the client by
// cmd/unity-minimap and written beside the textures it extracts as
// minimap.json. The browser's painter reads the same file; the palette in it
// replaces Palette on the panel, so the header and the scene's ground agree
// with the game rather than with a guess.
//
// Every number is the game's: the colours and grid from the Minimap
// component, the material's constants from minimap.mat, the sun from the
// EnvMan's daytime settings for one environment. Where a value is not in the
// bundles because it is written into the code, the extractor says so in the
// Notes.
type Assets struct {
	Extracted string `json:"extracted"`
	Source    string `json:"source"`

	// Palette is the biome colours by name, as Minimap.GetPixelColor returns
	// them: components in [0, 1].
	Palette map[string][3]float64 `json:"palette"`

	// Grid is the map texture's size in texels and metres per texel.
	Grid struct {
		TextureSize int     `json:"textureSize"`
		PixelSize   float64 `json:"pixelSize"`
	} `json:"grid"`

	// Material is minimap.mat's colours and floats by property name, without
	// the underscore: forestColor, waterColor, waterColorDeep,
	// waterColorAshlands, waterColorAshlandsDeep, lavaColor1, lavaColor2,
	// lightColor, ambientLightColor, normalWidth, normalIntensity.
	Material struct {
		Colors map[string][3]float64 `json:"colors"`
		Floats map[string]float64    `json:"floats"`
	} `json:"material"`

	// Sun is what the shader's global _SunDir, _SunColor and _AmbientColor
	// hold at noon in one environment: the direction towards the sun, its
	// colour times its intensity, and the ambient light.
	Sun struct {
		Environment string     `json:"environment"`
		Dir         [3]float64 `json:"dir"`
		Color       [3]float64 `json:"color"`
		Ambient     [3]float64 `json:"ambient"`
	} `json:"sun"`

	// Textures is shader property to file name, both without decoration:
	// forest, mountain, water, clouds, space, background, foglayer, lava.
	Textures map[string]string `json:"textures"`

	Notes []string `json:"notes,omitempty"`
}

// biomeNames is the palette's keys, which are the game's enum names.
var biomeNames = map[string]worldgen.Biome{
	"Meadows":     worldgen.BiomeMeadows,
	"Swamp":       worldgen.BiomeSwamp,
	"Mountain":    worldgen.BiomeMountain,
	"BlackForest": worldgen.BiomeBlackForest,
	"Plains":      worldgen.BiomePlains,
	"AshLands":    worldgen.BiomeAshLands,
	"DeepNorth":   worldgen.BiomeDeepNorth,
	"Ocean":       worldgen.BiomeOcean,
	"Mistlands":   worldgen.BiomeMistlands,
}

// ReadAssets parses minimap.json and checks it has what the painter needs.
func ReadAssets(data []byte) (*Assets, error) {
	var a Assets
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("worldmap: minimap.json: %w", err)
	}
	for name := range biomeNames {
		if _, ok := a.Palette[name]; !ok {
			return nil, fmt.Errorf("worldmap: minimap.json has no colour for %s", name)
		}
	}
	for _, tex := range []string{"forest", "mountain", "water", "clouds", "space", "background", "foglayer", "lava"} {
		if a.Textures[tex] == "" {
			return nil, fmt.Errorf("worldmap: minimap.json has no %s texture", tex)
		}
	}
	return &a, nil
}

// ApplyPalette replaces the chosen colours with the game's. Two keep their
// own. The void, because nothing in the game names a colour for no biome at
// all. And the ocean, because the game's is white: its map paints water over
// that colour and never shows it, while the scene draws the sea floor in it,
// and a white floor under deep water is a pale patch that reads as a hole
// beside the darker ground of the biome next door.
func (a *Assets) ApplyPalette() {
	for name, biome := range biomeNames {
		if biome == worldgen.BiomeOcean {
			continue
		}
		c := a.Palette[name]
		Palette[biome] = color.RGBA{toByte(c[0]), toByte(c[1]), toByte(c[2]), 255}
	}
}
