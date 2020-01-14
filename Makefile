.PHONY:
access-slackbot:
	make -C access/slackbot

.PHONY:
access-example:
	go build -o build/access-example ./access/example

.PHONY:
release/access-slackbot:
	make -C access/slackbot release
