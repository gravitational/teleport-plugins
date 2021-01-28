# This image builds a Docker image with Teleport Enterprise.
FROM teleport:latest

ARG RELEASE="teleport-ent-v6.0.0-alpha.2-linux-amd64-bin"

# Install Teleport
RUN (cd /teleport;\
      curl -L https://get.gravitational.com/$RELEASE.tar.gz | tar -xz ;\
      ./teleport-ent/install)

