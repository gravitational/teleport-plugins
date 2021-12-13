# Build the plugin binary
FROM golang:1.17.3 as builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Copy the go source
COPY access/slack access/slack
COPY vendor vendor
COPY lib lib

# Build
RUN make -C access/slack

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base:4f25af540d54
COPY --from=builder /workspace/access/slack/build/teleport-slack /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/teleport-slack"]
