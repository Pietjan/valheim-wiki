# ==================================================================================== #
# CONFIG
# ==================================================================================== #

main_package_path = ./cmd/wiki
binary_name = wiki
out_dir = ./public
# The site's path on GitHub Pages; "/" when served from a domain's root.
base_path = /valheim-wiki/
origin = https://pietjan.github.io
# The game client, for the data and icon extraction (not needed to build the site).
valheim_data = /mnt/c/Program Files (x86)/Steam/steamapps/common/Valheim/valheim_Data
valheim_softref = ${valheim_data}/StreamingAssets/SoftRef
# valheim-npc's extracted models, for the rendered location and creature icons.
mesh_dir = ../valheim-npc/meshes
game_version = $(shell grep -o 'new GameVersion([0-9, ]*)' ../valheim-npc/reference/assembly_valheim/Version.cs 2>/dev/null | tail -1 | tr -d 'newGamVrsio() ' | tr ',' '.')
wasm_out = web/static/world/world.wasm

# ==================================================================================== #
# HELPERS
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

.PHONY: confirm
confirm:
	@echo -n 'Are you sure? [y/N] ' && read ans && [ $${ans:-N} = y ]

.PHONY: no-dirty
no-dirty:
	@test -z "$(shell git status --porcelain)"

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## tidy: tidy modfiles and format .go files
.PHONY: tidy
tidy:
	go mod tidy -v
	go fmt ./...

## audit: run quality control checks
.PHONY: audit
audit: test
	go mod tidy -diff
	go mod verify
	test -z "$(shell gofmt -l .)"
	go vet ./...
	GOOS=js GOARCH=wasm go vet ./cmd/world-wasm/...
	go run honnef.co/go/tools/cmd/staticcheck@latest -checks=all,-ST1000,-U1000 ./...
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

## test: run all tests
.PHONY: test
test:
	go test -race -buildvcs ./...

## test/short: run the tests that do not place a whole world
.PHONY: test/short
test/short:
	go test -short -race -buildvcs ./...

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

## wasm: build the seed engine for the browser into web/static/world
.PHONY: wasm
wasm:
	GOOS=js GOARCH=wasm go build -ldflags='-s -w' -o ${wasm_out} ./cmd/world-wasm
	cp "$(shell go env GOROOT)/lib/wasm/wasm_exec.js" web/static/world/wasm_exec.js

## build: build the site into public/ (base path ${base_path})
.PHONY: build
build: wasm
	go run ${main_package_path} -out ${out_dir} -base ${base_path} -origin ${origin}

## build/local: build the site for serving at /
.PHONY: build/local
build/local: wasm
	go run ${main_package_path} -out ${out_dir} -base /

## run: build the site for / and serve it at http://localhost:8080
.PHONY: run
run: wasm
	go run ${main_package_path} -out ${out_dir} -base / -serve :8080

## data: read the game's bundles into data/ and assets/icons (needs the client)
.PHONY: data
data:
	GOMEMLIMIT=6GiB go run ./cmd/wiki-data -softref "${valheim_softref}" -out data -icons assets/icons -game "${game_version}"

## icons: render the location and creature icons from valheim-npc's meshes
.PHONY: icons
icons:
	go run ./cmd/wiki-icons -meshes ${mesh_dir} -out assets/icons

## models: pack the 3D view's models from ${mesh_dir} into web/static/models
.PHONY: models
models:
	go run ./cmd/wiki-models -meshes ${mesh_dir} -out web/static/models

## probe: dump the components of prefabs: make probe names=Troll,Crypt2
.PHONY: probe
probe:
	go run ./cmd/wiki-data -softref "${valheim_softref}" -probe "$(names)" -probe-out probe.txt
