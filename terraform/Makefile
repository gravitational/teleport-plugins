VERSION=0.0.1
PROVIDER_PATH = ~/.terraform.d/plugins/gravitational.com/teleport/teleport/$(VERSION)/$(TERRAFORM_ARCH)/

BUILDDIR ?= build
TFDIR ?= example

ADDFLAGS ?=
BUILDFLAGS ?= $(ADDFLAGS) -ldflags '-w -s'
CGOFLAG ?= CGO_ENABLED=1

OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
TERRAFORM_ARCH=$(OS)_$(ARCH)


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
	go clean

.PHONY: build
build: clean
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) go build -o $(BUILDDIR)/terraform-provider-teleport $(BUILDFLAGS)
	mkdir -p $(PROVIDER_PATH)
	mv $(BUILDDIR)/terraform-provider-teleport $(PROVIDER_PATH)

.PHONY: reapply
reapply: build
	cd $(TFDIR) && terraform init && terraform apply -auto-approve