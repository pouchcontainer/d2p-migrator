BINARY := d2p-migrator
PKGS := $(shell go list ./ ... | grep -v /vendor)

GOBUILD := "go build"

.PHONY: test
test: lint
	echo $@
	go test $(PKGS)

.PHONY: lint
lint:
	golint ./ ... | grep -Fv 'vendor/'

.PHONY: linux
linux:
	mkdir -p bin
	GOOS=linux $(GOBUILD) -o bin/$(BINARY)
GOBUILD=go build

.PHONY: build
build: linux


