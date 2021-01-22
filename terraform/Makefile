VERSION=0.0.1
TERRAFORM_ARCH=darwin_amd64
PROVIDER_PATH = ~/.terraform.d/plugins/gravitational.com/teleport/teleport/$(VERSION)/$(TERRAFORM_ARCH)/

.PHONY: build
build: clean
	go build -o ./build/terraform-provider-teleport
	mkdir -p $(PROVIDER_PATH)
	mv ./build/terraform-provider-teleport $(PROVIDER_PATH)

.PHONY: clean
clean:
	rm -rf $(PROVIDER_PATH)*
	rm -rf ./build/*
	go clean