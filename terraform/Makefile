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

gen-tfschema:
	@protoc \
		-I$(LOCALDIR)/../vendor/github.com/gravitational/teleport/api/types \
		-I$(LOCALDIR)/../vendor/github.com/gogo/protobuf \
		-I$(LOCALDIR)/../vendor \
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

.phone: debug_import
debug_import:
	make install && terraform -chdir=$(TFDIR) init -var-file="vars.tfvars" && terraform -chdir=$(TFDIR) import -var-file="vars.tfvars" teleport_role.test test