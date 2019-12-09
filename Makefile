.PHONY:
access-slackbot:
	go build -o build/access-slackbot ./access/slackbot

.PHONY:
access-example:
	go build -o build/access-example ./access/example

.PHONY:
clean:
	rm -r build
