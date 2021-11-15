module github.com/gravitational/teleport-plugins/terraform

go 1.15

replace (
	github.com/coreos/go-oidc => github.com/gravitational/go-oidc v0.0.3
	github.com/iovisor/gobpf => github.com/gravitational/gobpf v0.0.1
	github.com/sirupsen/logrus => github.com/gravitational/logrus v1.4.3
)

require (
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/fatih/color v1.10.0 // indirect
	github.com/gogo/protobuf v1.3.2
	github.com/gravitational/protoc-gen-terraform v0.0.0-20211108170245-3b37ff28d21e // master
	github.com/gravitational/teleport/api v0.0.0-20211110023606-aabae26c36b2 // 8.0.0-rc.1: TODO: Replace with 8.0.0 once released
	github.com/gravitational/trace v1.1.15
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-hclog v0.16.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-uuid v1.0.2 // indirect
	github.com/hashicorp/hcl/v2 v2.9.1 // indirect
	github.com/hashicorp/terraform-plugin-sdk/v2 v2.8.0
	github.com/hashicorp/yamux v0.0.0-20210316155119-a95892c5f864 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.3 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/sirupsen/logrus v1.8.1
	golang.org/x/net v0.0.0-20210415231046-e915ea6b2b7d // indirect
	golang.org/x/term v0.0.0-20210406210042-72f3dc4e9b72 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20210416161957-9910b6c460de // indirect
	google.golang.org/grpc v1.37.0
	google.golang.org/protobuf v1.26.0
)
