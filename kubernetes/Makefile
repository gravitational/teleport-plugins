VERSION=9.1.2
RELEASE_NAME=teleport-operator

# Image URL to use all building/pushing image targets
IMG ?= quay.io/gravitational/$(RELEASE_NAME):$(VERSION)
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.23

VERSRC = version.go gitref.go

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: crdgen
crdgen:
	make -C crdgen

.PHONY: manifests
manifests: crdgen controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
# Generate CRDs for apis/resources (custom generation through protoc plugin).

# The wrappers.proto file needed for this generator exist only inside the go mod cache,
# so we retrieve the file path for the cached proto files with go mod tools.
	$(eval API_MOD_PATH := $(shell go mod download --json github.com/gravitational/teleport/api | jq -r .Dir))
	$(eval PROTOBUF_MOD_PATH := $(shell go mod download --json github.com/gogo/protobuf | jq -r .Dir))
	$(eval CUSTOM_IMPORTS_TMP_DIR := $(shell mktemp -d))
	mkdir -p $(CUSTOM_IMPORTS_TMP_DIR)/github.com/gravitational/teleport/api/types/wrappers
	cp $(API_MOD_PATH)/types/wrappers/wrappers.proto $(CUSTOM_IMPORTS_TMP_DIR)/github.com/gravitational/teleport/api/types/wrappers
	protoc \
		-I=$(API_MOD_PATH)/types \
		-I=$(PROTOBUF_MOD_PATH) \
		-I=$(CUSTOM_IMPORTS_TMP_DIR) \
		--plugin=./crdgen/protoc-gen-crd \
		--crd_out=:./crd \
		types.proto
# Generate CRDs for apis/auth.
	$(CONTROLLER_GEN) \
		crd \
		paths="./apis/auth/..." \
		output:crd:artifacts:config=./crd

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet envtest $(VERSRC) ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

##@ Build

.PHONY: go-build
go-build:
	go build -o bin/teleport-operator

.PHONY: build
build: generate fmt vet $(VERSRC) go-build ## Build operator binary.

.PHONY: run
run: manifests generate fmt vet ## Run an operator from your host.
	go run . -- start-sidecar

.PHONY: docker-build
docker-build: test ## Build docker image with the operator.
	docker build --build-arg version=${VERSION} -t ${IMG} -f ./Dockerfile ..

.PHONY: docker-push
docker-push: ## Push docker image with the operator.
	docker push ${IMG}

##@ Deployment

.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	go run . -- install-crds

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.8.0)

ENVTEST = $(shell pwd)/bin/setup-envtest
.PHONY: envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

# go-install-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

# This rule triggers re-generation of version.go and gitref.go if Makefile changes
$(VERSRC): Makefile
	VERSION=$(VERSION) $(MAKE) -f version.mk setver
