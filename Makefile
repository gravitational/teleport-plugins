.PHONY:
slackbot:
	go build -o build/slackbot ./access/slackbot

.PHONY:
example:
	go build -o build/example ./access/example

.PHONY:
clean:
	rm -r build
