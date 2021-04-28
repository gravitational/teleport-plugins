ARG GO_VER
FROM golang:${GO_VER}

ARG UID
ARG GID
ARG ARCH

RUN set -ex && apt-get -q -y update --fix-missing && apt-get -q -y install unzip dumb-init libc6

RUN set -ex && \
    getent group  $GID || groupadd builder --gid=$GID -o; \
    getent passwd $UID || useradd builder --uid=$UID --gid=$GID --create-home --shell=/bin/sh;

RUN set -ex && (mkdir -p /.cache && \
     mkdir -p /terraform && \
     mkdir -p /var/lib/teleport && \
     chown -R $UID:$GID /go && \
     chown -R $UID:$GID /terraform && \
     chown -R $UID:$GID /var/lib/teleport && \
     chmod 777 /.cache && \
     chmod 777 /terraform && \
     chmod 777 /tmp && \
     chmod 777 /var/lib/teleport)

USER $UID:$GID

ENV LANGUAGE="en_US.UTF-8" \
     LANG="en_US.UTF-8" \
     LC_ALL="en_US.UTF-8" \
     LC_CTYPE="en_US.UTF-8" \
     PATH="$PATH:/opt/protoc/bin:/go/bin"