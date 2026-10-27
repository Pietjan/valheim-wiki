package worldmap

import (
	"encoding/json"
	"image/color"
	"testing"

	"github.com/pietjan/valheim-wiki/internal/worldgen"
)

// A minimap.json with everything the painter needs.
func fullAssets() map[string]any {
	palette := map[string][3]float64{}
	for name := range biomeNames {
		palette[name] = [3]float64{0.5, 0.5, 0.5}
	}
	palette["Meadows"] = [3]float64{0.5725, 0.6551, 0.3605}
	textures := map[string]string{}
	for _, t := range []string{"forest", "mountain", "water", "clouds", "space", "background", "foglayer", "lava"} {
		textures[t] = t + ".png"
	}
	return map[string]any{"palette": palette, "textures": textures}
}

func TestReadAssetsWantsEveryBiomeAndTexture(t *testing.T) {
	full, _ := json.Marshal(fullAssets())
	if _, err := ReadAssets(full); err != nil {
		t.Fatalf("a complete file is refused: %v", err)
	}

	short := fullAssets()
	delete(short["palette"].(map[string][3]float64), "Mistlands")
	data, _ := json.Marshal(short)
	if _, err := ReadAssets(data); err == nil {
		t.Error("a file without the mistlands' colour is accepted")
	}

	short = fullAssets()
	delete(short["textures"].(map[string]string), "forest")
	data, _ = json.Marshal(short)
	if _, err := ReadAssets(data); err == nil {
		t.Error("a file without the forest texture is accepted")
	}

	if _, err := ReadAssets([]byte("not json")); err == nil {
		t.Error("garbage is accepted")
	}
}

// The game's colours replace the chosen ones, and the void keeps its own.
func TestApplyPaletteTakesTheGamesColours(t *testing.T) {
	before := Palette[worldgen.BiomeMeadows]
	void := Palette[worldgen.BiomeNone]
	ocean := Palette[worldgen.BiomeOcean]
	t.Cleanup(func() { Palette[worldgen.BiomeMeadows] = before })

	full, _ := json.Marshal(fullAssets())
	a, err := ReadAssets(full)
	if err != nil {
		t.Fatal(err)
	}
	a.ApplyPalette()

	want := color.RGBA{146, 167, 92, 255}
	if got := Palette[worldgen.BiomeMeadows]; got != want {
		t.Errorf("meadows are %v after the game's palette, want %v", got, want)
	}
	if Palette[worldgen.BiomeNone] != void {
		t.Error("the void's colour was replaced; the game has none for it")
	}
	if Palette[worldgen.BiomeOcean] != ocean {
		t.Error("the ocean's colour was replaced; the game's is white and never drawn")
	}
}
