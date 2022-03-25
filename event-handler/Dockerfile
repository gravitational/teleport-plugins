# Build the plugin binary
ARG GO_VERSION

FROM golang:${GO_VERSION} as builder

ARG GITREF

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Copy the go source
COPY event-handler event-handler
COPY lib lib

# Build
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build make -C event-handler GITREF=${GITREF}

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base@sha256:03dcbf61f859d0ae4c69c6242c9e5c3d7e1a42e5d3b69eb235e81a5810dd768e

COPY --from=builder /workspace/event-handler/build/teleport-event-handler /usr/local/bin/teleport-event-handler

ENTRYPOINT ["/usr/local/bin/teleport-event-handler"]