include install.mk

LOCALDIR := $(dir $(CURDIR)/$(word $(words $(MAKEFILE_LIST)),$(MAKEFILE_LIST)))
GENTERRAFORMPATH := $(shell go env GOPATH)/bin

BUILDDIR ?= build
TFDIR ?= example

ADDFLAGS ?=
BUILDFLAGS ?= $(ADDFLAGS) -ldflags '-w -s'
CGOFLAG ?= CGO_ENABLED=1

RELEASE = terraform-provider-teleport-v$(VERSION)-$(OS)-$(ARCH)-bin

.PHONY: tfclean
tfclean:
	rm -rf $(TFDIR)/terraform.tfstate
	rm -rf $(TFDIR)/terraform.tfstate.backup
	rm -rf $(TFDIR)/.terraform
	rm -rf $(TFDIR)/.terraform.lock.hcl

.PHONY: clean
clean: tfclean
	rm -rf $(PROVIDER_PATH)*
	rm -rf $(BUILDDIR)/*
	rm -rf $(RELEASE).tar.gz
	go clean

.PHONY: build
build: clean
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) go build -o $(BUILDDIR)/terraform-provider-teleport $(BUILDFLAGS)

CUSTOM_IMPORTS_TMP_DIR ?= /tmp/protoc-gen-terraform/custom-imports
PROTOC_GEN_TERRAFORM_VERSION ?= v1.0.0
PROTOC_GEN_TERRAFORM_EXIST := $(shell protoc-gen-terraform version 2>&1 >/dev/null | grep 'protoc-gen-terraform $(PROTOC_GEN_TERRAFORM_VERSION)')

.PHONY: gen-tfschema
gen-tfschema:
ifndef PROTOC_GEN_TERRAFORM_EXIST
	@echo "protoc-gen-terraform v1.0.0 does not exist. Please, refer to README.md on installation instructions."
	@exit -1
endif

# The wrappers.proto file needed for this generator exist only inside the go mod cache,
# so we retrieve the file path for the cached proto files with go mod tools.
	$(eval API_MOD_PATH := $(shell go mod download --json github.com/gravitational/teleport/api | jq .Dir))
	$(eval PROTOBUF_MOD_PATH := $(shell go mod download --json github.com/gogo/protobuf | jq .Dir))

# In order for types.proto to find the wrappers.proto file in the mod cache above, it
# needs be imported (-I) with its full import path discoverable. To achieve this, we
# create a temp directory and move wrappers.proto into it.
#
# Ideally, protoc-gen-terraform could be updated to reroute paths in a similar way to
# gogofast with the "M" option, which we used in the main teleport repo to overcome a
# similar issue.
	rm -rf $(CUSTOM_IMPORTS_TMP_DIR)
	mkdir -p $(CUSTOM_IMPORTS_TMP_DIR)/github.com/gravitational/teleport/api/types/wrappers
	cp $(API_MOD_PATH)/types/wrappers/wrappers.proto $(CUSTOM_IMPORTS_TMP_DIR)/github.com/gravitational/teleport/api/types/wrappers

	@protoc \
		-I$(API_MOD_PATH)/types \
		-I$(PROTOBUF_MOD_PATH) \
		-I$(CUSTOM_IMPORTS_TMP_DIR) \
		--plugin=$(GENTERRAFORMPATH)/protoc-gen-terraform \
		--terraform_out=config=protoc-gen-terraform-teleport.yaml:./tfschema \
		types.proto

.PHONY: release
release: build
	tar -C $(BUILDDIR) -czf $(RELEASE).tar.gz .

.PHONY: apply
apply: install
	terraform -chdir=$(TFDIR) init -var-file="vars.tfvars" && terraform -chdir=$(TFDIR) apply -auto-approve -var-file="vars.tfvars"

.PHONY: reapply
reapply:
	terraform -chdir=$(TFDIR) apply -var-file="vars.tfvars"

# NOTE: Please note that running these tests might require to increase ulimit (ulimit -n 1024). 
# Default OS X limit of 256 does not work.
.PHONY: test
test: install
	TF_ACC=true go test ./test -v
