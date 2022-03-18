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

.PHONY: access-gitlab
access-gitlab:
	make -C access/gitlab

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
 docker-build-access-gitlab \
 docker-build-access-jira \
 docker-build-access-mattermost \
 docker-build-access-pagerduty \
 docker-build-access-slack

# Build event-handler plugin with docker
.PHONY: docker-build-event-handler
docker-build-event-handler:
	$(MAKE) -C event-handler docker-build

.PHONY: terraform
terraform:
	make -C terraform

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

.PHONY: release/access-gitlab
release/access-gitlab:
	make -C access/gitlab clean release

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
releases: release/access-slack release/access-jira release/access-mattermost release/access-pagerduty release/access-gitlab release/access-email

.PHONY: build-all
build-all: access-slack access-jira access-mattermost access-pagerduty access-gitlab access-email terraform event-handler

.PHONY: update-version
update-version:
	# Make sure VERSION is set on the command line "make update-version VERSION=x.y.z".
	@test $(VERSION)
	sed -i '1s/.*/VERSION=$(VERSION)/' event-handler/Makefile
	make -C event-handler version.go
	sed -i '1s/.*/VERSION=$(VERSION)/' access/jira/Makefile
	make -C access/jira version.go
	sed -i '1s/.*/VERSION=$(VERSION)/' access/mattermost/Makefile
	make -C access/mattermost version.go
	sed -i '1s/.*/VERSION=$(VERSION)/' access/slack/Makefile
	make -C access/slack version.go
	sed -i '1s/.*/VERSION=$(VERSION)/' access/pagerduty/Makefile
	make -C access/pagerduty version.go
	sed -i '1s/.*/VERSION=$(VERSION)/' access/email/Makefile
	make -C access/email version.go
	sed -i '1s/.*/VERSION=$(VERSION)/' terraform/install.mk

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
