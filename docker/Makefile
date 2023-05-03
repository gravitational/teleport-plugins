# teleport-plugins docker flow makefile
# Based on Teleport's Docker Makefile
# https://github.com/gravitational/teleport/blob/master/docker/Makefile

# source directory in the build box
SRCDIR ?= /go/src/github.com/gravitational/teleport-plugins
MKFILE_PATH := $(abspath $(lastword $(MAKEFILE_LIST)))

# Teleport Enterprise Binary release name. Can be any teleport
# enterprise version, but has to be available on get.gravitational.com
RELEASE ?= teleport-ent-v6.0.2-linux-amd64-bin

RUNTIME ?= go1.20.4
BBOX ?= quay.io/gravitational/teleport-buildbox:$(RUNTIME)

# Teleport CLI and plugins CLI flags to pass to them on start
# This is used in insecure-up and debug to pass
# --debug and --insecure-no-tls flags
TELEPORT_FLAGS ?=
PLUGIN_FLAGS ?=

# Which plugins to run on make up?
PLUGINS ?= teleport-slack teleport-jira teleport-mattermost teleport-pagerduty teleport-webhooks

#
# Default target starts a teleport cluster with a single node,
# provisions all the required configurations for users and roles,
# and then starts all the plugins.
#
.PHONY: up
up: down
	TELEPORT_FLAGS=${TELEPORT_FLAGS} PLUGIN_FLAGS=${PLUGIN_FLAGS} docker-compose up teleport ${PLUGINS}

# 'make down' stops all Teleport containers, deletes them
# and their network
#
.PHONY:down
down:
	docker-compose down

# `make enter-teleport` gives you shell inside auth server
# of cluster "one"
#
.PHONY:enter-teleport
enter-teleport:
	docker-compose run teleport /bin/bash

# builds teleport:latest docker image
# Using the ../../teleport's docker instructions
# Note: This doesn't build Teleport itself. If you need to build teleport,
# go to teleport repo root, and run make -c build.assets
#
.PHONY: teleport-ent
teleport-ent:
	docker build \
		-t teleport-ent:latest \
		-f ./teleport/Dockerfile \
		--build-arg RELEASE=$(RELEASE) \
		.

# `make config` provisions configs for the `teleport cluster:
# 1. Creates the role and user for tests
# 2. Creates the role and user for plugins to run with
# 3. Exports the access-plugin certs so that we can later use them for plugins.
#
.PHONY: config
config:
	docker-compose up -d teleport

	# ----> Creating dummy roles for requests and plugins to use.
	docker-compose exec teleport /bin/bash -c "tctl create -f \$$PLUGINSHOME/docker/teleport/foo-role.yaml"
	docker-compose exec teleport /bin/bash -c "tctl create -f \$$PLUGINSHOME/docker/teleport/access-plugin-user-role.yaml"

	# ----> Exporting certificates.
	docker-compose exec teleport /bin/bash -c "tctl auth sign --format=tls --user=access-plugin --out=plug"
	docker-compose exec teleport /bin/bash -c "mkdir -p /mnt/shared/certs/access-plugin && mv plug.* /mnt/shared/certs/access-plugin/"

	docker-compose stop teleport

.PHONY: foo-certs
foo-certs:
	docker-compose up -d teleport
	docker-compose exec teleport /bin/bash -c "tctl auth sign --format=tls --user=foo --out=auth"
	docker-compose exec teleport /bin/bash -c "rm -rf /root/go/src/github.com/gravitational/teleport-plugins/docker/tmp/ && mkdir /root/go/src/github.com/gravitational/teleport-plugins/docker/tmp/ && mv auth.* /root/go/src/github.com/gravitational/teleport-plugins/docker/tmp/"
	docker-compose stop teleport

# Builds teleport-plugins:latest docker image that will run plugins in it.
#
.PHONY: plugins
plugins: build-plugins
	docker build \
		-t teleport-plugins:latest \
		-f ./plugins/Dockerfile \
		--build-arg RUNTIME=$(RUNTIME) \
		--build-arg BBOX=$(BBOX) \
		.

# Builds all of the extensions inside the build box docker container.
#
.PHONY: build-plugins
build-plugins:
	docker run \
		-v $(MKFILE_PATH)/../../:$(SRCDIR) \
		$(BBOX) \
			make -C $(SRCDIR) build-all

.PHONY: clean
clean:
	rm -rf data
