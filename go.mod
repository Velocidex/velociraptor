module www.velocidex.com/golang/velociraptor

require (
	cloud.google.com/go/pubsub v1.50.1
	cloud.google.com/go/storage v1.58.0
	github.com/Depado/bfchroma v1.3.0
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Showmax/go-fqdn v1.0.0
	github.com/Velocidex/amsi v0.0.0-20250418124629-ea341d1aa3f2
	github.com/Velocidex/etw v0.0.0-20251027041548-6d97883fd588
	github.com/Velocidex/go-elasticsearch/v7 v7.3.1-0.20191001125819-fee0ef9cac6b
	github.com/Velocidex/go-magic v0.0.0-20250203094020-32f94b14f00f
	github.com/Velocidex/go-yara v1.1.10-0.20250823152352-e5fc0843e50e
	github.com/Velocidex/grpc-go-pool v1.2.2-0.20241016164850-ff0cb80037a8
	github.com/Velocidex/json v0.0.0-20220224052537-92f3c0326e5a
	github.com/Velocidex/pkcs7 v0.0.0-20230220112103-d4ed02e1862a
	github.com/Velocidex/sflags v0.3.1-0.20241126160332-cc1a5b66b8f1
	github.com/Velocidex/ttlcache/v2 v2.9.1-0.20240517145123-a3f45e86e130
	github.com/Velocidex/yaml/v2 v2.2.8
	github.com/Velocidex/zip v0.0.0-20251027040802-582e676739bd
	github.com/alecthomas/assert v1.0.0
	github.com/alecthomas/chroma v0.7.3
	github.com/alecthomas/participle v0.7.1
	github.com/alecthomas/repr v0.5.2 // indirect
	github.com/alexmullins/zip v0.0.0-20180717182244-4affb64b04d0
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de
	github.com/clbanning/mxj v1.8.4
	github.com/crewjam/saml v0.4.14
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/dustin/go-humanize v1.0.1
	github.com/elastic/go-elasticsearch/v7 v7.3.0 // indirect
	github.com/go-ole/go-ole v1.2.6
	github.com/go-sql-driver/mysql v1.7.1
	github.com/golang/mock v1.6.0
	github.com/google/btree v1.1.2
	github.com/google/rpmpack v0.5.0
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/google/uuid v1.6.0
	github.com/gorilla/csrf v1.7.3
	github.com/gorilla/schema v1.4.1
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.18.0
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hillu/go-ntdll v0.0.0-20220801201350-0d23f057ef1f
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/jonboulle/clockwork v0.3.0 // indirect
	github.com/juju/ratelimit v1.0.1
	github.com/lib/pq v1.10.9
	github.com/magefile/mage v1.15.0
	github.com/mattn/go-isatty v0.0.20
	github.com/mattn/go-pointer v0.0.0-20180825124634-49522c3f3791
	github.com/mattn/go-sqlite3 v1.14.33
	github.com/microcosm-cc/bluemonday v1.0.23
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/panicwrap v1.0.0
	github.com/olekukonko/tablewriter v0.0.5
	github.com/oschwald/maxminddb-golang v1.8.0
	github.com/pkg/sftp v1.13.6
	github.com/prometheus/client_golang v1.15.1
	github.com/prometheus/client_model v0.6.2
	github.com/qri-io/starlib v0.5.0
	github.com/rifflock/lfshook v0.0.0-20180920164130-b9218ef580f5
	github.com/robertkrimen/otto v0.3.0
	github.com/russross/blackfriday/v2 v2.1.0
	github.com/sebdah/goldie/v2 v2.8.0
	github.com/sergi/go-diff v1.4.0
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.11.1
	github.com/xor-gate/ar v0.0.0-20170530204233-5c72ae81e2b7 // indirect
	github.com/xor-gate/debpkg v1.0.0
	go.starlark.net v0.0.0-20230925163745-10651d5192ab
	golang.org/x/crypto v0.46.0
	golang.org/x/mod v0.31.0
	golang.org/x/net v0.48.0
	golang.org/x/sys v0.40.0
	golang.org/x/text v0.32.0
	golang.org/x/time v0.14.0
	google.golang.org/api v0.258.0
	google.golang.org/genproto v0.0.0-20251222181119-0a764e51fe1b // indirect
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
	gopkg.in/sourcemap.v1 v1.0.5 // indirect
	howett.net/plist v1.0.0
	www.velocidex.com/golang/evtx v0.2.1-0.20251107004037-f9f4e4ed0236
	www.velocidex.com/golang/go-ese v0.2.1-0.20250215160921-5af66dc0f6ed
	www.velocidex.com/golang/go-ntfs v0.2.1-0.20250322152626-3c09d909d740
	www.velocidex.com/golang/go-pe v0.1.1-0.20251107001057-f93001158cd9
	www.velocidex.com/golang/go-prefetch v0.0.0-20251027080408-85407689d0cb
	www.velocidex.com/golang/oleparse v0.0.0-20250312121321-f7c2b4ec0959
	www.velocidex.com/golang/regparser v0.0.0-20250203141505-31e704a67ef7
	www.velocidex.com/golang/vfilter v0.0.0-20250915140904-eb07b966bcef
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.6.0
	github.com/Masterminds/semver/v3 v3.4.0
	github.com/Masterminds/sprig/v3 v3.2.2
	github.com/Velocidex/WinPmem/go-winpmem v0.0.0-20251014141320-f59d776a3def
	github.com/Velocidex/disklru v0.0.0-20260204055416-324452f39e80
	github.com/Velocidex/file-rotatelogs v0.0.0-20211221020724-d12e4dae4e11
	github.com/Velocidex/fileb0x v1.1.2-0.20251122021447-f71ad8da5502
	github.com/Velocidex/go-ewf v0.0.0-20240210123447-97dc81b7d8c3
	github.com/Velocidex/go-ext4 v0.0.0-20250510085914-b0b955af0359
	github.com/Velocidex/go-fat v0.0.0-20230923165230-3e6c4265297a
	github.com/Velocidex/go-journalctl v0.0.0-20250902002606-881a5f66df10
	github.com/Velocidex/go-mscfb v0.0.0-20240618091452-31f4ccc54002
	github.com/Velocidex/go-vhdx v0.0.0-20250511013458-5cba970cdeda
	github.com/Velocidex/go-vmdk v0.0.0-20250505140221-bd4633ce2fbf
	github.com/Velocidex/grok v0.0.1
	github.com/Velocidex/ordereddict v0.0.0-20250821063524-02dc06e46238
	github.com/Velocidex/sigma-go v0.0.0-20241113062227-c1c5ea4b5250
	github.com/Velocidex/tracee_velociraptor v0.0.0-20260113161018-f7c951ffeab2
	github.com/Velocidex/velociraptor-site-search v0.0.0-20260205051915-8d028f2816e8
	github.com/Velocidex/yara-x-go v0.0.0-20251010010632-d8eaad9c539c
	github.com/VirusTotal/gyp v0.9.1-0.20231202132633-bb35dbf177a6
	github.com/alecthomas/kingpin/v2 v2.4.0
	github.com/alitto/pond/v2 v2.1.6
	github.com/andybalholm/brotli v1.0.5
	github.com/aws/aws-sdk-go-v2 v1.25.2
	github.com/aws/aws-sdk-go-v2/config v1.27.6
	github.com/aws/aws-sdk-go-v2/credentials v1.17.6
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.16.8
	github.com/aws/aws-sdk-go-v2/service/s3 v1.51.3
	github.com/blevesearch/bleve v1.0.14
	github.com/charmbracelet/huh v0.6.0
	github.com/charmbracelet/lipgloss v1.0.0
	github.com/clayscode/Go-Splunk-HTTP/splunk/v2 v2.0.1-0.20221027171526-76a36be4fa02
	github.com/coreos/go-oidc/v3 v3.11.0
	github.com/elastic/go-libaudit/v2 v2.4.0
	github.com/evanphx/json-patch/v5 v5.6.0
	github.com/glaslos/tlsh v0.2.0
	github.com/go-errors/errors v1.4.2
	github.com/go-json-experiment/json v0.0.0-20250910080747-cc2cfa0554c3
	github.com/golang-jwt/jwt/v4 v4.5.2
	github.com/golang/protobuf v1.5.4
	github.com/gorilla/websocket v1.5.2-0.20240215025916-695e9095ce87
	github.com/hanwen/go-fuse/v2 v2.5.1
	github.com/hashicorp/go-retryablehttp v0.7.7
	github.com/hillu/go-archive-zip-crypto v0.0.0-20200712202847-bd5cf365dd44
	github.com/hirochachacha/go-smb2 v1.1.0
	github.com/inconshreveable/mousetrap v1.1.0
	github.com/jackwakefield/gopac v1.0.2
	github.com/kaptinlin/jsonschema v0.5.2
	github.com/leodido/go-syslog v1.0.1
	github.com/lpar/gzipped v1.1.0
	github.com/mccutchen/go-httpbin/v2 v2.18.3
	github.com/mitchellh/go-wordwrap v1.0.1
	github.com/mooijtech/go-pst/v6 v6.0.2
	github.com/pkg/errors v0.9.1
	github.com/rogpeppe/go-internal v1.14.1
	github.com/shirou/gopsutil/v4 v4.25.1
	github.com/syndtr/goleveldb v1.0.0
	github.com/valyala/fastjson v1.6.4
	github.com/vincent-petithory/dataurl v1.0.0
	github.com/virtuald/go-paniclog v0.0.0-20190812204905-43a7fa316459
	golang.org/x/oauth2 v0.34.0
	google.golang.org/genproto/googleapis/api v0.0.0-20251222181119-0a764e51fe1b
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.2.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	software.sslmate.com/src/go-pkcs12 v0.2.0
	www.velocidex.com/golang/vtypes v0.0.0-20250802153006-821cec8fd392
)

require (
	cel.dev/expr v0.25.1 // indirect
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.18.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/iam v1.5.3 // indirect
	cloud.google.com/go/monitoring v1.24.3 // indirect
	cloud.google.com/go/pubsub/v2 v2.3.0 // indirect
	github.com/360EntSecGroup-Skylar/excelize v1.4.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.17.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.10.0 // indirect
	github.com/BurntSushi/toml v1.5.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.30.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.54.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.54.0 // indirect
	github.com/PuerkitoBio/goquery v1.8.1 // indirect
	github.com/RoaringBitmap/roaring v1.9.4 // indirect
	github.com/alecthomas/colour v0.1.0 // indirect
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b // indirect
	github.com/andybalholm/cascadia v1.3.2 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.1 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.15.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.11.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.3.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.11.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.17.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.20.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.23.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.28.3 // indirect
	github.com/aws/smithy-go v1.20.1 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beevik/etree v1.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.24.4 // indirect
	github.com/blevesearch/go-porterstemmer v1.0.3 // indirect
	github.com/blevesearch/mmap-go v1.0.4 // indirect
	github.com/blevesearch/segment v0.9.1 // indirect
	github.com/blevesearch/snowballstem v0.9.0 // indirect
	github.com/blevesearch/zap/v11 v11.0.14 // indirect
	github.com/blevesearch/zap/v12 v12.0.14 // indirect
	github.com/blevesearch/zap/v13 v13.0.6 // indirect
	github.com/blevesearch/zap/v14 v14.0.5 // indirect
	github.com/blevesearch/zap/v15 v15.0.3 // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/catppuccin/go v0.2.0 // indirect
	github.com/cavaliergopher/cpio v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/bubbles v0.20.0 // indirect
	github.com/charmbracelet/bubbletea v1.2.4 // indirect
	github.com/charmbracelet/x/ansi v0.5.2 // indirect
	github.com/charmbracelet/x/exp/strings v0.0.0-20241209212528-0eec74ecaa6f // indirect
	github.com/charmbracelet/x/term v0.2.1 // indirect
	github.com/cilium/ebpf v0.20.1-0.20251215101449-df5c3096bd8c // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.3.0 // indirect
	github.com/cncf/xds/go v0.0.0-20251210132809-ee656c7534f5 // indirect
	github.com/couchbase/vellum v1.0.2 // indirect
	github.com/crewjam/httperr v0.2.0 // indirect
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	github.com/dlclark/regexp2 v1.7.0 // indirect
	github.com/dustmop/soup v1.1.2-0.20190516214245-38228baa104e // indirect
	github.com/ebitengine/purego v0.8.3 // indirect
	github.com/emersion/go-message v0.16.0 // indirect
	github.com/emersion/go-textwrapper v0.0.0-20200911093747-65d896831594 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.36.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.3.0 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/geoffgarside/ber v1.1.0 // indirect
	github.com/gizak/termui/v3 v3.1.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/godzie44/go-uring v0.0.0-20220926161041-69611e8b13d5 // indirect
	github.com/golang/gddo v0.0.0-20210115222349-20d68f94ee1f // indirect
	github.com/golang/glog v1.2.5 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.7 // indirect
	github.com/googleapis/gax-go/v2 v2.16.0 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hillu/go-yara/v4 v4.3.2 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kaptinlin/go-i18n v0.2.0 // indirect
	github.com/kaptinlin/messageformat-go v0.4.6 // indirect
	github.com/karrick/godirwalk v1.17.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/labstack/echo v3.3.10+incompatible // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/lestrrat-go/strftime v1.0.5 // indirect
	github.com/libp2p/go-sockaddr v0.1.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattermost/xml-roundtrip-validator v0.1.0 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/mschoch/smat v0.2.0 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/termenv v0.15.3-0.20240618155329-98d742f6907a // indirect
	github.com/nsf/termbox-go v1.1.1 // indirect
	github.com/paulmach/orb v0.10.0 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rotisserie/eris v0.5.4 // indirect
	github.com/russellhaering/goxmldsig v1.3.0 // indirect
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spiffe/go-spiffe/v2 v2.6.0 // indirect
	github.com/steveyen/gtreap v0.1.0 // indirect
	github.com/tidwall/btree v1.6.0 // indirect
	github.com/tinylib/msgp v1.6.3 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/willf/bitset v1.1.11 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.etcd.io/bbolt v1.4.3 // indirect
	go.mongodb.org/mongo-driver v1.12.1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.39.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.64.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.64.0 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/sdk v1.39.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/arch v0.23.0 // indirect
	golang.org/x/exp v0.0.0-20251219203646-944ab1f22d93 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/term v0.38.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251222181119-0a764e51fe1b // indirect
	kernel.org/pub/linux/libs/security/libcap/cap v1.2.71 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.71 // indirect
	www.velocidex.com/golang/binparsergen v0.1.1-0.20240404114946-8f66c7cf586e // indirect
)

// replace github.com/Velocidex/yara-x-go => ../yara-x-go
// replace github.com/Velocidex/grok => ../grok
// replace www.velocidex.com/golang/vfilter => ../vfilter
// replace www.velocidex.com/golang/regparser => ../regparser
// replace www.velocidex.com/golang/go-ntfs => ../go-ntfs
// replace github.com/Velocidex/go-fat => ../go-fat
// replace github.com/Velocidex/go-vmdk => ../go-vmdk
// replace www.velocidex.com/golang/go-pe => ../go-pe
// replace www.velocidex.com/golang/evtx => ../evtx
// replace www.velocidex.com/golang/go-ese => ../go-ese
// replace github.com/Velocidex/ordereddict => ../ordereddict
// replace github.com/Velocidex/yaml/v2 => ../yaml
// replace www.velocidex.com/golang/go-prefetch => ../go-prefetch
// replace github.com/Velocidex/go-magic => ../go-magic
// replace github.com/Velocidex/go-yara => ../go-yara-velocidex
// replace github.com/Velocidex/json => ../json
// replace github.com/russross/blackfriday/v2 => ../blackfriday
// replace www.velocidex.com/golang/vtypes => ../vtypes
// replace github.com/Velocidex/ttlcache/v2 => ../ttlcache
// replace github.com/Velocidex/zip => ../zip
// replace github.com/Velocidex/sflags => ../sflags
// replace github.com/Velocidex/etw => ../etw
// replace github.com/Velocidex/go-ewf => ../go-ewf
// replace github.com/Velocidex/grpc-go-pool => ../grpc-go-pool
// replace www.velocidex.com/golang/oleparse => ../oleparse
// replace github.com/go-errors/errors => ../errors
// replace github.com/Velocidex/ttlcache/v2 => ../ttlcache
// replace github.com/Velocidex/go-vhdx => ../go-vhdx
// replace github.com/Velocidex/go-mscfb => ../go-mscfb
// replace github.com/Velocidex/WinPmem/go-winpmem => ../WinPmem/go-winpmem
// replace github.com/Velocidex/sigma-go => ../sigma-go
// replace github.com/Velocidex/tracee_velociraptor => ../tracee_velociraptor
// replace github.com/Velocidex/disklru => ../disklru

// replace github.com/Velocidex/fileb0x => ../fileb0x
// replace github.com/Velocidex/go-ext4 => ../go-ext4
// replace github.com/Velocidex/amsi => ../amsi
// replace github.com/Velocidex/go-journalctl ../go-journalctl
// replace github.com/Velocidex/velociraptor-site-search => ../velociraptor-docs/velociraptor-site-search

// Remove search for html end block. This allows inserting unbalanced
// HTML tags into the markdown
replace github.com/russross/blackfriday/v2 => github.com/Velocidex/blackfriday/v2 v2.0.2-0.20200811050547-4f26a09e2b3b

go 1.25.3

// Needed for syntax highlighting VQL. Removes extra fat.
replace github.com/alecthomas/chroma => github.com/Velocidex/chroma v0.6.8-0.20200418131129-82edc291369c

replace github.com/go-errors/errors => github.com/Velocidex/errors v0.0.0-20221019164655-9ace6bf61e26

//replace github.com/bradleyjkemp/sigma-go => ../sigma-go
