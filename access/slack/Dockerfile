FROM golang:1.14.15-buster

RUN update-ca-certificates

COPY . /app
WORKDIR /app
RUN make access-slack
RUN cp /app/access/slack/build/teleport-slack /usr/local/bin/teleport-slack
RUN rm -r /app

ENTRYPOINT ["/usr/local/bin/teleport-slack", "start"]
