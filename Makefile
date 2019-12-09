.PHONY:
access-slackbot:
	go build -o build/slackbot ./access/slackbot

.PHONY:
access-example:
	go build -o build/example ./access/example

.PHONY:
clean:
	rm -r build
