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

.PHONY: access-webhooks
access-webhooks:
	make -C access/webhooks

.PHONY: access-example
access-example:
	go build -o build/access-example ./access/example

# Run all tests
.PHONY: test
test:
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

.PHONY: release/access-webhooks
release/access-webhooks:
	make -C access/webhooks clean release

# Run all releases
.PHONY: releases
releases: release/access-slack release/access-jira release/access-mattermost release/access-pagerduty release/access-gitlab

.PHONY: build-all
build-all: access-slack access-jira access-mattermost access-pagerduty access-gitlab access-webhooks

#
# Lint the Go code.
# By default lint scans the entire repo. Pass GO_LINT_FLAGS='--new' to only scan local
# changes (or last commit).
#
.PHONY: lint
lint: GO_LINT_FLAGS ?=
lint:
	golangci-lint run -c .golangci.yml $(GO_LINT_FLAGS)
