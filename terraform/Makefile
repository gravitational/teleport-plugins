include install.mk

LOCALDIR := $(dir $(CURDIR)/$(word $(words $(MAKEFILE_LIST)),$(MAKEFILE_LIST)))
GENTERRAFORMPATH := $(shell go env GOPATH)/bin

BUILDDIR ?= build
TFDIR ?= example

ADDFLAGS ?=
BUILDFLAGS ?= $(ADDFLAGS) -ldflags '-w -s'
CGOFLAG ?= CGO_ENABLED=0

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
PROTOC_GEN_TERRAFORM_EXISTS := $(shell protoc-gen-terraform version 2>&1 >/dev/null | grep 'protoc-gen-terraform $(PROTOC_GEN_TERRAFORM_VERSION)')

.PHONY: gen-tfschema
gen-tfschema:
ifndef PROTOC_GEN_TERRAFORM_EXISTS
	@echo "protoc-gen-terraform v1.0.0 is not installed. Please, refer to README.md for installation instructions."
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

	@go run ./_gen/main.go

.PHONY: release
release: build
	tar -C $(BUILDDIR) -czf $(RELEASE).tar.gz .

TERRAFORM_EXISTS := $(shell terraform -version | grep 'Terraform v1.')
CURRENT_ULIMIT := $(shell ulimit -n)

.PHONY: test
test: install
ifndef TERRAFORM_EXISTS
	@echo "Terraform v1.1+ is not installed. Please, refer to README.md for installation instructions."
	@exit -1
endif
# NOTE: This is related to an old bug in Terraform and will be fixed in future releases (possibly, by a workaround on our side)
ifeq ($(shell expr $(CURRENT_ULIMIT) \< 1024), 1)
	@echo "ulimit -n is too low ($(CURRENT_ULIMIT)), please set ulimit -n 1024"
	@exit -1
endif
	TF_ACC=true go test ./test -v

.PHONY: apply
apply: install
	terraform -chdir=$(TFDIR) init && terraform -chdir=$(TFDIR) apply -auto-approve

.PHONY: reapply
reapply:
	terraform -chdir=$(TFDIR) apply

.PHONY: destroy
destroy:
	terraform -chdir=$(TFDIR) destroy -auto-approve
