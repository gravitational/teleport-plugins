module github.com/gravitational/teleport-plugins/terraform

go 1.15

replace (
	github.com/coreos/go-oidc => github.com/gravitational/go-oidc v0.0.3
	github.com/iovisor/gobpf => github.com/gravitational/gobpf v0.0.1
	github.com/sirupsen/logrus => github.com/gravitational/logrus v1.4.3
	google.golang.org/grpc => google.golang.org/grpc v1.27.0
)

require (
	github.com/gravitational/configure v0.0.0-20180808141939-c3428bd84c23 // indirect
	github.com/gravitational/teleport v1.3.3-0.20210121233412-127693c315fb
	github.com/gravitational/trace v1.1.13
	github.com/hashicorp/terraform v0.14.5
	github.com/hashicorp/terraform-plugin-sdk/v2 v2.4.1
	github.com/hashicorp/yamux v0.0.0-20200609203250-aecfd211c9ce // indirect
	github.com/mailgun/minheap v0.0.0-20170619185613-3dbe6c6bf55f // indirect
	github.com/mailgun/timetools v0.0.0-20170619190023-f3a7b8ffff47 // indirect
	github.com/mailgun/ttlmap v0.0.0-20170619185759-c1c17f74874f // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pquerna/otp v1.3.0 // indirect
	github.com/sirupsen/logrus v1.7.0 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad // indirect
	golang.org/x/net v0.0.0-20210119194325-5f4716e94777 // indirect
	golang.org/x/sys v0.0.0-20210119212857-b64e53b001e4 // indirect
	golang.org/x/term v0.0.0-20201210144234-2321bbc49cbf // indirect
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22 // indirect
)
