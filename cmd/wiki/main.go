// Command wiki builds the static site from data/ and web/ into public/,
// and can serve the result for a look.
package main

import (
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pietjan/valheim-wiki/internal/model"
	"github.com/pietjan/valheim-wiki/internal/site"
)

func main() {
	data := flag.String("data", "data", "the extracted data directory")
	icons := flag.String("icons", "assets/icons", "the icons directory")
	web := flag.String("web", "web", "templates and static files")
	out := flag.String("out", "public", "output directory")
	base := flag.String("base", "/", "URL path the site is served under, e.g. /valheim-wiki/")
	origin := flag.String("origin", "", "canonical origin for the sitemap, e.g. https://pietjan.github.io")
	serve := flag.String("serve", "", "after building, serve the output at this address, e.g. :8080")
	flag.Parse()

	started := time.Now()
	db, err := model.Read(*data)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("==> %d items, %d pieces, %d creatures, %d locations, %d resources\n", len(db.Items), len(db.Pieces), len(db.Creatures), len(db.Locations), len(db.Resources))
	if err := os.RemoveAll(*out); err != nil {
		log.Fatal(err)
	}
	cfg := site.Config{DB: db, Web: os.DirFS(*web), Icons: *icons, Out: *out, Base: *base, Origin: *origin}
	if err := site.Build(cfg); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("==> built in %.1fs\n", time.Since(started).Seconds())
	if *serve == "" {
		return
	}
	mime.AddExtensionType(".wasm", "application/wasm")
	prefix := strings.TrimSuffix(*base, "/")
	fs := http.FileServer(http.Dir(*out))
	h := http.Handler(fs)
	if prefix != "" {
		h = http.StripPrefix(prefix, fs)
	}
	http.Handle("/", h)
	abs, _ := filepath.Abs(*out)
	fmt.Printf("==> serving %s at http://localhost%s%s/\n", abs, *serve, prefix)
	log.Fatal(http.ListenAndServe(*serve, nil))
}
