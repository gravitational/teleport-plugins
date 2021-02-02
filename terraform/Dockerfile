ARG RUNTIME="go1.15.5"
ARG BBOX="quay.io/gravitational/teleport-buildbox"

FROM quay.io/gravitational/teleport-buildbox:go1.15.5

# DEBUG=1 is needed for the Web UI to be loaded from static assets instead
# of the binary
ENV GOPATH=/root/go

# Install the basic set of packages
RUN apt-get update; apt-get install -y software-properties-common lsb-release htop vim screen curl gnupg2;

# Install terraform CLI
RUN curl -fsSL https://apt.releases.hashicorp.com/gpg | apt-key add -
RUN apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
RUN apt-get update && apt-get install -y terraform


VOLUME ["/root/go/src/github.com/gravitational/teleport-plugins"]
