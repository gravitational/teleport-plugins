module github.com/gravitational/teleport-plugins/terraform

go 1.15

replace (
	github.com/coreos/go-oidc => github.com/gravitational/go-oidc v0.0.3
	github.com/iovisor/gobpf => github.com/gravitational/gobpf v0.0.1
	github.com/sirupsen/logrus => github.com/gravitational/logrus v0.10.1-0.20171120195323-8ab1e1b91d5f
	google.golang.org/grpc => google.golang.org/grpc v1.27.0
)

require (
	github.com/coreos/go-oidc v0.0.0-00010101000000-000000000000 // indirect
	github.com/docker/docker v1.13.1 // indirect
	github.com/gokyle/hotp v0.0.0-20160218004637-c180d57d286b // indirect
	github.com/gravitational/configure v0.0.0-20180808141939-c3428bd84c23 // indirect
	github.com/gravitational/form v0.0.0-20151109031454-c4048f792f70 // indirect
	github.com/gravitational/kingpin v2.1.10+incompatible // indirect
	github.com/gravitational/oxy v0.0.0-20200916204440-3eb06d921a1d // indirect
	github.com/gravitational/roundtrip v1.0.0 // indirect
	github.com/gravitational/teleport v4.3.8+incompatible
	github.com/gravitational/trace v1.1.11
	github.com/hashicorp/terraform v0.13.5
	github.com/mailgun/minheap v0.0.0-20170619185613-3dbe6c6bf55f // indirect
	github.com/mailgun/timetools v0.0.0-20170619190023-f3a7b8ffff47 // indirect
	github.com/mailgun/ttlmap v0.0.0-20170619185759-c1c17f74874f // indirect
	github.com/mattn/go-sqlite3 v1.14.5 // indirect
	github.com/mdp/rsc v0.0.0-20160131164516-90f07065088d // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pquerna/otp v1.3.0 // indirect
	github.com/russellhaering/gosaml2 v0.0.0-20170515204909-8908227c114a
	github.com/russellhaering/goxmldsig v1.1.0 // indirect
	github.com/tstranex/u2f v0.0.0-20160508205855-eb799ce68da4
	github.com/vulcand/predicate v1.1.0 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22 // indirect
)
