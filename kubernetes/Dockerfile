# Build the operator binary
FROM golang:1.17 as builder
ARG version=9.2.4

WORKDIR /dependencies
# We need the tctl library
RUN curl https://get.gravitational.com/teleport-v${version}-linux-amd64-bin.tar.gz | tar -xz
RUN cp teleport/tctl /dependencies

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY kubernetes/main.go kubernetes/main.go
COPY kubernetes/apis/ kubernetes/apis/
COPY kubernetes/controllers/ kubernetes/controllers/
COPY kubernetes/sidecar/ kubernetes/sidecar/
COPY lib/ lib/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o teleport-operator kubernetes/main.go

# Use distroless as minimal base image to package the operator binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# we had to switch to cc instead of static or base because we need libgcc_s.so.1 for the tctl binary
FROM gcr.io/distroless/cc
WORKDIR /
COPY --from=builder /workspace/teleport-operator .
COPY --from=builder /dependencies/tctl /usr/local/bin/tctl

ENTRYPOINT ["/teleport-operator"]
