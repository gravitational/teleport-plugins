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

# Regenerates types_terraform.go
#
# The proto files needed for this generator exist only inside the go mod cache,
# so we retrieve the file path for the cached proto files with go mod tools.
gen-schema: API_MOD_PATH ?= $(shell go mod download --json github.com/gravitational/teleport/api | jq .Dir)
gen-schema: PROTOBUF_MOD_PATH ?= $(shell go mod download --json github.com/gogo/protobuf | jq .Dir)
# protoc expects to find .proto files needed by types.proto by its full path.
# Due to the way go modules work, we don't have the exact path to the cached
# module on file, so we make a temporary directory with a symlink to the cached 
# module to get the desired file structure. This is a bandaid fix due to the
# removal of /vendor, and should be replaced with a better solution. In the main
# teleport repo, we ran into a similar issue with our grpc generation, and we used
# the M flag to achieve a similar result, but I haven't found the way to do this here.
gen-schema:
	mkdir -p $(LOCALDIR)/vendor/github.com/gravitational/teleport/api
	ln -s $(API_MOD_PATH)/* $(LOCALDIR)/vendor/github.com/gravitational/teleport/api

	@protoc \
		-I$(API_MOD_PATH)/types \
		-I$(PROTOBUF_MOD_PATH) \
		-I$(LOCALDIR)/vendor \
		--plugin=$(GENTERRAFORMPATH)/protoc-gen-terraform\
		--terraform_out=config=gen_teleport.yaml:./tfschema \
		types.proto

	rm -r $(LOCALDIR)/vendor
