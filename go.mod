module github.com/gravitational/teleport-plugins

go 1.16

require (
	github.com/alecthomas/assert v0.0.0-20170929043011-405dbfeb8e38 // indirect
	github.com/alecthomas/colour v0.1.0 // indirect
	github.com/alecthomas/kong v0.2.17
	github.com/alecthomas/repr v0.0.0-20200325044227-4184120f674c // indirect
	github.com/alecthomas/units v0.0.0-20210208195552-ff826a37aa15 // indirect
	github.com/go-resty/resty/v2 v2.3.0
	github.com/gobuffalo/flect v0.2.5
	github.com/gogo/protobuf v1.3.2
	github.com/google/go-querystring v1.0.0
	github.com/google/uuid v1.2.0
	github.com/gravitational/kingpin v2.1.11-0.20190130013101-742f2714c145+incompatible
	github.com/gravitational/teleport/api v0.0.0-20220406233052-f577413d3c2a // tag v9.0.4
	github.com/gravitational/trace v1.1.17
	github.com/hashicorp/go-version v1.3.0
	github.com/hashicorp/terraform-plugin-framework v0.6.1
	github.com/hashicorp/terraform-plugin-go v0.8.0
	github.com/hashicorp/terraform-plugin-sdk/v2 v2.10.1
	github.com/jonboulle/clockwork v0.2.2
	github.com/json-iterator/go v1.1.12
	github.com/julienschmidt/httprouter v1.3.0
	github.com/mailgun/holster/v3 v3.15.2
	github.com/mailgun/mailgun-go/v4 v4.5.3
	github.com/manifoldco/promptui v0.8.0
	github.com/pelletier/go-toml v1.9.3
	github.com/peterbourgon/diskv/v3 v3.0.0
	github.com/sethvargo/go-limiter v0.7.2
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.19.1
	golang.org/x/crypto v0.0.0-20220126234351-aa10faf2a1f8
	golang.org/x/net v0.0.0-20220127200216-cd36cc0744dd
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	google.golang.org/grpc v1.45.0
	google.golang.org/protobuf v1.27.1
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/mail.v2 v2.3.1
	k8s.io/api v0.23.6
	k8s.io/apiextensions-apiserver v0.23.6
	k8s.io/apimachinery v0.23.6
	k8s.io/client-go v0.23.6
	k8s.io/component-base v0.23.6
	sigs.k8s.io/controller-runtime v0.11.2
	sigs.k8s.io/yaml v1.3.0
)

replace (
	github.com/gogo/protobuf => github.com/gravitational/protobuf v1.3.2-0.20201123192827-2b9fcfaffcbf
	github.com/julienschmidt/httprouter => github.com/rw-access/httprouter v1.3.1-0.20210321233808-98e93175c124
)
