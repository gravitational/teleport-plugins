# Set up a system-agnostic in-place sed command
IS_GNU_SED = $(shell sed --version 1>/dev/null 2>&1 && echo true || echo false)

ifeq ($(IS_GNU_SED),true)
	SED = sed -i
else
	SED = sed -i ''
endif

.PHONY: access-slack
access-slack:
	make -C access/slack

.PHONY: access-jira
access-jira:
	make -C access/jira

.PHONY: access-mattermost
access-mattermost:
	make -C access/mattermost

.PHONY: access-pagerduty
access-pagerduty:
	make -C access/pagerduty

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
 docker-build-access-jira \
 docker-build-access-mattermost \
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
	helm package -d packages charts/access/slack
	helm package -d packages charts/access/pagerduty
	helm package -d packages charts/access/mattermost

.PHONY: terraform
terraform:
	make -C terraform

.PHONY: terraform-gen-tfschema
terraform-gen-tfschema:
	make -C terraform gen-tfschema

.PHONY: event-handler
event-handler:
	make -C event-handler

# Run all tests
.PHONY: test
test: test-tooling
	@echo Testing plugins against Teleport $(TELEPORT_GET_VERSION)
	go test -race -count 1 $(shell go list ./...)

.PHONY: test-tooling
test-tooling:
	(cd tooling; go test -v -race ./...)

# Individual releases
.PHONY: release/access-slack
release/access-slack:
	make -C access/slack clean release

.PHONY: release/access-jira
release/access-jira:
	make -C access/jira clean release

.PHONY: release/access-mattermost
release/access-mattermost:
	make -C access/mattermost clean release

.PHONY: release/access-pagerduty
release/access-pagerduty:
	make -C access/pagerduty clean release

.PHONY: release/access-email
release/access-email:
	make -C access/email clean release

.PHONY: release/terraform
release/terraform:
	make -C terraform clean release

.PHONY: release/event-handler
release/event-handler:
	make -C event-handler clean release

# Run all releases
.PHONY: releases
releases: release/access-slack release/access-jira release/access-mattermost release/access-pagerduty release/access-email

.PHONY: build-all
build-all: access-slack access-jira access-mattermost access-pagerduty access-email terraform event-handler

.PHONY: update-version
update-version:
	# Make sure VERSION is set on the command line "make update-version VERSION=x.y.z".
	@test $(VERSION)
	$(SED) '1s/.*/VERSION=$(VERSION)/' event-handler/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/jira/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/mattermost/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/slack/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/pagerduty/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' access/email/Makefile
	$(SED) '1s/.*/VERSION=$(VERSION)/' terraform/install.mk
	$(MAKE) update-helm-version
	$(MAKE) terraform-gen-tfschema

# Update all charts to VERSION
.PHONY: update-helm-version
update-helm-version:
	$(MAKE) update-helm-version-access-email
	$(MAKE) update-helm-version-access-slack
	$(MAKE) update-helm-version-access-pagerduty
	$(MAKE) update-helm-version-access-mattermost

# Update specific chart
.PHONY: update-helm-version-%
update-helm-version-access-%:
	$(SED) 's/appVersion: .*/appVersion: "$(VERSION)"/' charts/access/$*/Chart.yaml
	$(SED) 's/version: .*/version: "$(VERSION)"/' charts/access/$*/Chart.yaml
	# Update snapshots
	@helm unittest -u charts/access/$* || { echo "Please install unittest as described in .cloudbuild/helm-unittest.yaml" ; exit 1; }

.PHONY: update-tag
update-tag:
	# Make sure VERSION is set on the command line "make update-tag VERSION=x.y.z".
	@test $(VERSION)
	# Tag all releases first locally.
	git tag teleport-event-handler-v$(VERSION)
	git tag teleport-jira-v$(VERSION)
	git tag teleport-mattermost-v$(VERSION)
	git tag teleport-slack-v$(VERSION)
	git tag teleport-pagerduty-v$(VERSION)
	git tag teleport-email-v$(VERSION)
	git tag terraform-provider-teleport-v$(VERSION)
	git tag v$(VERSION)
	# Push all releases to origin.
	git push origin teleport-event-handler-v$(VERSION)
	git push origin teleport-jira-v$(VERSION)
	git push origin teleport-mattermost-v$(VERSION)
	git push origin teleport-slack-v$(VERSION)
	git push origin teleport-pagerduty-v$(VERSION)
	git push origin teleport-email-v$(VERSION)
	git push origin terraform-provider-teleport-v$(VERSION)
	git push origin v$(VERSION)

#
# Lint the Go code.
# By default lint scans the entire repo. Pass GO_LINT_FLAGS='--new' to only scan local
# changes (or last commit).
#
.PHONY: lint
lint: GO_LINT_FLAGS ?=
lint:
	golangci-lint run -c .golangci.yml $(GO_LINT_FLAGS)

.PHONY: test-helm-access-%
test-helm-access-%:
	helm unittest ./charts/access/$*

.PHONY: test-helm
test-helm:
	$(MAKE) test-helm-access-email
	$(MAKE) test-helm-access-slack
	$(MAKE) test-helm-access-pagerduty
	$(MAKE) test-helm-access-mattermost
