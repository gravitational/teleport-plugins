# This image builds a Docker image with Teleport Enterprise.
FROM teleport:latest

ARG RELEASE="teleport-ent-v14.0.0-linux-amd64-bin"

# Install Teleport
RUN (cd /teleport;\
      curl -L https://cdn.teleport.dev/$RELEASE.tar.gz | tar -xz ;\
      ./teleport-ent/install)

