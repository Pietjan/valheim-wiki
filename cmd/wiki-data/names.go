package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// fillNames runs after every domain is read and makes sure each thing a
// page is made for has an English name: a name still holding a $token
// (a token the main table lacks, or a nested one) is localized again,
// and an empty one falls back to NameOf (names.tsv, the thing's token,
// then the prefab spaced out). Locations whose prefab has no names.tsv
// row are listed so the table can be extended.
func fillNames(x *X) error {
	fixed := 0
	fix := func(prefab, token string, name *string) {
		if strings.Contains(*name, "$") {
			if t := x.Text(*name); t != "" && !strings.Contains(t, "$") {
				*name = t
				fixed++
				return
			}
		}
		if *name == "" || strings.HasPrefix(*name, "$") {
			*name = x.NameOf(prefab, token)
			fixed++
		}
	}
	for _, k := range model.SortedKeys(x.DB.Items) {
		it := x.DB.Items[k]
		fix(it.Prefab, it.Token, &it.Name)
	}
	for _, k := range model.SortedKeys(x.DB.Pieces) {
		p := x.DB.Pieces[k]
		fix(p.Prefab, p.Token, &p.Name)
	}
	for _, k := range model.SortedKeys(x.DB.Stations) {
		s := x.DB.Stations[k]
		fix(s.Prefab, "", &s.Name)
	}
	for _, k := range model.SortedKeys(x.DB.Converters) {
		c := x.DB.Converters[k]
		fix(c.Prefab, "", &c.Name)
	}
	for _, k := range model.SortedKeys(x.DB.Creatures) {
		c := x.DB.Creatures[k]
		fix(c.Prefab, c.Token, &c.Name)
	}
	for _, k := range model.SortedKeys(x.DB.Resources) {
		r := x.DB.Resources[k]
		fix(r.Prefab, r.Token, &r.Name)
	}
	for _, k := range model.SortedKeys(x.DB.Effects) {
		e := x.DB.Effects[k]
		fix(e.ID, e.Token, &e.Name)
	}
	for _, s := range x.DB.Skills {
		fix(s.ID, "", &s.Name)
	}
	var unnamed []string
	for _, k := range model.SortedKeys(x.DB.Locations) {
		l := x.DB.Locations[k]
		fix(l.Prefab, l.Token, &l.Name)
		if l.Kind == "" {
			l.Kind = slug(l.Name)
		}
		if r, ok := x.names[l.Prefab]; !ok || r.english == "" {
			unnamed = append(unnamed, fmt.Sprintf("%s=%q", l.Prefab, l.Name))
		}
	}
	sort.Strings(unnamed)
	fmt.Printf("    %d names fixed; %d locations without a names.tsv row: %s\n", fixed, len(unnamed), strings.Join(unnamed, " "))
	return nil
}
