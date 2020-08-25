GO_LINTERS ?= "golint,unused,govet,typecheck,deadcode,goimports,varcheck,structcheck,bodyclose,staticcheck,ineffassign,unconvert,misspell"

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

# Run all tests
.PHONY: test
test:
	go test -count 1 ./...

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

# Run all releases
.PHONY: releases
releases: release/access-slack release/access-jira release/access-mattermost release/access-pagerduty release/access-gitlab

.PHONY: build-all
build-all: access-slack access-jira access-mattermost access-pagerduty access-gitlab

#
# Lint the Go code.
# By default lint scans the entire repo. Pass FLAGS='--new' to only scan local
# changes (or last commit).
#
.PHONY: lint
lint: FLAGS ?=
lint:
	golangci-lint run \
		--disable-all \
		--exclude-use-default \
		--skip-dirs vendor \
		--uniq-by-line=false \
		--max-same-issues=0 \
		--max-issues-per-linter 0 \
		--timeout=5m \
		--enable $(GO_LINTERS) \
		$(FLAGS)
