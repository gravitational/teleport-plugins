module github.com/gravitational/teleport-plugins/terraform

go 1.15

replace (
	github.com/coreos/go-oidc => github.com/gravitational/go-oidc v0.0.3
	github.com/iovisor/gobpf => github.com/gravitational/gobpf v0.0.1
	github.com/sirupsen/logrus => github.com/gravitational/logrus v0.10.1-0.20171120195323-8ab1e1b91d5f
	google.golang.org/grpc => google.golang.org/grpc v1.27.0
)

require (
	github.com/apparentlymart/go-cidr v1.1.0 // indirect
	github.com/apparentlymart/go-versions v1.0.0 // indirect
	github.com/gravitational/configure v0.0.0-20180808141939-c3428bd84c23 // indirect
	github.com/gravitational/teleport v1.3.3-0.20201201014150-c4583b7a1af6
	github.com/gravitational/trace v1.1.11
	github.com/hashicorp/go-hclog v0.9.2 // indirect
	github.com/hashicorp/hcl/v2 v2.6.0 // indirect
	github.com/hashicorp/terraform v0.13.0-beta1
	github.com/hashicorp/yamux v0.0.0-20181012175058-2f1d1f20f75d // indirect
	github.com/imdario/mergo v0.3.9 // indirect
	github.com/jonboulle/clockwork v0.2.2 // indirect
	github.com/mailgun/minheap v0.0.0-20170619185613-3dbe6c6bf55f // indirect
	github.com/mailgun/timetools v0.0.0-20170619190023-f3a7b8ffff47 // indirect
	github.com/mailgun/ttlmap v0.0.0-20170619185759-c1c17f74874f // indirect
	github.com/mattn/go-sqlite3 v1.14.5 // indirect
	github.com/mitchellh/reflectwalk v1.0.1 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pquerna/otp v1.3.0 // indirect
	github.com/sirupsen/logrus v1.7.0 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/zclconf/go-cty v1.5.1 // indirect
	github.com/zclconf/go-cty-yaml v1.0.2 // indirect
	golang.org/x/crypto v0.0.0-20201124201722-c8d3bf9c5392 // indirect
	golang.org/x/net v0.0.0-20201110031124-69a78807bb2b // indirect
	golang.org/x/sys v0.0.0-20201130171929-760e229fe7c5 // indirect
	golang.org/x/term v0.0.0-20201126162022-7de9c90e9dd1 // indirect
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22 // indirect
)
