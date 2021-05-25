module github.com/gravitational/teleport-plugins

go 1.16

require (
	github.com/Azure/go-autorest/autorest v0.11.10 // indirect
	github.com/go-resty/resty/v2 v2.3.0
	github.com/google/go-querystring v1.0.0
	github.com/gravitational/kingpin v2.1.11-0.20190130013101-742f2714c145+incompatible
	github.com/gravitational/teleport v1.3.3
	github.com/gravitational/teleport/api v0.0.0
	github.com/gravitational/trace v1.1.15
	github.com/hashicorp/go-version v1.2.1
	github.com/jonboulle/clockwork v0.2.2
	github.com/julienschmidt/httprouter v1.3.0
	github.com/mailgun/holster/v3 v3.15.2
	github.com/pborman/uuid v1.2.1
	github.com/pelletier/go-toml v1.8.0
	github.com/sirupsen/logrus v1.8.1-0.20210219125412-f104497f2b21
	github.com/stretchr/testify v1.7.0
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	go.etcd.io/bbolt v1.3.5
	golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83
	google.golang.org/grpc v1.31.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/resty.v1 v1.12.0
)

replace (
	github.com/coreos/go-oidc => github.com/gravitational/go-oidc v0.0.3
	github.com/gogo/protobuf => github.com/gravitational/protobuf v1.3.2-0.20201123192827-2b9fcfaffcbf
	github.com/gravitational/teleport => github.com/gravitational/teleport v1.3.3-0.20210408010938-1115eb1e6cbe
	github.com/gravitational/teleport/api => github.com/gravitational/teleport/api v0.0.0-20210408010938-1115eb1e6cbe
	github.com/iovisor/gobpf => github.com/gravitational/gobpf v0.0.1
	github.com/julienschmidt/httprouter => github.com/rw-access/httprouter v1.3.1-0.20210321233808-98e93175c124
	github.com/siddontang/go-mysql v1.1.0 => github.com/gravitational/go-mysql v1.1.1-0.20210212011549-886316308a77
	google.golang.org/grpc => google.golang.org/grpc v1.29.1
)
