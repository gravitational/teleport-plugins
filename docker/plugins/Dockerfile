# Defines Teleport Slack image, based on Teleport OSS image.
ARG RUNTIME="go1.16.2"
ARG BBOX="quay.io/gravitational/teleport-buildbox"
FROM $BBOX:$RUNTIME

RUN apt-get update

# DEBUG=1 is needed for the Web UI to be loaded from static assets instead
# of the binary
ENV DEBUG=1 GOPATH=/root/go PATH=$PATH:/root/go/src/github.com/gravitational/teleport/build:/root/go/src/github.com/gravitational/teleport-plugins/access/slack/build:/root/go/bin

# htop is useful for testing terminal resizing
RUN apt-get update; apt-get install --no-install-recommends -y htop vim screen;

# allows ansible and ssh testing
RUN apt-get install --no-install-recommends -y ansible ssh inetutils-syslogd

RUN mkdir /run/sshd

VOLUME ["/root/go/src/github.com/gravitational/teleport-plugins", "/var/lib/teleport"]
COPY .bashrc /root/.bashrc
COPY .screenrc /root/.screenrc
COPY ./sshd/start.sh /usr/bin/start-sshd.sh
