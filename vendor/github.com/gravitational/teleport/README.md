# Gravitational Teleport

Gravitational Teleport is a modern security gateway for remotely accessing:
* Clusters of Linux servers via SSH or SSH-over-HTTPS in a browser.
* Kubernetes clusters.

It is intended to be used instead or together with `sshd` for organizations who
need:

* SSH audit with session recording/replay.
* Kubernetes API Access with audit and `kubectl exec` recording/replay.
* Easily manage trust between teams, organizations and data centers.
* Have SSH or Kubernetes access to behind-firewall clusters without any open ports.
* Role-based access control (RBAC) for SSH protocol.
* Unified RBAC for SSH and Kubernetes.

In addition to its hallmark features, Teleport is interesting for smaller teams
because it facilitates easy adoption of the best infrastructure security
practices like:

- No need to distribute keys: Teleport uses certificate-based access with automatic certificate expiration time.
- 2nd factor authentication (2FA) for SSH and Kubernetes.
- Collaboratively troubleshoot issues through session sharing.
- Single sign-on (SSO) for SSH/Kubernetes and your organization identities via
  Github Auth, OpenID Connect or SAML with endpoints like Okta or Active Directory.
- Cluster introspection: every SSH node and its status can be queried via CLI and Web UI.

Teleport is built on top of the high-quality [Golang SSH](https://godoc.org/golang.org/x/crypto/ssh)
implementation and it is _fully compatible with OpenSSH_ and can be used with
`sshd` servers and `ssh` clients.

|Project Links| Description
|---|----
| [Teleport Website](https://gravitational.com/teleport)  | The official website of the project |
| [Documentation](https://gravitational.com/teleport/docs/quickstart/)  | Admin guide, user manual and more |
| [Demo Video](https://www.youtube.com/watch?v=DUlTAlEJr5w) | 5-minute video overview of the UI. |
| [Teleconsole](https://www.teleconsole.com) | The free service to "invite" SSH clients behind NAT, built on top of Teleport |
| [Blog](https://gravitational.com/blog/) | Our blog where we publish Teleport news |
| [Community Forum](https://community.gravitational.com) | Teleport Community Forum|

[![Teleport 4.3 Demo](/docs/4.3/img/readme/teleport-4.3-video-thumb.png)](https://www.youtube.com/watch?v=DUlTAlEJr5w)

## Installing and Running

Download the [latest binary release](https://gravitational.com/teleport/download/),
unpack the .tar.gz and run `sudo ./install`. This will copy Teleport binaries into
`/usr/local/bin`.

Then you can run Teleport as a single-node cluster:

```bash
$ sudo teleport start
```

In a production environment Teleport must run as root. But to play, just do `chown $USER /var/lib/teleport`
and run it under `$USER`, in this case you will not be able to login as someone else though.

## Docker

### Deploy Teleport
If you wish to deploy Teleport inside a Docker container:
```
# This command will pull the Teleport container image for version 4.3
# Replace 4.3 with the version you need:
$ docker pull quay.io/gravitational/teleport:4.3
```
View latest tags on [Quay.io | gravitational/teleport](https://quay.io/repository/gravitational/teleport?tab=tags)

### For Local Testing and Development
Follow instructions at [docker/README](docker/README.md)

## Building Teleport

Teleport source code consists of the actual Teleport daemon binary written in Golang, and also
of a web UI (a git submodule located in /webassets directory) written in Javascript.

Make sure you have Golang `v1.15` or newer, then run:

```bash
# get the source & build:
$ git clone https://github.com/gravitational/teleport.git
$ cd teleport
$ make full

# create the default data directory before starting:
$ sudo mkdir -p -m0700 /var/lib/teleport
$ sudo chown $USER /var/lib/teleport
```

If the build succeeds the binaries will be placed in
`$GOPATH/src/github.com/gravitational/teleport/build`

NOTE: The Go compiler is somewhat sensitive to amount of memory: you will need
at least 1GB of virtual memory to compile Teleport. 512MB instance without swap
will not work.

NOTE: This will build the latest version of Teleport, regardless of whether it is stable. If you want to build the latest stable release, `git checkout` to that tag (e.g. `git checkout v4.3.0`) before running `make full`.

### Rebuilding Web UI

Teleport Web UI is located in the [Gravitational Webapps](https://github.com/gravitational/webapps) monorepo.
You can clone that repository and rebuild teleport UI package with:

```bash
$ git clone git@github.com:gravitational/webapps.git
$ cd webapps
$ make build-teleport
```

Then you can replace Teleport web UI files with the one found in the generated `/dist` folder.

To enable speedy iterations on the Web UI, you can run a
[local web-dev server](https://github.com/gravitational/webapps/tree/master/packages/teleport).

You can also tell teleport to load the web UI assets from the source directory.
To enable this behavior, set the environment variable `DEBUG=1` and rebuild with the default target:

```bash
# Run Teleport as a single-node cluster in development mode:
$ DEBUG=1 ./build/teleport start -d
```

Keep the server running in this mode, and make your UI changes in `/dist` directory.
Refer to [the webapps README](https://github.com/gravitational/webapps/blob/master/README.md) for instructions on how to update the Web UI.

### Updating Documentation

TL;DR version:

```
make docs
make run-docs
```

For more details, take a look at [docs/README](docs/README.md)

### Managing dependencies

Dependencies are managed using [Go
modules](https://blog.golang.org/using-go-modules). Here are instructions for
some common tasks:

#### Add a new dependency

Latest version:

```
go get github.com/new/dependency
# Update the source to actually use this dependency, then run:
make update-vendor
```

Specific version:

```
go get github.com/new/dependency@version
# Update the source to actually use this dependency, then run:
make update-vendor
```

#### Set dependency to a specific version

```
go get github.com/new/dependency@version
make update-vendor
```

#### Update dependency to the latest version

```
go get -u github.com/new/dependency
make update-vendor
```

#### Update all dependencies

```
go get -u all
make update-vendor
```

#### Debugging dependencies

Why is a specific package imported: `go mod why $pkgname`.

Why is a specific module imported: `go mod why -m $modname`.

Why is a specific version of a module imported: `go mod graph | grep $modname`.

## Why did We Build Teleport?

The Teleport creators used to work together at Rackspace. We noticed that most
cloud computing users struggle with setting up and configuring infrastructure
security because popular tools, while flexible, are complex to understand and
expensive to maintain. Additionally, most organizations use multiple
infrastructure form factors such as several cloud providers, multiple cloud
accounts, servers in colocation, and even smart devices. Some of those devices
run on untrusted networks, behind third party firewalls. This only magnifies
complexity and increases operational overhead.

We had a choice, either to start a security consulting business or build a
solution that’s dead-easy to use and understand, something that creates an
illusion of all of your servers being in the same room as you as if they were
magically _teleported_. And Teleport was born!

## More Information

* [Quick Start Guide](https://gravitational.com/teleport/docs/quickstart)
* [Teleport Architecture](https://gravitational.com/teleport/docs/architecture)
* [Admin Manual](https://gravitational.com/teleport/docs/admin-guide)
* [User Manual](https://gravitational.com/teleport/docs/user-manual)
* [FAQ](https://gravitational.com/teleport/docs/faq)

## Support and Contributing

We offer a few different options for support. First of all, we try to provide clear and comprehensive documentation. The docs are also in Github, so feel free to create a PR or file an issue if you think improvements can be made. If you still have questions after reviewing our docs, you can also:

* Join the [Teleport Community](https://community.gravitational.com/c/teleport) to ask questions. Our engineers are available there to help you.
* If you want to contribute to Teleport or file a bug report/issue, you can do so by creating an issue here in Github.
* If you are interested in Teleport Enterprise or more responsive support during a POC, we can also create a dedicated Slack channel for you during your POC. You can [reach out to us through our website](https://gravitational.com/teleport/) to arrange for a POC.

## Is Teleport Secure and Production Ready?

Teleport has completed several security audits from the nationally recognized
technology security companies. [Some](https://gravitational.com/blog/teleport-release-2-2/) of
[them](https://gravitational.com/blog/teleport-security-audit/) have been made public.
We are comfortable with the use of Teleport from a security perspective.

You can see the list of companies who use Teleport in production on the Teleport
[product page](https://gravitational.com/teleport#customerlist).

However, Teleport is still a relatively young product so you may experience
usability issues.  We are actively supporting Teleport and addressing any
issues that are submitted to this repo. Ask questions, send pull requests,
report issues and don't be shy! :)

The latest stable Teleport build can be found in [Releases](https://gravitational.com/teleport/download/)

## Who Built Teleport?

Teleport was created by [Gravitational Inc](https://gravitational.com). We have
built Teleport by borrowing from our previous experiences at Rackspace. It has
been extracted from [Gravity](https://gravitational.com/gravity/), our
Kubernetes distribution optimized for deploying and remotely controlling complex
applications into multiple environments _at the same time_:

* Multiple cloud regions
* Colocation
* Private enterprise clouds located behind firewalls
