# Build the plugin binary
ARG GO_VERSION

FROM golang:${GO_VERSION} as builder

ARG ACCESS_PLUGIN

WORKDIR /workspaces

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

# Copy the go source
COPY access/${ACCESS_PLUGIN} access/${ACCESS_PLUGIN}
COPY lib lib

# Build
RUN make -C access/${ACCESS_PLUGIN}

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base@sha256:03dcbf61f859d0ae4c69c6242c9e5c3d7e1a42e5d3b69eb235e81a5810dd768e
ARG ACCESS_PLUGIN
COPY --from=builder /workspaces/access/${ACCESS_PLUGIN}/build/teleport-${ACCESS_PLUGIN} /usr/local/bin/teleport-plugin

ENTRYPOINT ["/usr/local/bin/teleport-plugin"]