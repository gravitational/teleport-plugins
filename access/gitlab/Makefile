VERSION=0.2.0
GO_VERSION=1.17.8

BUILDDIR ?= build
BINARY = $(BUILDDIR)/teleport-gitlab
GITTAG=v$(VERSION)
ADDFLAGS ?=
BUILDFLAGS ?= $(ADDFLAGS) -ldflags '-w -s'
CGOFLAG ?= CGO_ENABLED=1

OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
RELEASE_NAME=teleport-access-gitlab
RELEASE=$(RELEASE_NAME)-$(GITTAG)-$(OS)-$(ARCH)-bin

RELEASE_MESSAGE = "Building with GOOS=$(OS) GOARCH=$(ARCH)."

VERSRC = version.go gitref.go

DOCKER_NAME=access-plugin-gitlab
DOCKER_IMAGE = 146628656107.dkr.ecr.us-west-2.amazonaws.com/teleport/$(DOCKER_NAME):$(VERSION)
DOCKER_BUILD_ARGS = --build-arg GO_VERSION=${GO_VERSION} --build-arg ACCESS_PLUGIN=gitlab

.PHONY: $(BINARY)
$(BINARY):
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) go build -o $(BINARY) $(BUILDFLAGS)

.PHONY: test
test: FLAGS ?= '-race'
test: $(VERSRC)
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) go test $(FLAGS) $(ADDFLAGS)

clean:
	@echo "---> Cleaning up build artifacts."
	rm -rf $(BUILDDIR)
	-go clean -cache
	rm -rf $(RELEASE_NAME)
	rm -rf *.gz
	rm -f gitref.go

.PHONY: release
release: clean $(VERSRC) $(BINARY)
	@echo "---> $(RELEASE_MESSAGE)"
	mkdir $(RELEASE_NAME)
	cp -rf $(BINARY) \
		README.md \
		CHANGELOG.md \
		install \
		$(RELEASE_NAME)/
	echo $(GITTAG) > $(RELEASE_NAME)/VERSION
	tar -czf $(RELEASE).tar.gz $(RELEASE_NAME)
	rm -rf $(RELEASE_NAME)/
	@echo "---> Created $(RELEASE).tar.gz."

.PHONY: docker-build
docker-build: ## Build docker image with the plugin.
	docker build ${DOCKER_BUILD_ARGS} -t ${DOCKER_IMAGE} -f ../Dockerfile ../..

.PHONY: docker-push
docker-push: 
	docker push ${DOCKER_IMAGE}

# This rule triggers re-generation of version.go and gitref.go if Makefile changes
$(VERSRC): Makefile
	VERSION=$(VERSION) $(MAKE) -f version.mk setver
