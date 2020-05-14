GO_LINTERS ?= "unused,govet,typecheck,deadcode,goimports,varcheck,structcheck,bodyclose,staticcheck,ineffassign,unconvert,misspell"

.PHONY: access-slackbot
access-slackbot:
	make -C access/slackbot

.PHONY: access-jirabot
access-jirabot:
	make -C access/jirabot

.PHONY: access-mattermost
access-mattermost:
	make -C access/mattermost

.PHONY: access-pagerduty
access-pagerduty:
	make -C access/pagerduty

.PHONY: access-example
access-example:
	go build -o build/access-example ./access/example

# Run all tests
.PHONY: test
test:
	go test -count 1 ./...

# Individual releases
.PHONY: release/access-slackbot
release/access-slackbot:
	make -C access/slackbot clean release

.PHONY: release/access-jirabot
release/access-jirabot:
	make -C access/jirabot clean release

.PHONY: release/access-mattermost
release/access-mattermost:
	make -C access/mattermost clean release

.PHONY: release/access-pagerduty
release/access-pagerduty:
	make -C access/pagerduty clean release

# Run all releases
.PHONY: releases
releases: release/access-slackbot release/access-jirabot release/access-mattermost release/access-pagerduty

.PHONY: get-deps
get-deps:
	go get -v -t -d ./...
	
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
