.PHONY:
slackbot:
	go build -o build/slackbot ./access/slackbot

.PHONY:
clean:
	rm -r build
