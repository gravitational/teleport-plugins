LOCALDIR := $(dir $(CURDIR)/$(word $(words $(MAKEFILE_LIST)),$(MAKEFILE_LIST)))
TOP ?= $(abspath $(LOCALDIR)/../..)
SRCDIR=/go/src/github.com/gravitational/teleport-plugins/fluentd-forward
VERSION=0.0.1
CGOFLAG ?= CGO_ENABLED=1
BUILDDIR=$(TOP)/build
OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)

HOSTNAME=buildbox
BUILDBOX ?= teleport-plugins-fluentd-forward:$(VERSION)
GOCACHE ?= $(HOME)/.cache/go-build
DOCKERFLAGS := --rm \
	-v "$(TOP)":$(SRCDIR) \
	-v $(GOCACHE):/go/cache \
	-e GOCACHE=/go/cache \
	-w $(SRCDIR) \
	-h $(HOSTNAME)

BUILDFORHOST := -e OS=$(OS) \
	-e ARCH=$(ARCH) \
	-e CGOFLAG="CGO_ENABLED=0"

DOCKERPULLFLAGS ?= --pull
GO_VER = 1.16

NOROOT=-u $$(id -u):$$(id -g)
BINDIR=/usr/local/bin

.PHONY: buildbox
buildbox:
	docker build \
		--build-arg GO_VER=$(GO_VER) \
		--build-arg UID=$(shell id -u) \
		--build-arg GID=$(shell id -g) \
		--build-arg ARCH=$(ARCH) \
		$(DOCKERPULLFLAGS) --tag $(BUILDBOX) .

.PHONY: build
build: buildbox
	docker run $(DOCKERFLAGS) $(BUILDFORHOST) $(NOROOT) $(BUILDBOX) \
		dumb-init make -C $(SRCDIR)/fluentd-forward build

.PHONY: test
test: build
	docker run $(DOCKERFLAGS) $(BUILDFORHOST) $(NOROOT) $(BUILDBOX) \
		dumb-init make -C $(SRCDIR)/fluentd-forward test

.PHONY: install
install: build
	mkdir -p $(BINDIR)
	cp ../build/teleport-fluentd-forward $(BINDIR)