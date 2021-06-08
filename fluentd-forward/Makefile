VERSION=0.0.1

OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
TERRAFORM_ARCH=$(OS)_$(ARCH)

LOCALDIR := $(dir $(CURDIR)/$(word $(words $(MAKEFILE_LIST)),$(MAKEFILE_LIST)))
GENTERRAFORMPATH := $(shell go env GOPATH)/bin

BUILDDIR ?= build

ADDFLAGS ?=
BUILDFLAGS ?= $(ADDFLAGS) -ldflags "-w -s -X main.Sha=`git rev-parse HEAD` -X main.Version=$(VERSION)"
CGOFLAG ?= CGO_ENABLED=1

PASS ?= 1234

CLOUD_ADDR=evilmartians.teleport.sh:443
IDENTITY_FILE=example/keys/identity

.PHONY: gen-example-mtls
gen-example-mtls:
	rm -f example/keys/*

	openssl genrsa -out example/keys/ca.key 4096
	chmod 444 example/keys/ca.key
	openssl req -config example/ssl.conf -key example/keys/ca.key -new -x509 -days 7300 -sha256 -extensions v3_ca -subj "/CN=ca" -out example/keys/ca.crt

	openssl genrsa -aes256 -passout pass:$(PASS) -out example/keys/server.key 4096
	chmod 444 example/keys/server.key
	openssl req -passin pass:$(PASS) -config example/ssl.conf -subj "/CN=server" -key example/keys/server.key -new -out example/keys/server.csr
	openssl x509 -req -in example/keys/server.csr -CA example/keys/ca.crt -CAkey example/keys/ca.key -CAcreateserial -days 365 -out example/keys/server.crt -extfile example/ssl.conf -extensions server_cert

	openssl genrsa -out example/keys/server_nopass.key 4096
	chmod 444 example/keys/server_nopass.key
	openssl req -config example/ssl.conf -subj "/CN=server-nopass" -key example/keys/server_nopass.key -new -out example/keys/server_nopass.csr
	openssl x509 -req -in example/keys/server_nopass.csr -CA example/keys/ca.crt -CAkey example/keys/ca.key -CAcreateserial -days 365 -out example/keys/server_nopass.crt -extfile example/ssl.conf -extensions server_cert

	openssl genrsa -out example/keys/client.key 4096
	chmod 444 example/keys/client.key
	openssl req -config example/ssl.conf -subj "/CN=client" -key example/keys/client.key -new -out example/keys/client.csr
	openssl x509 -req -in example/keys/client.csr -CA example/keys/ca.crt -CAkey example/keys/ca.key -CAcreateserial -days 365 -out example/keys/client.crt -extfile example/ssl.conf -extensions client_cert

.PHONY: clean
clean:
	go clean

.PHONY: build
build: clean
	GOOS=$(OS) GOARCH=$(ARCH) $(CGOFLAG) go build -o $(BUILDDIR)/fluentd-forward $(BUILDFLAGS)

.PHONY: install
install: build
	go install

.PHONY: fluentd
fluentd: gen-example-mtls
	docker run -p 8888:8888 -v $(LOCALDIR)example/keys:/fluentd/etc/keys -v $(LOCALDIR)example/fluent.conf:/fluentd/etc/fluent.conf fluent/fluentd:edge 

.PHONY: example
example: build
	./build/fluentd-forward -c example/keys/client.crt -k example/keys/client.key --fluentd-ca example/keys/ca.crt --fluentd-url https://localhost:8888/test.log --teleport-ca ~/www/teleport-installation/tf.cas --teleport-cert ~/www/teleport-installation/tf.crt --teleport-key ~/www/teleport-installation/tf.key --teleport-addr localhost:3025 --storage /tmp/storage --batch 10 --timeout 10s

.PHONY: example-cloud
example-cloud: build
	./build/fluentd-forward -c example/keys/client.crt -k example/keys/client.key --fluentd-ca example/keys/ca.crt --fluentd-url https://localhost:8888/test.log --teleport-identity $(IDENTITY_FILE) --teleport-addr $(CLOUD_ADDR) --storage /tmp/storage-cloud --batch 10 --timeout 10s --start-time 2020-01-01T00:00:00Z

.PHONY: test
test: gen-example-mtls
	rm -rf tmp && mkdir -p tmp
	go test -coverprofile=cover.out