package site

import (
	"strings"
	"unicode"
)

// Slug is a URL segment for a name: lower case ASCII words joined by
// hyphens. "Bronze Sword" -> "bronze-sword"; "Yagluth's Thing" ->
// "yagluths-thing"; "Mímir" -> "mimir".
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		r = fold(r)
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r == '\'' || r == '’':
			// elide apostrophes: "Odin's" -> "odins"
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// fold maps accented letters to their base letter.
func fold(r rune) rune {
	switch r {
	case 'á', 'à', 'â', 'ä', 'ã', 'å', 'ā':
		return 'a'
	case 'é', 'è', 'ê', 'ë', 'ē':
		return 'e'
	case 'í', 'ì', 'î', 'ï', 'ī':
		return 'i'
	case 'ó', 'ò', 'ô', 'ö', 'õ', 'ø', 'ō':
		return 'o'
	case 'ú', 'ù', 'û', 'ü', 'ū':
		return 'u'
	case 'ý', 'ÿ':
		return 'y'
	case 'ñ':
		return 'n'
	case 'ç':
		return 'c'
	case 'æ':
		return 'a'
	case 'ð':
		return 'd'
	case 'þ':
		return 't'
	case 'ß':
		return 's'
	}
	if unicode.IsLetter(r) && r > 127 {
		return '-'
	}
	return r
}

// slugger hands out unique slugs within one namespace: a second thing
// with the same name gets its prefab appended.
type slugger struct {
	used map[string]string // slug -> prefab
}

func newSlugger() *slugger { return &slugger{used: map[string]string{}} }

func (s *slugger) take(name, prefab string) string {
	base := Slug(name)
	if base == "" {
		base = Slug(prefab)
	}
	if base == "" {
		base = "x"
	}
	if held, ok := s.used[base]; !ok || held == prefab {
		s.used[base] = prefab
		return base
	}
	alt := base + "-" + Slug(prefab)
	if held, ok := s.used[alt]; ok && held != prefab {
		for i := 2; ; i++ {
			try := alt + "-" + itoa(i)
			if _, ok := s.used[try]; !ok {
				alt = try
				break
			}
		}
	}
	s.used[alt] = prefab
	return alt
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// prettyOverrides names the few prefabs the game leaves unnamed where
// splitting the prefab would read wrong.
var prettyOverrides = map[string]string{
	"Deer_White":          "White Deer",
	"ArmorStand_Male":     "Armour Stand (male)",
	"ArmorStand_Female":   "Armour Stand (female)",
	"MemorialStone_Large": "Memorial Stone (large)",
}

// Pretty is a readable name for a prefab the game gives none: the
// override when there is one, "Spirit Wolf" for a spiritcaller's summon,
// else the prefab's words split at underscores and capital letters, with
// the effect and spawner prefixes dropped.
func Pretty(prefab string) string {
	if n, ok := prettyOverrides[prefab]; ok {
		return n
	}
	if base, ok := strings.CutSuffix(prefab, "_spiritcaller"); ok {
		return "Spirit " + Pretty(base)
	}
	s := prefab
	for _, pre := range []string{"vfx_", "sfx_", "fx_"} {
		s = strings.TrimPrefix(s, pre)
	}
	if rest, ok := strings.CutPrefix(s, "Spawner_"); ok {
		return Pretty(rest) + " spawner"
	}
	var b strings.Builder
	prev := rune(0)
	for i, r := range s {
		switch {
		case r == '_':
			b.WriteByte(' ')
		case i > 0 && unicode.IsUpper(r) && unicode.IsLower(prev):
			b.WriteByte(' ')
			b.WriteRune(r)
		case i == 0:
			b.WriteRune(unicode.ToUpper(r))
		default:
			b.WriteRune(r)
		}
		prev = r
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
