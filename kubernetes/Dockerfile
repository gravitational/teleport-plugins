ARG version

# Build the operator binary
FROM golang:1.17.9 as builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

# Copy the go source
COPY kubernetes kubernetes
COPY lib lib

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make -C ./kubernetes go-build

FROM quay.io/gravitational/teleport:${version} as teleport

# Use distroless as minimal base image to package the operator binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/cc-debian11
WORKDIR /
COPY --from=builder /workspace/kubernetes/bin/teleport-operator /usr/local/bin/
COPY --from=teleport /usr/local/bin/tctl /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/teleport-operator"]
