<div align="center">
   <img src="https://goteleport.com/blog/images/2020/gravitational-is-teleport-header.png" width=750/>
   <div align="center" style="padding: 25px">
      <a href="https://goteleport.com/docs/">
      <img src="https://img.shields.io/badge/Teleport-7.0-651FFF.svg" />
      </a>
      <a href="https://golang.org/">
      <img src="https://img.shields.io/badge/Go-1.16-7fd5ea.svg" />
      </a>
      <a href="https://github.com/gravitational/teleport/blob/master/CODE_OF_CONDUCT.md">
      <img src="https://img.shields.io/badge/Contribute-🙌-green.svg" />
      </a>
      <a href="https://www.apache.org/licenses/LICENSE-2.0">
      <img src="https://img.shields.io/badge/Apache-2.0-red.svg" />
      </a>
   </div>
</div>
</br>

> Read our Blog: https://goteleport.com/blog/

> Read our Documentation: https://goteleport.com/docs/getting-started/

## Table of Contents

1. [Introduction](#Introduction)
1. [Installing and Running](#Installing-and-Running)
1. [Docker](#Docker)
1. [Building Teleport](#Building-Teleport)
1. [Why did We Build Teleport?](#Why-did-We-Build-Teleport?)
1. [More Information](#More-Information)
1. [Support and Contributing](#Support-and-Contributing)
1. [Is Teleport Secure and Production Ready?](#Is-Teleport-Secure-and-Production-Ready?)
1. [Who Built Teleport?](#Who-Built-Teleport?)

## Introduction

Teleport is an identity-aware, multi-protocol access proxy which understands
SSH, HTTPS, Kubernetes API, MySQL, and PostgreSQL wire protocols.

On the server-side, Teleport is a single binary which enables convenient secure
access to behind-NAT resources such as:

* [SSH nodes](https://goteleport.com/teleport/docs/quickstart/) - SSH works in browsers too!
* [Kubernetes clusters](https://goteleport.com/teleport/docs/kubernetes-access/)
* [PostgreSQL and MySQL databases](https://goteleport.com/teleport/docs/database-access/)
* [Internal Web apps](https://goteleport.com/teleport/docs/application-access/)

Teleport is trivial to set up as a Linux daemon or in a Kubernetes pod. It's rapidly
replacing legacy `sshd`-based setups at organizations who need:

* Developer convenience of having instant secure access to everything they need
  across many environments and cloud providers.
* Audit log with session recording/replay for multiple protocols
* Easily manage trust between teams, organizations and data centers.
* Role-based access control (RBAC) and flexible access workflows (one-time access requests)

In addition to its hallmark features, Teleport is interesting for smaller teams
because it facilitates easy adoption of the best infrastructure security
practices like:

- No need to manage shared secrets such as SSH keys: Teleport uses certificate-based access with automatic certificate expiration time for all protocols.
- Two-factor authentication (2FA) for everything.
- Collaboratively troubleshoot issues through session sharing.
- Single sign-on (SSO) for everything via Github Auth, OpenID Connect, or SAML with endpoints like Okta or Active Directory.
- Infrastructure introspection: Use Teleport via the CLI or Web UI to view the status of every SSH node, database instance, Kubernetes cluster, or internal web app.

Teleport is built upon the high-quality [Golang SSH](https://godoc.org/golang.org/x/crypto/ssh)
implementation. It is _fully compatible with OpenSSH_,
`sshd` servers, and `ssh` clients.

|Project Links| Description
|---|----
| [Teleport Website](https://goteleport.com/teleport) | The official website of the project. |
| [Documentation](https://goteleport.com/teleport/docs/quickstart/) | Admin guide, user manual and more. |
| [Demo Video](https://www.youtube.com/watch?v=0HlyGk8dihM) | 5-minute video overview of the UI. |
| [Blog](https://goteleport.com/blog/) | Our blog where we publish Teleport news. |
| [Forum](https://github.com/gravitational/teleport/discussions) | Ask us a setup question, post your tutorial, feedback, or idea on our forum. |
| [Slack](https://goteleport.com/slack) | Need help with your setup? Ping us in our Slack channel. |
| [Cloud-hosted](https://goteleport.com/pricing) | We offer Teleport Pro and Enteprise with a Cloud-hosted option. For teams that require easy and secure access to their computing environments. |


[Teleport 6.0 - 4:00m Demo Video](https://www.youtube.com/watch?v=0HlyGk8dihM)

## Installing and Running

Download the [latest binary release](https://goteleport.com/teleport/download),
unpack the .tar.gz and run `sudo ./install`. This will copy Teleport binaries into
`/usr/local/bin`.

Then you can run Teleport as a single-node cluster:

```bash
$ sudo teleport start
```

In a production environment, Teleport must run as `root`. For testing or non-production environments, run it as the `$USER`:

`chown $USER /var/lib/teleport`

* In this case, you will not be able to log in as another user.

## Docker

### Deploy Teleport

If you wish to deploy Teleport inside a Docker container:
```
# This command will pull the Teleport container image for version 6
$ docker pull quay.io/gravitational/teleport:6
```
View latest tags on [Quay.io | gravitational/teleport](https://quay.io/repository/gravitational/teleport?tab=tags)

### For Local Testing and Development

Follow the instructions in the [docker/README](docker/README.md) file.

## Building Teleport

The Teleport source code contains the Teleport daemon binary written in Golang and a web UI written in Javascript (a git submodule located in the `/webassets` directory).

Make sure you have Golang `v1.16` or newer, then run:

```bash
# get the source & build:
$ git clone https://github.com/gravitational/teleport.git
$ cd teleport
$ make full

# create the default data directory before starting:
$ sudo mkdir -p -m0700 /var/lib/teleport
$ sudo chown $USER /var/lib/teleport
```

If the build succeeds, the installer places the binaries in the following directory:
`$GOPATH/src/github.com/gravitational/teleport/build`

**Important:**
* The Go compiler is somewhat sensitive to the amount of memory: you will need **at least** 1GB of virtual memory to compile Teleport. A 512MB instance without swap will **not** work.
* This will build the latest version of Teleport, **regardless** of whether it is stable. If you want to build the latest stable release, run `git checkout` to the corresponding tag (for example, run `git checkout v6.0.0`) **before** running `make full`.

### Web UI

The Teleport Web UI resides in the [Gravitational Webapps](https://github.com/gravitational/webapps) repo.

#### Rebuilding Web UI for development

To clone this repository and rebuild the Teleport UI package, run the following commands:

```bash
$ git clone git@github.com:gravitational/webapps.git
$ cd webapps
$ make build-teleport
```

Then you can replace Teleport Web UI files with the files from the newly-generated `/dist` folder.

To enable speedy iterations on the Web UI, you can run a
[local web-dev server](https://github.com/gravitational/webapps/tree/master/packages/teleport).

You can also tell Teleport to load the Web UI assets from the source directory.
To enable this behavior, set the environment variable `DEBUG=1` and rebuild with the default target:

```bash
# Run Teleport as a single-node cluster in development mode:
$ DEBUG=1 ./build/teleport start -d
```

Keep the server running in this mode, and make your UI changes in `/dist` directory.
For instructions about how to update the Web UI, read [the `webapps` README](https://github.com/gravitational/webapps/blob/master/README.md.) file.

#### Updating Web UI assets

After you commit a change to [the `webapps`
repo](https://github.com/gravitational/webapps), you need to update the Web UI
assets in the `webassets/` git submodule.

Run `make update-webassets` to update the `webassets` repo and create a PR for
`teleport` to update its git submodule.

You will need to have the `gh` utility installed on your system for the script
to work. For installation instructions, read the [GitHub CLI installation](https://github.com/cli/cli/releases/latest) documentation.

### Updating Documentation

TL;DR version:

```bash
make docs
make run-docs
```

For more details, read the [docs/README](docs/README.md) file.

### Managing dependencies

All dependencies are managed using [Go modules](https://blog.golang.org/using-go-modules). Here are the instructions for some common tasks:

#### Add a new dependency

Latest version:

```bash
go get github.com/new/dependency
```

Update the source to use this dependency, then run:

```bash
make update-vendor
```

Specific version:

```bash
go get github.com/new/dependency@version
```

Update the source to use this dependency, then run:

```bash
make update-vendor
```

#### Set dependency to a specific version

```bash
go get github.com/new/dependency@version
make update-vendor
```

#### Update dependency to the latest version

```bash
go get -u github.com/new/dependency
make update-vendor
```

#### Update all dependencies

```bash
go get -u all
make update-vendor
```

#### Debugging dependencies

Why is a specific package imported?

`go mod why $pkgname`

Why is a specific module imported?

`go mod why -m $modname`

Why is a specific version of a module imported?

`go mod graph | grep $modname`

## Why did We Build Teleport?

The Teleport creators used to work together at Rackspace. We noticed that most cloud computing users struggle with setting up and configuring infrastructure security because popular tools, while flexible, are complex to understand and expensive to maintain. Additionally, most organizations use multiple infrastructure form factors such as several cloud providers, multiple cloud accounts, servers in colocation, and even smart devices. Some of those devices run on untrusted networks, behind third-party firewalls. This only magnifies complexity and increases operational overhead.

We had a choice, either start a security consulting business or build a solution that's dead-easy to use and understand. A real-time representation of all of your servers in the same room as you, as if they were magically _teleported_. Thus, Teleport was born!

## More Information

* [Quick Start Guide](https://goteleport.com/teleport/docs/quickstart)
* [Teleport Architecture](https://goteleport.com/teleport/docs/architecture)
* [Admin Manual](https://goteleport.com/teleport/docs/admin-guide)
* [User Manual](https://goteleport.com/teleport/docs/user-manual)
* [FAQ](https://goteleport.com/teleport/docs/faq)

## Support and Contributing

We offer a few different options for support. First of all, we try to provide clear and comprehensive documentation. The docs are also in Github, so feel free to create a PR or file an issue if you have ideas for improvements. If you still have questions after reviewing our docs, you can also:

* Join [Teleport Discussions](https://github.com/gravitational/teleport/discussions) to ask questions. Our engineers are available there to help you.
* If you want to contribute to Teleport or file a bug report/issue, you can create an issue here in Github.
* If you are interested in Teleport Enterprise or more responsive support during a POC, we can also create a dedicated Slack channel for you during your POC. You can [reach out to us through our website](https://goteleport.com/teleport/) to arrange for a POC.

## Is Teleport Secure and Production Ready?

Teleport has completed several security audits from the nationally recognized
technology security companies. [Some](https://goteleport.com/blog/teleport-release-2-2/) of
[them](https://goteleport.com/blog/teleport-security-audit/) have been made public.
We are comfortable with the use of Teleport from a security perspective.

You can see the list of companies who use Teleport in production on the Teleport
[product page](https://goteleport.com/case-study/).

However, Teleport is still a relatively young product, so you may experience usability issues.  We actively support Teleport and address any issues that users submit to this repo. Ask questions, send pull requests,
report issues, and don't be shy! :)

You can find the latest stable Teleport build on our [Releases](https://goteleport.com/teleport/download) page.

## Who Built Teleport?

Teleport was created by [Gravitational Inc](https://goteleport.com). We have
built Teleport by borrowing from our previous experiences at Rackspace. It has
been extracted from [Gravity](https://goteleport.com/gravity), our
Kubernetes distribution optimized for deploying and remotely controlling complex
applications into multiple environments _at the same time_:

* Multiple cloud regions
* Colocation
* Private enterprise clouds located behind firewalls
