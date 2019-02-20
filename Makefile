BINARY := d2p-migrator
PKGS := $(shell go list ./... | grep -v /vendor)

# VERSION is used for daemon Release Version in go build.
VERSION ?= "1.0.0"

# GIT_COMMIT is used for daemon GitCommit in go build.
GIT_COMMIT=$(shell git describe --dirty --always --tags 2> /dev/null || true)

# BUILD_TIME is used for daemon BuildTime in go build.
BUILD_TIME=$(shell date --rfc-3339 s 2> /dev/null | sed -e 's/ /T/')
VERSION_PKG=github.com/pouchcontainer/d2p-migrator
DEFAULT_LDFLAGS="-X ${VERSION_PKG}/version.GitCommit=${GIT_COMMIT} \
		  -X ${VERSION_PKG}/version.Version=${VERSION} \
		  -X ${VERSION_PKG}/version.BuildTime=${BUILD_TIME}"

.PHONY: test
test: lint
	@echo $@
	@go test $(PKGS)

.PHONY: lint
lint:
	@golint $(PKGS)

.PHONY: linux
linux: plugin
	@mkdir -p bin
	@GOOS=linux go build -ldflags ${DEFAULT_LDFLAGS} -o bin/$(BINARY)
	@./hack/module --clean

.PHONY: build
build: linux

.PHONY: plugin
plugin: ## build hook plugin
	@echo "build $@"
	@./hack/module --add-plugin github.com/pouchcontainer/d2p-migrator/hookplugins/containerplugin
