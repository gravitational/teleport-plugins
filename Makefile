# Set up a system-agnostic in-place sed command
IS_GNU_SED = $(shell sed --version 1>/dev/null 2>&1 && echo true || echo false)

DRONE ?= drone
DRONE_REPO ?= gravitational/teleport-plugins
DRONE_PROMOTE_ENV ?= production
PLUGINS ?= teleport-event-handler \
			teleport-discord \
			teleport-jira \
			teleport-mattermost \
			teleport-msteams \
			teleport-slack \
			teleport-pagerduty \
			teleport-email \
			terraform-provider-teleport

ifeq ($(IS_GNU_SED),true)
	SED = sed -i
else
	SED = sed -i ''
endif

.PHONY: access-slack
access-slack:
	$(MAKE) -C access/slack

.PHONY: access-discord
access-discord:
	$(MAKE) -C access/discord

.PHONY: access-jira
access-jira:
	$(MAKE) -C access/jira

.PHONY: access-mattermost
access-mattermost:
	$(MAKE) -C access/mattermost

.PHONY: access-msteams
access-msteams:
	$(MAKE) -C access/msteams

.PHONY: access-pagerduty
access-pagerduty:
	$(MAKE) -C access/pagerduty

.PHONY: access-example
access-example:
	go build -o build/access-example ./access/example

.PHONY: access-email
access-email:
	go build -o build/access-email ./access/email

# Build specific access plugin with docker
.PHONY: docker-build-access-%
docker-build-access-%:
	$(MAKE) -C access/$* docker-build

# Build all access plugins with docker
.PHONY: docker-build-access-plugins
docker-build-access-plugins: docker-build-access-email \
 docker-build-access-discord \
 docker-build-access-jira \
 docker-build-access-mattermost \
 docker-build-access-msteams \
 docker-build-access-pagerduty \
 docker-build-access-slack

# Push specific access plugin with docker to ECR
.PHONY: docker-push-access-%
docker-push-access-%: docker-build-access-%
	$(MAKE) -C access/$* docker-push

# Pulls and pushes image from ECR to quay.
.PHONY: docker-promote-access-%
docker-promote-access-%:
	$(MAKE) -C access/$* docker-promote

# Build event-handler plugin with docker
.PHONY: docker-build-event-handler
docker-build-event-handler:
	$(MAKE) -C event-handler docker-build

.PHONY: docker-push-event-handler
docker-push-event-handler: docker-build-event-handler
	$(MAKE) -C event-handler docker-push

.PHONY: docker-promote-event-handler
docker-promote-event-handler:
	$(MAKE) -C event-handler docker-promote


.PHONY: helm-package-charts
helm-package-charts:
	mkdir -p packages
	helm package -d packages charts/access/email
	helm package -d packages charts/access/discord
	helm package -d packages charts/access/jira
	helm package -d packages charts/access/slack
	helm package -d packages charts/access/pagerduty
	helm package -d packages charts/access/mattermost
	helm package -d packages charts/access/msteams
	helm package -d packages charts/event-handler

.PHONY: terraform
terraform:
	$(MAKE) -C terraform

.PHONY: terraform-gen-tfschema
terraform-gen-tfschema:
	$(MAKE) -C terraform gen-tfschema

.PHONY: test-terraform
test-terraform:
	$(MAKE) -C terraform test

.PHONY: event-handler
event-handler:
	$(MAKE) -C event-handler

# Run all tests
.PHONY: test
test: test-tooling test-unit test-terraform
	@echo Testing plugins against Teleport $(TELEPORT_GET_VERSION)
	go test -race -count 1 $(shell go list ./... | grep -v -e '/terraform/' -e '/tooling/' -e '/lib')

.PHONY: test-tooling
test-tooling:
	(cd tooling; go test -v -race ./...)

.PHONY: test-unit
test-unit: test-tooling test-access test-event-handler

.PHONY: test-access
test-access:
	(cd access; go test -v -race ./...)

.PHONY: test-event-handler
test-event-handler:
	(cd event-handler; go test -v -race ./...)

# Individual releases
.PHONY: release/access-slack
release/access-slack:
	$(MAKE) -C access/slack clean release

.PHONY: release/access-discord
release/access-discord:
	$(MAKE) -C access/discord clean release

.PHONY: release/access-jira
release/access-jira:
	$(MAKE) -C access/jira clean release

.PHONY: release/access-mattermost
release/access-mattermost:
	$(MAKE) -C access/mattermost clean release

.PHONY: release/access-msteams
release/access-msteams:
	$(MAKE) -C access/msteams clean release

.PHONY: release/access-pagerduty
release/access-pagerduty:
	$(MAKE) -C access/pagerduty clean release

.PHONY: release/access-email
release/access-email:
	$(MAKE) -C access/email clean release

.PHONY: release/terraform
release/terraform:
	$(MAKE) -C terraform clean release

.PHONY: release/event-handler
release/event-handler:
	$(MAKE) -C event-handler clean release

# Run all releases
.PHONY: releases
releases: release/access-slack release/access-discord release/access-jira release/access-mattermost release/access-msteams release/access-pagerduty release/access-email

.PHONY: build-all
build-all: access-slack access-discord access-jira access-mattermost access-msteams access-pagerduty access-email terraform event-handler

.PHONY: update-version
update-version:
	# Make sure VERSION is set on the command line "make update-version VERSION=x.y.z".
	@test $(VERSION)
	$(SED) '1s/.*/VERSION=$(VERSION)/' event-handler/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/discord/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/jira/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/mattermost/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/msteams/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/slack/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/pagerduty/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/email/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' terraform/install.mk
	$(MAKE) update-helm-version
	$(MAKE) terraform-gen-tfschema
	$(MAKE) update-teleport-dep-version

# Update all charts to VERSION
.PHONY: update-helm-version
update-helm-version:
	$(MAKE) update-helm-version-access-email
	$(MAKE) update-helm-version-access-discord
	$(MAKE) update-helm-version-access-jira
	$(MAKE) update-helm-version-access-slack
	$(MAKE) update-helm-version-access-pagerduty
	$(MAKE) update-helm-version-access-mattermost
	$(MAKE) update-helm-version-access-msteams
	$(MAKE) update-helm-version-event-handler

# Update specific chart
.PHONY: update-helm-version-%
update-helm-version-%:
	$(SED) 's/appVersion: .*/appVersion: "$(VERSION)"/' charts/$(subst access-,access/,$*)/Chart.yaml
	$(SED) 's/version: .*/version: "$(VERSION)"/' charts/$(subst access-,access/,$*)/Chart.yaml
	# Update snapshots
	@helm unittest -u -3 charts/$(subst access-,access/,$*) || { echo "Please install unittest as described in .cloudbuild/helm-unittest.yaml" ; exit 1; }

.PHONY: update-teleport-dep-version
update-teleport-dep-version:
	./update_teleport_dep_version.sh v$(VERSION)

.PHONY: update-tag
update-tag:
	# Make sure VERSION is set on the command line "make update-tag VERSION=x.y.z".
	@test $(VERSION)
	# Tag all releases first locally.
	git tag teleport-event-handler-v$(VERSION)
	git tag teleport-discord-v$(VERSION)
	git tag teleport-jira-v$(VERSION)
	git tag teleport-mattermost-v$(VERSION)
	git tag teleport-msteams-v$(VERSION)
	git tag teleport-slack-v$(VERSION)
	git tag teleport-pagerduty-v$(VERSION)
	git tag teleport-email-v$(VERSION)
	git tag terraform-provider-teleport-v$(VERSION)
	git tag v$(VERSION)
	# Push all releases to origin.
	git push origin teleport-event-handler-v$(VERSION)
	git push origin teleport-discord-v$(VERSION)
	git push origin teleport-jira-v$(VERSION)
	git push origin teleport-mattermost-v$(VERSION)
	git push origin teleport-msteams-v$(VERSION)
	git push origin teleport-slack-v$(VERSION)
	git push origin teleport-pagerduty-v$(VERSION)
	git push origin teleport-email-v$(VERSION)
	git push origin terraform-provider-teleport-v$(VERSION)
	git push origin v$(VERSION)

TELEPORT_GET_VERSION ?= v12.0.4
.PHONY: update-test-version
update-test-version:
	curl https://get.gravitational.com/teleport-{ent-,}${TELEPORT_GET_VERSION}-{darwin-amd64,linux-{amd64,arm64,arm}}-bin.tar.gz.sha256 > \
	lib/testing/integration/download_sha.dsv
	$(SED) 's/TELEPORT_GET_VERSION: .*/TELEPORT_GET_VERSION: $(TELEPORT_GET_VERSION)/g' .drone.yml
	$(SED) 's/TELEPORT_GET_VERSION: .*/TELEPORT_GET_VERSION: $(TELEPORT_GET_VERSION)/g' .github/workflows/terraform-tests.yaml
	$(SED) 's/TELEPORT_GET_VERSION: .*/TELEPORT_GET_VERSION: $(TELEPORT_GET_VERSION)/g' .github/workflows/unit-tests.yaml
	@echo Please sign .drone.yml before staging and committing the changes

# promote-tag executes Drone promotion pipeline for the plugins.
#
# It has to be run after tag builds triggered by the "update-tag" target have
# been completed. Requires "drone" executable to be available and configured
# to talk to our Drone cluster.
#
# To promote all plugins:
#   VERSION=10.2.6 make promote-tag
#
# To promote a particular plugin:
#   VERSION=10.2.6 PLUGINS=teleport-slack make promote-tag
.PHONY: promote-tag
promote-tag:
	@test $(VERSION)
	@for PLUGIN in $(PLUGINS); do \
		BUILD=$$($(DRONE) build ls --status success --event tag --format "{{.Number}} {{.Ref}}" $(DRONE_REPO) | grep $${PLUGIN}-v$(VERSION) | cut -d ' ' -f1); \
		if [ "$${BUILD}" = "" ]; then \
			echo "Failed to find Drone build number for $${PLUGIN}-v$(VERSION)" && exit 1; \
		else \
			echo "\n\n --> Promoting build $${BUILD} for plugin $${PLUGIN}" to $(DRONE_PROMOTE_ENV); \
			$(DRONE) build promote $(DRONE_REPO) $${BUILD} $(DRONE_PROMOTE_ENV); \
		fi; \
	done

.PHONY: update-goversion
update-goversion:
	# Make sure GOVERSION is set on the command line "make update-goversion GOVERSION=x.y.z".
	@test $(GOVERSION)
	$(SED) '2s/.*/GO_VERSION=$(GOVERSION)/' access/discord/Makefile
	$(SED) '2s/.*/GO_VERSION=$(GOVERSION)/' access/jira/Makefile
	$(SED) '2s/.*/GO_VERSION=$(GOVERSION)/' access/mattermost/Makefile
	$(SED) '2s/.*/GO_VERSION=$(GOVERSION)/' access/msteams/Makefile
	$(SED) '2s/.*/GO_VERSION=$(GOVERSION)/' access/slack/Makefile
	$(SED) '2s/.*/GO_VERSION=$(GOVERSION)/' access/pagerduty/Makefile
	$(SED) '2s/.*/GO_VERSION=$(GOVERSION)/' access/email/Makefile
	$(SED) '2s/.*/GO_VERSION=$(GOVERSION)/' event-handler/Makefile
	$(SED) 's/^RUNTIME ?= go.*/RUNTIME ?= go$(GOVERSION)/' docker/Makefile
	$(SED) 's/Setup Go .*/Setup Go $(GOVERSION)/g' .github/workflows/unit-tests.yaml
	$(SED) 's/Setup Go .*/Setup Go $(GOVERSION)/g' .github/workflows/terraform-tests.yaml
	$(SED) 's/Setup Go .*/Setup Go $(GOVERSION)/g' .github/workflows/lint.yaml
	$(SED) "s/go-version: '.*/go-version: '$(GOVERSION)'/g" .github/workflows/unit-tests.yaml
	$(SED) "s/go-version: '.*/go-version: '$(GOVERSION)'/g" .github/workflows/terraform-tests.yaml
	$(SED) "s/go-version: '.*/go-version: '$(GOVERSION)'/g" .github/workflows/lint.yaml
	$(SED) 's/image: golang:.*/image: golang:$(GOVERSION)/g' .drone.yml
	$(SED) 's/GO_VERSION: go.*/GO_VERSION: go$(GOVERSION)/g' .drone.yml
	@echo Please sign .drone.yml before staging and committing the changes

# Lint the project
# Currently lints the go files and license headers in most files.
.PHONY: lint
lint: lint-go lint-license

#
# Lint the Go code.
# By default lint scans the entire repo. Pass GO_LINT_FLAGS='--new' to only scan local
# changes (or last commit).
#
.PHONY: lint-go
lint-go: GO_LINT_FLAGS ?=
lint-go:
	golangci-lint run -c .golangci.yml $(GO_LINT_FLAGS)

GOPATH ?= $(shell go env GOPATH)
ADDLICENSE := $(GOPATH)/bin/addlicense
ADDLICENSE_ARGS := -c 'Gravitational, Inc' -l apache \
		-ignore '**/Dockerfile' \
		-ignore '**/*.xml' \
		-ignore '**/*.tf' \
		-ignore '**/*.js' \
		-ignore '**/*.sh' \
		-ignore '**/*.java' \
		-ignore '**/*.yaml' \
		-ignore '**/*.yml'

.PHONY: lint-license
lint-license: $(ADDLICENSE)
	$(ADDLICENSE) $(ADDLICENSE_ARGS) -check * 2>/dev/null

.PHONY: fix-license
fix-license: $(ADDLICENSE)
	$(ADDLICENSE) $(ADDLICENSE_ARGS) * 2>/dev/null

$(ADDLICENSE):
	cd && go install github.com/google/addlicense@v1.0.0

GCI := $(GOPATH)/bin/gci
$(GCI):
	cd && go install github.com/daixiang0/gci@latest

.PHONY: fix-imports
fix-imports: $(GCI)
	$(GCI) write -s standard -s default -s 'prefix(github.com/gravitational/teleport-plugins)' --skip-generated .

.PHONY: test-helm-%
test-helm-%:
	helm unittest -3 ./charts/$(subst access-,access/,$*)

.PHONY: test-helm
test-helm:
	$(MAKE) test-helm-access-email
	$(MAKE) test-helm-access-discord
	$(MAKE) test-helm-access-jira
	$(MAKE) test-helm-access-slack
	$(MAKE) test-helm-access-pagerduty
	$(MAKE) test-helm-access-mattermost
	$(MAKE) test-helm-access-msteams
	$(MAKE) test-helm-event-handler
