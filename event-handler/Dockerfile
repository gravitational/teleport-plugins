# Build the plugin binary
ARG GO_VERSION

FROM golang:${GO_VERSION}-bullseye as builder

ARG GITREF

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Copy the go source
COPY event-handler event-handler

# Build
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build make -C event-handler GITREF=${GITREF}

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base@sha256:46c5b9bd3e3efff512e28350766b54355fce6337a0b44ba3f822ab918eca4520

COPY --from=builder /workspace/event-handler/build/teleport-event-handler /usr/local/bin/teleport-event-handler

ENTRYPOINT ["/usr/local/bin/teleport-event-handler"]
