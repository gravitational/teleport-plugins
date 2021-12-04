VERSION=8.0.0

OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
TERRAFORM_ARCH=$(OS)_$(ARCH)

LOCALDIR := $(dir $(CURDIR)/$(word $(words $(MAKEFILE_LIST)),$(MAKEFILE_LIST)))
BUILDDIR ?= build

CUSTOM_WASMER_RUNTIME_DIR ?=
ADDFLAGS ?=
BUILDFLAGS ?= $(ADDFLAGS) -ldflags "-w -s -X main.Sha=`git rev-parse HEAD` -X main.Version=$(VERSION)" 
CGOFLAG ?= CGO_ENABLED=1

# Custom wasmer runtime is required for Apple M1.
#
# git clone git@github.com:wasmerio/wasmer-go.git && cd wasmer-go && cargo build --release && cd ..
# CUSTOM_WASMER_RUNTIME_DIR=$(pwd)/wasmer-go make build
ifneq ($(CUSTOM_WASMER_RUNTIME_DIR),)
	CGO_CFLAGS ?= "-I$(CUSTOM_WASMER_RUNTIME_DIR)/wasmer/packaged/include/"
	CGO_LDFLAGS ?= "-Wl,-rpath,$(CUSTOM_WASMER_RUNTIME_DIR)/target/release/ -L$(CUSTOM_WASMER_RUNTIME_DIR)/target/release/ -lwasmer_go"
	TAGS ?= --tags custom_wasmer_runtime 
endif

# Args for make gen-example-mtls
PASS ?= 1234
KEYLEN ?= 1024

RELEASE = teleport-event-handler-v$(VERSION)-$(OS)-$(ARCH)-bin

.PHONY: build
build: clean
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) CGO_CFLAGS=$(CGO_CFLAGS) CGO_LDFLAGS=$(CGO_LDFLAGS) go build -o $(BUILDDIR)/teleport-event-handler $(BUILDFLAGS) $(TAGS)

.PHONY: release
release: build
	tar -C $(BUILDDIR) -czf $(RELEASE).tar.gz .

.PHONY: clean
clean:
	go clean

.PHONY: install
install: build
	go install

.PHONY: test
test: gen-example-mtls
	rm -rf tmp && mkdir -p tmp
	go test -coverprofile=cover.out

.PHONY: configure
configure: build
	rm -rf tmp/*
	./build/teleport-event-handler configure tmp
	tctl create -f tmp/teleport-event-handler-role.yaml
	tctl auth sign --out tmp/identity --user teleport-event-handler

.PHONY: fluentd
fluentd:
	docker run -p 8888:8888 -v $(LOCALDIR)tmp:/keys -v $(LOCALDIR)tmp/fluent.conf:/fluentd/etc/fluent.conf fluent/fluentd:edge 

.PHONY: example
example: build
	./build/teleport-event-handler start --config tmp/teleport-event-handler.toml --start-time 2020-01-01T00:00:00Z

.PHONY: gen-example-mtls
gen-example-mtls:
	mkdir -p example/keys
	rm -f example/keys/*

	openssl genrsa -out example/keys/ca.key $(KEYLEN)
	chmod 444 example/keys/ca.key
	openssl req -config example/ssl.conf -key example/keys/ca.key -new -x509 -days 7300 -sha256 -extensions v3_ca -subj "/CN=ca" -out example/keys/ca.crt

	openssl genrsa -aes256 -passout pass:$(PASS) -out example/keys/server.key $(KEYLEN)
	chmod 444 example/keys/server.key
	openssl req -passin pass:$(PASS) -config example/ssl.conf -subj "/CN=server" -key example/keys/server.key -new -out example/keys/server.csr
	openssl x509 -req -in example/keys/server.csr -CA example/keys/ca.crt -CAkey example/keys/ca.key -CAcreateserial -days 365 -out example/keys/server.crt -extfile example/ssl.conf -extensions server_cert

	openssl genrsa -out example/keys/server_nopass.key $(KEYLEN)
	chmod 444 example/keys/server_nopass.key
	openssl req -config example/ssl.conf -subj "/CN=server-nopass" -key example/keys/server_nopass.key -new -out example/keys/server_nopass.csr
	openssl x509 -req -in example/keys/server_nopass.csr -CA example/keys/ca.crt -CAkey example/keys/ca.key -CAcreateserial -days 365 -out example/keys/server_nopass.crt -extfile example/ssl.conf -extensions server_cert

	openssl genrsa -out example/keys/client.key $(KEYLEN)
	chmod 444 example/keys/client.key
	openssl req -config example/ssl.conf -subj "/CN=client" -key example/keys/client.key -new -out example/keys/client.csr
	openssl x509 -req -in example/keys/client.csr -CA example/keys/ca.crt -CAkey example/keys/ca.key -CAcreateserial -days 365 -out example/keys/client.crt -extfile example/ssl.conf -extensions client_cert


