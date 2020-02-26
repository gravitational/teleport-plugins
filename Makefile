.PHONY:
access-slackbot:
	make -C access/slackbot

.PHONY: access-jirabot
access-jirabot:
	make -C access/jirabot

.PHONY:
access-example:
	go build -o build/access-example ./access/example

.PHONY:
release/access-slackbot:
	make -C access/slackbot release

.PHONY: release/access-jirabot
release/access-jirabot:
	make -C access/jirabot release
