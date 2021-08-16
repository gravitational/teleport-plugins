VERSION=7.0.2-f1

BUILDDIR ?= build
BINARY = $(BUILDDIR)/teleport-slack
GITTAG=v$(VERSION)
ADDFLAGS ?=
BUILDFLAGS ?= $(ADDFLAGS) -ldflags '-w -s'
CGOFLAG ?= CGO_ENABLED=1

OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
RELEASE_NAME=teleport-access-slack
RELEASE=$(RELEASE_NAME)-$(GITTAG)-$(OS)-$(ARCH)-bin

RELEASE_MESSAGE := "Building with GOOS=$(OS) GOARCH=$(ARCH)."

VERSRC = version.go gitref.go

export

.PHONY: $(BINARY)
$(BINARY):
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) go build -o $(BINARY) $(BUILDFLAGS)

.PHONY: test
test: FLAGS ?= '-race'
test: $(VERSRC)
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) go test $(FLAGS) $(ADDFLAGS)

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

.PHONY: docker
docker: $(VERSRC)
	cd ../.. && docker build -t teleport-slackbot:$(VERSION) -f access/slack/Dockerfile .

check-aws-env:
ifndef AWS_ACCESS_KEY_ID
	$(error AWS_ACCESS_KEY_ID is undefined)
endif
ifndef AWS_SECRET_ACCESS_KEY
	$(error AWS_SECRET_ACCESS_KEY is undefined)
endif

.PHONY: docker-push
docker-push: check-aws-env $(VERSRC) docker
	$(eval ACCOUNT := $(shell aws sts get-caller-identity --query 'Account' --output text))
	aws ecr get-login-password --region us-east-2 | docker login --username AWS --password-stdin $(ACCOUNT).dkr.ecr.us-east-2.amazonaws.com
	docker tag teleport-slackbot:$(VERSION) $(ACCOUNT).dkr.ecr.us-east-2.amazonaws.com/teleport-slackbot:$(VERSION)
	docker push $(ACCOUNT).dkr.ecr.us-east-2.amazonaws.com/teleport-slackbot:$(VERSION)
