VERSION=13.0.2
GO_VERSION=1.20.4

BUILDDIR ?= build
BINARY = $(BUILDDIR)/teleport-discord
GITTAG=v$(VERSION)
GITREF ?= $(shell git describe --dirty --long --tags --match '*discord*')
ADDFLAGS ?=
BUILDFLAGS ?= $(ADDFLAGS) -ldflags "-w -s -X main.Gitref=$(GITREF) -X main.Version=$(VERSION)"
CGOFLAG ?= CGO_ENABLED=1

OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
RELEASE_NAME=teleport-access-discord
RELEASE=$(RELEASE_NAME)-$(GITTAG)-$(OS)-$(ARCH)-bin

RELEASE_MESSAGE = "Building with GOOS=$(OS) GOARCH=$(ARCH)."

DOCKER_NAME = teleport-plugin-discord
DOCKER_IMAGE = 146628656107.dkr.ecr.us-west-2.amazonaws.com/gravitational/$(DOCKER_NAME):$(VERSION)
DOCKER_IMAGE_ECR_PUBLIC = public.ecr.aws/gravitational/$(DOCKER_NAME):$(VERSION)
DOCKER_BUILD_ARGS = --build-arg GO_VERSION=${GO_VERSION} --build-arg ACCESS_PLUGIN=discord --build-arg GITREF=$(GITREF)

.PHONY: $(BINARY)
$(BINARY):
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) go build -o $(BINARY) $(BUILDFLAGS)

.PHONY: test
test: FLAGS ?= '-race'
test:
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) go test $(FLAGS) $(ADDFLAGS)

clean:
	@echo "---> Cleaning up build artifacts."
	rm -rf $(BUILDDIR)
	-go clean -cache
	rm -rf $(RELEASE_NAME)
	rm -rf *.gz
	rm -f gitref.go

.PHONY: release
release: clean $(BINARY)
	@echo "---> $(RELEASE_MESSAGE)"
	mkdir $(RELEASE_NAME)
	cp -rf $(BINARY) \
		README.md \
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
docker-push: ## Push docker image with the plugin.
	docker push ${DOCKER_IMAGE}

.PHONY: docker-promote
docker-promote: docker-promote-ecr-public

.PHONY: docker-promote-ecr-public
docker-promote-ecr-public:
	docker pull ${DOCKER_IMAGE} && \
	docker tag ${DOCKER_IMAGE} ${DOCKER_IMAGE_ECR_PUBLIC} && \
	docker push ${DOCKER_IMAGE_ECR_PUBLIC}
