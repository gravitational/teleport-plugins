# Defines Teleport Slack image
# Based on Teleport's image
# Tweak this and then do `make build-teleport-slack-image` to rebuild the image.

# The base image (buildbox:latest) is built by running `make -C build.assets`
# from the teleport repo directory $GOPATH/gravitational.com/teleport
ARG RUNTIME="go1.15.5"
FROM quay.io/gravitational/teleport-buildbox:$RUNTIME
RUN apt-get update
# DEBUG=1 is needed for the Web UI to be loaded from static assets instead
# of the binary
ENV DEBUG=1 GOPATH=/root/go PATH=$PATH:/root/go/src/github.com/gravitational/teleport/build:/root/go/src/github.com/gravitational/teleport-plugins/access/slack/build:/root/go/bin

# htop is useful for testing terminal resizing
RUN apt-get update; apt-get install -y htop vim screen;

# allows ansible and ssh testing
RUN apt-get install -y ansible ssh inetutils-syslogd

RUN mkdir /run/sshd

VOLUME ["/root/go/src/github.com/gravitational/teleport-plugins", "/var/lib/teleport"]
COPY .bashrc /root/.bashrc
COPY .screenrc /root/.screenrc
COPY ./sshd/start.sh /usr/bin/start-sshd.sh
