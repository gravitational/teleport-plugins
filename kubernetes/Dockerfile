ARG version

# Build the operator binary
FROM golang:1.17.5 as builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Copy the go source
COPY kubernetes kubernetes
COPY vendor vendor
COPY lib lib

# Build
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 make -C ./kubernetes go-build

FROM quay.io/gravitational/teleport:${version} as teleport

# Use distroless as minimal base image to package the operator binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/cc-debian11@sha256:2510b5f6a5ca842b863b298ef6b1e423b7b0f1a5343db5e94ffc1943d0e70098
WORKDIR /
COPY --from=builder /workspace/kubernetes/bin/teleport-operator /usr/local/bin/
COPY --from=teleport /usr/local/bin/tctl /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/teleport-operator"]
