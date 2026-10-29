// Package site renders the wiki: the data in model.DB becomes a tree of
// static pages under one output directory, with the client assets beside
// them. Every page is a directory with an index.html; every link goes
// through the base path so the same build serves at / and at
// /valheim-wiki/.
package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// Config is what a build needs.
type Config struct {
	DB     *model.DB
	Web    fs.FS  // the web/ directory: templates/ and static/
	Icons  string // the assets/icons directory on disk
	Out    string // where to write
	Base   string // URL prefix, "/" or "/valheim-wiki/"
	Title  string // the site's name
	Origin string // canonical origin, e.g. https://pietjan.github.io (for sitemap, og tags); "" for none
}

// Site is one build in progress.
type Site struct {
	Config
	pageT map[string]*template.Template
	Index *Index
	// pages written, by URL path, for the sitemap.
	pages []string
	// iconSets is what icons exist on disk: kind -> name -> true.
	iconSets map[string]map[string]bool
	built    time.Time
	// The guides, built on first use, and the stager they share; see
	// guides.go and guides_gear.go.
	food   *FoodGuide
	gear   *GearGuide
	stages *stager
}

// Build renders everything.
func Build(cfg Config) error {
	if cfg.Base == "" {
		cfg.Base = "/"
	}
	if !strings.HasSuffix(cfg.Base, "/") {
		cfg.Base += "/"
	}
	if cfg.Title == "" {
		cfg.Title = "Vegvísir"
	}
	s := &Site{Config: cfg, built: time.Now()}
	if err := s.scanIcons(); err != nil {
		return err
	}
	s.Index = BuildIndex(cfg.DB, s.hasIcon)
	if err := s.loadTemplates(); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.Out, 0o755); err != nil {
		return err
	}
	steps := []struct {
		name string
		fn   func() error
	}{
		{"static", s.copyStatic},
		{"icons", s.copyIcons},
		{"home", s.pageHome},
		{"items", s.pagesItems},
		{"crafting", s.pagesCrafting},
		{"building", s.pagesBuilding},
		{"creatures", s.pagesCreatures},
		{"biomes", s.pagesBiomes},
		{"locations", s.pagesLocations},
		{"effects", s.pagesEffects},
		{"traders", s.pagesTraders},
		{"events", s.pagesEvents},
		{"guides", s.pagesGuides},
		{"world", s.pageWorld},
		{"search", s.pageSearch},
		{"about", s.pageAbout},
		{"data", s.writeSiteData},
		{"sitemap", s.writeSitemap},
	}
	for _, st := range steps {
		t := time.Now()
		if err := st.fn(); err != nil {
			return fmt.Errorf("%s: %w", st.name, err)
		}
		fmt.Printf("    %-9s %.1fs\n", st.name, time.Since(t).Seconds())
	}
	fmt.Printf("==> %d pages in %s\n", len(s.pages), cfg.Out)
	return nil
}

// URL is the absolute path of a site-relative URL.
func (s *Site) URL(p string) string {
	p = strings.TrimPrefix(p, "/")
	return s.Base + p
}

// scanIcons lists the icon files on disk.
func (s *Site) scanIcons() error {
	s.iconSets = map[string]map[string]bool{}
	kinds, err := os.ReadDir(s.Icons)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, k := range kinds {
		if !k.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(s.Icons, k.Name()))
		if err != nil {
			return err
		}
		set := map[string]bool{}
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".png") {
				set[strings.TrimSuffix(f.Name(), ".png")] = true
			}
		}
		s.iconSets[k.Name()] = set
	}
	return nil
}

// hasIcon is whether assets/icons/<kind>/<name>.png exists.
func (s *Site) hasIcon(kind, name string) bool { return s.iconSets[kind][name] }

// Icon is the URL of an icon, or "" when there is none.
func (s *Site) Icon(kind, name string) string {
	if !s.hasIcon(kind, name) {
		return ""
	}
	return s.URL("icons/" + kind + "/" + name + ".png")
}

// write renders a template into <out>/<urlpath>/index.html.
func (s *Site) write(urlPath, tmpl string, data any) error {
	t := s.pageT[tmpl]
	if t == nil {
		return fmt.Errorf("%s: no template %q", urlPath, tmpl)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout.html", data); err != nil {
		return fmt.Errorf("%s: %w", urlPath, err)
	}
	dir := filepath.Join(s.Out, filepath.FromSlash(strings.Trim(urlPath, "/")))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), buf.Bytes(), 0o644); err != nil {
		return err
	}
	s.pages = append(s.pages, path.Clean("/"+urlPath)+"/")
	return nil
}

// writeFile writes raw bytes under the output directory.
func (s *Site) writeFile(rel string, b []byte) error {
	p := filepath.Join(s.Out, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// writeJSON writes a compact JSON file under the output directory.
func (s *Site) writeJSON(rel string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.writeFile(rel, b)
}

// copyStatic copies web/static into <out>/static.
func (s *Site) copyStatic() error {
	return fs.WalkDir(s.Web, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(s.Web, p)
		if err != nil {
			return err
		}
		return s.writeFile(p, b)
	})
}

// copyIcons copies assets/icons into <out>/icons.
func (s *Site) copyIcons() error {
	if _, err := os.Stat(s.Icons); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(s.Icons, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(s.Icons, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return s.writeFile(filepath.Join("icons", rel), b)
	})
}

// writeSitemap lists every page.
func (s *Site) writeSitemap() error {
	pages := append([]string(nil), s.pages...)
	sort.Strings(pages)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	origin := strings.TrimSuffix(s.Origin, "/")
	for _, p := range pages {
		fmt.Fprintf(&b, "<url><loc>%s%s</loc></url>\n", origin, s.URL(p))
	}
	b.WriteString("</urlset>\n")
	if err := s.writeFile("sitemap.xml", []byte(b.String())); err != nil {
		return err
	}
	robots := "User-agent: *\nAllow: /\n"
	if origin != "" {
		robots += "Sitemap: " + origin + s.URL("sitemap.xml") + "\n"
	}
	if err := s.writeFile("robots.txt", []byte(robots)); err != nil {
		return err
	}
	// GitHub Pages: no Jekyll.
	return s.writeFile(".nojekyll", nil)
}
