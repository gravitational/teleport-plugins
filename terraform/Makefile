VERSION=0.0.1
PROVIDER_PATH = ~/.terraform.d/plugins/gravitational.com/teleport/teleport/$(VERSION)/$(TERRAFORM_ARCH)/

BUILDDIR ?= build

ADDFLAGS ?=
BUILDFLAGS ?= $(ADDFLAGS) -ldflags '-w -s'
CGOFLAG ?= CGO_ENABLED=1

MKFILE_PATH := $(abspath $(lastword $(MAKEFILE_LIST)))

OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
TERRAFORM_ARCH=$(OS)_$(ARCH)

DOCKER_SRC_PATH=/root/go/src/github.com/gravitational/teleport-plugins

RUNTIME ?= go1.15.5
BBOX ?= quay.io/gravitational/teleport-buildbox:

.PHONY: build
build: clean
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) go build -o ./$(BUILDDIR)/terraform-provider-teleport $(BUILDFLAGS)
	mkdir -p $(PROVIDER_PATH)
	mv ./$(BUILDDIR)/terraform-provider-teleport $(PROVIDER_PATH)


.PHONY: clean
clean:
	rm -rf $(PROVIDER_PATH)*
	rm -rf ./build/*
	go clean


.PHONY: docker
docker:
	docker build \
		-t teleport-terraform:latest \
		--build-arg RUNTIME=$(RUNTIME) \
		--build-arg BBOX=$(BBOX) \
		.

.PHONY: docker-build
docker-build:
	docker run \
		-v $(MKFILE_PATH)/../:$(DOCKER_SRC_PATH) \
		teleport-terraform:latest \
			make -C $(DOCKER_SRC_PATH)/terraform build
