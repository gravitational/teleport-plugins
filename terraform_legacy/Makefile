include install.mk

LOCALDIR := $(dir $(CURDIR)/$(word $(words $(MAKEFILE_LIST)),$(MAKEFILE_LIST)))
GENTERRAFORMPATH := $(shell go env GOPATH)/bin

BUILDDIR ?= build
TFDIR ?= tf

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

.PHONY: release
release: build
	tar -C $(BUILDDIR) -czf $(RELEASE).tar.gz .

# Used for debugging
.PHONY: setup-tf
setup-tf:
	mkdir -p tf
	cp example/* tf
	cp tf/vars.tfvars.example tf/vars.tfvars

# Used for debugging
.PHONY: apply
apply: install
	-tctl tokens rm example
	-tctl users rm example
	-tctl rm role/example
	-tctl rm github/example
	-tctl rm oidc/example
	-tctl rm saml/example
	-tctl rm app/example
	-tctl rm db/example
	terraform -chdir=$(TFDIR) init -var-file="vars.tfvars" && terraform -chdir=$(TFDIR) apply -auto-approve -var-file="vars.tfvars"

# Used for debugging
.PHONY: reapply
reapply:
	terraform -chdir=$(TFDIR) apply -var-file="vars.tfvars"

CUSTOM_IMPORTS_TMP_DIR ?= /tmp/protoc-gen-terraform/custom-imports

# Regenerates types_terraform.go
gen-schema:

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
		--plugin=$(GENTERRAFORMPATH)/protoc-gen-terraform\
		--terraform_out=config=gen_teleport.yaml:./tfschema \
		types.proto
