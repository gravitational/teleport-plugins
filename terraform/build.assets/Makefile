include ../install.mk

LOCALDIR := $(dir $(CURDIR)/$(word $(words $(MAKEFILE_LIST)),$(MAKEFILE_LIST)))
TOP ?= $(abspath $(LOCALDIR))
SRCDIR=/go/src/github.com/gzigzigzeo/teleport-plugins/terraform
VERSION=0.0.1
CGOFLAG ?= CGO_ENABLED=1
BUILDDIR=$(TOP)/build
TELEPORT_VER ?= 6.0.1
TERRAFORM_VER ?= 0.14.9

HOSTNAME=buildbox
BUILDBOX ?= teleport-plugins-terraform:$(VERSION)
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

.PHONY: buildbox
buildbox:
	docker build \
		--build-arg GO_VER=$(GO_VER) \
		--build-arg UID=$(shell id -u) \
		--build-arg GID=$(shell id -g) \
		--build-arg TELEPORT_VER=$(TELEPORT_VER) \
		--build-arg TERRAFORM_VER=${TERRAFORM_VER} \
		--build-arg ARCH=$(ARCH) \
		$(DOCKERPULLFLAGS) --tag $(BUILDBOX) .


.PHONY: build
build: buildbox
	docker run $(DOCKERFLAGS) $(BUILDFORHOST) $(NOROOT) $(BUILDBOX) \
		dumb-init make -C $(SRCDIR) build
