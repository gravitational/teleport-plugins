VERSION=0.1.0-dev.1

BUILDDIR ?= build
BINARY = $(BUILDDIR)/teleport-jirabot
GITTAG=v$(VERSION)
ADDFLAGS ?=
BUILDFLAGS ?= $(ADDFLAGS) -ldflags '-w -s'
CGOFLAG ?= CGO_ENABLED=1

OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
RELEASE_NAME=teleport-access-jirabot
RELEASE=$(RELEASE_NAME)-$(GITTAG)-$(OS)-$(ARCH)-bin

RELEASE_MESSAGE := "Building with GOOS=$(OS) GOARCH=$(ARCH)."

VERSRC = version.go gitref.go

export

.PHONY: $(BINARY)
$(BINARY):
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) go build -o $(BINARY) $(BUILDFLAGS)

clean:
	@echo "---> Cleaning up build artifacts."
	rm -rf $(BUILDDIR)
	-go clean -cache
	rm -rf $(RELEASE_NAME)
	rm -rf *.gz
	rm -f gitref.go

.PHONY: release
release: clean $(VERSRC) $(BINARY)
	@echo "---> $(RELEASE_MESSAGE)"
	mkdir $(RELEASE_NAME)
	cp -rf $(BINARY) \
		README.md \
		CHANGELOG.md \
		install \
		$(RELEASE_NAME)/
	echo $(GITTAG) > $(RELEASE_NAME)/VERSION
	tar -czf $(RELEASE).tar.gz $(RELEASE_NAME)
	rm -rf $(RELEASE_NAME)/
	@echo "---> Created $(RELEASE).tar.gz."

# This rule triggers re-generation of version.go and gitref.go if Makefile changes
$(VERSRC): Makefile
	VERSION=$(VERSION) $(MAKE) -f version.mk setver
