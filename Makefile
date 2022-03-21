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

.PHONY: access-email
access-email:
	make -C access/email

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

# Push specific access plugin with docker to ECR
.PHONY: docker-push-access-%
docker-push-access-%: docker-build-access-%
	$(MAKE) -C access/$* docker-push

# Build event-handler plugin with docker
.PHONY: docker-build-event-handler
docker-build-event-handler:
	$(MAKE) -C event-handler docker-build

.PHONY: docker-push-event-handler
docker-push-event-handler: docker-build-event-handler
	$(MAKE) -C event-handler docker-push

.PHONY: terraform
terraform:
	make -C terraform

.PHONY: event-handler
event-handler:
	make -C event-handler

# Run all tests
.PHONY: test
test:
	@echo Testing plugins against Teleport $(TELEPORT_GET_VERSION)
	go test -race -count 1 ./...

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

.PHONY: update-tag
update-tag:
	# Make sure VERSION is set on the command line "make update-tag VERSION=x.y.z".
	@test $(VERSION)
	# Tag all releases first locally.
	git tag --sign --message "Teleport Event Handler Plugin $(VERSION)" teleport-event-handler-v$(VERSION)
	git tag --sign --message "Teleport Access Jira Plugin $(VERSION)" teleport-jira-v$(VERSION)
	git tag --sign --message "Teleport Access Mattermost Plugin $(VERSION)" teleport-mattermost-v$(VERSION)
	git tag --sign --message "Teleport Access Slack Plugin $(VERSION)" teleport-slack-v$(VERSION)
	git tag --sign --message "Teleport Access Pagerduty $(VERSION)" teleport-pagerduty-v$(VERSION)
	git tag --sign --message "Teleport Access Email $(VERSION)" teleport-email-v$(VERSION)
	git tag --sign --message "Teleport Terraform Provider $(VERSION)" terraform-provider-teleport-v$(VERSION)
	git tag --sign --message "Teleport Plugins $(VERSION)" v$(VERSION)
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

.PHONY: print-version
print-version:
	@./version.sh