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

# Releases
.PHONY: release/access-slackbot
release/access-slackbot:
	make -C access/slackbot release

.PHONY: release/access-jirabot
release/access-jirabot:
	make -C access/jirabot release

.PHONY: release/access-mattermost
release/access-mattermost:
	make -C access/mattermost release

.PHONY: release/access-pagerduty
release/access-pagerduty:
	make -C access/pagerduty release

.PHONY: release-all
release-all: release/access-slackbot release/access-jirabot release/access-mattermost release/access-pagerduty
