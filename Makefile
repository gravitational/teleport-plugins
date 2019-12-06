.PHONY:
slackbot:
	go build -o build/slackbot ./access/slackbot

.PHONY:
example-cli:
	go build -o build/example-cli ./access/example-cli

.PHONY:
clean:
	rm -r build
