module github.com/gravitational/teleport-plugins

go 1.16

require (
	github.com/alecthomas/assert v0.0.0-20170929043011-405dbfeb8e38 // indirect
	github.com/alecthomas/colour v0.1.0 // indirect
	github.com/alecthomas/kong v0.2.18
	github.com/alecthomas/repr v0.0.0-20200325044227-4184120f674c // indirect
	github.com/alecthomas/units v0.0.0-20210208195552-ff826a37aa15 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-resty/resty/v2 v2.3.0
	github.com/gobuffalo/flect v0.2.4
	github.com/gogo/protobuf v1.3.2
	github.com/google/go-querystring v1.0.0
	github.com/google/uuid v1.2.0
	github.com/gravitational/kingpin v2.1.11-0.20190130013101-742f2714c145+incompatible
	github.com/gravitational/protoc-gen-terraform v0.0.0-20211108170245-3b37ff28d21e // protoc-gen-terraform master (#13)
	github.com/gravitational/teleport/api v0.0.0-20211213214838-0e304c323c49 // tag v8.0.5
	github.com/gravitational/trace v1.1.16-0.20210609220119-4855e69c89fc
	github.com/hashicorp/go-version v1.3.0
	github.com/hashicorp/terraform-plugin-sdk/v2 v2.8.0
	github.com/jonboulle/clockwork v0.2.2
	github.com/json-iterator/go v1.1.11
	github.com/julienschmidt/httprouter v1.3.0
	github.com/mailgun/holster/v3 v3.15.2
	github.com/mailgun/mailgun-go/v4 v4.5.3
	github.com/manifoldco/promptui v0.8.0
	github.com/pelletier/go-toml v1.8.0
	github.com/peterbourgon/diskv/v3 v3.0.0
	github.com/sethvargo/go-limiter v0.7.2
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	go.etcd.io/bbolt v1.3.6
	go.uber.org/zap v1.19.0
	golang.org/x/crypto v0.0.0-20210421170649-83a5a9bb288b
	golang.org/x/net v0.0.0-20210520170846-37e1c6afe023
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210908233432-aa78b53d3365 // indirect
	golang.org/x/tools v0.1.5 // indirect
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/mail.v2 v2.3.1
	gopkg.in/resty.v1 v1.12.0
	k8s.io/api v0.22.4
	k8s.io/apiextensions-apiserver v0.22.4
	k8s.io/apimachinery v0.22.4
	k8s.io/client-go v0.22.4
	k8s.io/component-base v0.22.4
	sigs.k8s.io/controller-runtime v0.10.3
	sigs.k8s.io/yaml v1.3.0
)

replace (
	github.com/gogo/protobuf => github.com/gravitational/protobuf v1.3.2-0.20201123192827-2b9fcfaffcbf
	github.com/julienschmidt/httprouter => github.com/rw-access/httprouter v1.3.1-0.20210321233808-98e93175c124
)
