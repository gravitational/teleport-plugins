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
	terraform -chdir=$(TFDIR) init -var-file="vars.tfvars" && terraform -chdir=$(TFDIR) apply -auto-approve -var-file="vars.tfvars"

# Used for debugging
.PHONY: reapply
reapply:
	terraform -chdir=$(TFDIR) apply -var-file="vars.tfvars"

# Regenerates types_terraform.go
gen-schema:
	@protoc \
		-I$(LOCALDIR)/vendor/github.com/gravitational/teleport/api/types \
		-I$(LOCALDIR)/vendor/github.com/gogo/protobuf \
		-I$(LOCALDIR)/vendor \
		--plugin=$(GENTERRAFORMPATH)/protoc-gen-terraform \
		--terraform_out=config=gen_teleport.yaml:./tfschema \
		types.proto
