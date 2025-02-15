module www.velocidex.com/golang/velociraptor

require (
	cloud.google.com/go/pubsub v1.33.0
	cloud.google.com/go/storage v1.33.0
	github.com/Depado/bfchroma v1.3.0
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Showmax/go-fqdn v1.0.0
	github.com/Velocidex/amsi v0.0.0-20200608120838-e5d93b76f119
	github.com/Velocidex/etw v0.0.0-20231115144702-0b885b292f0f
	github.com/Velocidex/go-elasticsearch/v7 v7.3.1-0.20191001125819-fee0ef9cac6b
	github.com/Velocidex/go-magic v0.0.0-20211018155418-c5dc48282f28
	github.com/Velocidex/go-yara v1.1.10-0.20240309155455-3f491847cec9
	github.com/Velocidex/grpc-go-pool v1.2.2-0.20241016164850-ff0cb80037a8
	github.com/Velocidex/json v0.0.0-20220224052537-92f3c0326e5a
	github.com/Velocidex/pkcs7 v0.0.0-20230220112103-d4ed02e1862a
	github.com/Velocidex/sflags v0.3.1-0.20231011011525-620ab7ca8617
	github.com/Velocidex/ttlcache/v2 v2.9.1-0.20240517145123-a3f45e86e130
	github.com/Velocidex/yaml/v2 v2.2.8
	github.com/Velocidex/zip v0.0.0-20210101070220-e7ecefb7aad7
	github.com/alecthomas/assert v1.0.0
	github.com/alecthomas/chroma v0.7.3
	github.com/alecthomas/participle v0.7.1
	github.com/alecthomas/repr v0.4.0 // indirect
	github.com/alexmullins/zip v0.0.0-20180717182244-4affb64b04d0
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de
	github.com/clbanning/mxj v1.8.4
	github.com/crewjam/saml v0.4.14
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/dimchansky/utfbom v1.1.1
	github.com/dustin/go-humanize v1.0.0
	github.com/elastic/go-elasticsearch/v7 v7.3.0 // indirect
	github.com/go-ole/go-ole v1.2.6
	github.com/go-sql-driver/mysql v1.7.1
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.6.0
	github.com/google/btree v1.1.2
	github.com/google/rpmpack v0.5.0
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/google/uuid v1.6.0
	github.com/gorilla/csrf v1.6.2
	github.com/gorilla/schema v1.4.1
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.18.0
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hillu/go-ntdll v0.0.0-20220801201350-0d23f057ef1f
	github.com/hinshun/vt10x v0.0.0-20220301184237-5011da428d02 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/jonboulle/clockwork v0.3.0 // indirect
	github.com/juju/ratelimit v1.0.1
	github.com/lib/pq v1.10.9
	github.com/magefile/mage v1.15.0
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20
	github.com/mattn/go-pointer v0.0.0-20180825124634-49522c3f3791
	github.com/mattn/go-sqlite3 v1.14.22
	github.com/microcosm-cc/bluemonday v1.0.23
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/panicwrap v1.0.0
	github.com/olekukonko/tablewriter v0.0.5
	github.com/oschwald/maxminddb-golang v1.8.0
	github.com/pkg/sftp v1.13.6
	github.com/prometheus/client_golang v1.15.1
	github.com/prometheus/client_model v0.4.0
	github.com/qri-io/starlib v0.5.0
	github.com/rifflock/lfshook v0.0.0-20180920164130-b9218ef580f5
	github.com/robertkrimen/otto v0.3.0
	github.com/russross/blackfriday/v2 v2.1.0
	github.com/sebdah/goldie/v2 v2.5.3
	github.com/sergi/go-diff v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.9.0
	github.com/xor-gate/ar v0.0.0-20170530204233-5c72ae81e2b7 // indirect
	github.com/xor-gate/debpkg v1.0.0
	go.starlark.net v0.0.0-20230925163745-10651d5192ab
	golang.org/x/crypto v0.28.0
	golang.org/x/mod v0.17.0
	golang.org/x/net v0.30.0
	golang.org/x/sys v0.26.0
	golang.org/x/text v0.21.0
	golang.org/x/time v0.3.0
	google.golang.org/api v0.146.0
	google.golang.org/genproto v0.0.0-20231009173412-8bfb1ae86b6c // indirect
	google.golang.org/grpc v1.67.1
	google.golang.org/protobuf v1.35.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
	gopkg.in/sourcemap.v1 v1.0.5 // indirect
	howett.net/plist v1.0.0
	www.velocidex.com/golang/evtx v0.2.1-0.20240730174545-3e4ff3d96433
	www.velocidex.com/golang/go-ese v0.2.1-0.20240919031214-2aa005106db2
	www.velocidex.com/golang/go-ntfs v0.2.1-0.20250215044736-81b32bb0b4d2
	www.velocidex.com/golang/go-pe v0.1.1-0.20250101153735-7a925ba8334b
	www.velocidex.com/golang/go-prefetch v0.0.0-20240910051453-2385582c1c22
	www.velocidex.com/golang/oleparse v0.0.0-20230217092320-383a0121aafe
	www.velocidex.com/golang/regparser v0.0.0-20240404115756-2169ac0e3c09
	www.velocidex.com/golang/vfilter v0.0.0-20241123123542-6b030f4d2090
)

require (
	github.com/AlecAivazis/survey/v2 v2.3.6
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.0.0
	github.com/Masterminds/semver/v3 v3.2.1
	github.com/Masterminds/sprig/v3 v3.2.2
	github.com/Velocidex/WinPmem/go-winpmem v0.0.0-20240711041142-80f6ecbbeb7f
	github.com/Velocidex/file-rotatelogs v0.0.0-20211221020724-d12e4dae4e11
	github.com/Velocidex/go-ewf v0.0.0-20240210123447-97dc81b7d8c3
	github.com/Velocidex/go-ext4 v0.0.0-20240608083317-8dd00855b069
	github.com/Velocidex/go-fat v0.0.0-20230923165230-3e6c4265297a
	github.com/Velocidex/go-journalctl v0.0.0-20241004063153-cc1c858415bd
	github.com/Velocidex/go-mscfb v0.0.0-20240618091452-31f4ccc54002
	github.com/Velocidex/go-vhdx v0.0.0-20240601014259-b204818c95fd
	github.com/Velocidex/go-vmdk v0.0.0-20240909080044-e373986b6517
	github.com/Velocidex/grok v0.0.1
	github.com/Velocidex/ordereddict v0.0.0-20230909174157-2aa49cc5d11d
	github.com/Velocidex/sigma-go v0.0.0-20241025122940-1b771d3d57a9
	github.com/VirusTotal/gyp v0.9.0
	github.com/alecthomas/kingpin/v2 v2.4.0
	github.com/alitto/pond v1.8.3
	github.com/andybalholm/brotli v1.0.4
	github.com/aws/aws-sdk-go-v2 v1.25.2
	github.com/aws/aws-sdk-go-v2/config v1.27.6
	github.com/aws/aws-sdk-go-v2/credentials v1.17.6
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.16.8
	github.com/aws/aws-sdk-go-v2/service/s3 v1.51.3
	github.com/clayscode/Go-Splunk-HTTP/splunk/v2 v2.0.1-0.20221027171526-76a36be4fa02
	github.com/coreos/go-oidc/v3 v3.11.0
	github.com/elastic/go-libaudit/v2 v2.4.0
	github.com/evanphx/json-patch/v5 v5.6.0
	github.com/glaslos/tlsh v0.2.0
	github.com/go-errors/errors v1.4.2
	github.com/golang-jwt/jwt/v4 v4.4.3
	github.com/golang/protobuf v1.5.4
	github.com/gorilla/websocket v1.5.2-0.20240215025916-695e9095ce87
	github.com/hanwen/go-fuse/v2 v2.5.1
	github.com/hashicorp/go-retryablehttp v0.7.7
	github.com/hillu/go-archive-zip-crypto v0.0.0-20200712202847-bd5cf365dd44
	github.com/hirochachacha/go-smb2 v1.1.0
	github.com/inconshreveable/mousetrap v1.1.0
	github.com/jackwakefield/gopac v1.0.2
	github.com/lpar/gzipped v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/rogpeppe/go-internal v1.12.0
	github.com/shirou/gopsutil/v3 v3.21.11
	github.com/syndtr/goleveldb v1.0.0
	github.com/valyala/fastjson v1.6.4
	github.com/vincent-petithory/dataurl v1.0.0
	github.com/virtuald/go-paniclog v0.0.0-20190812204905-43a7fa316459
	golang.org/x/oauth2 v0.22.0
	google.golang.org/genproto/googleapis/api v0.0.0-20240814211410-ddb44dafa142
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.2.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	software.sslmate.com/src/go-pkcs12 v0.2.0
	www.velocidex.com/golang/vtypes v0.0.0-20240123105603-069d4a7f435c
)

require (
	cloud.google.com/go v0.110.8 // indirect
	cloud.google.com/go/compute/metadata v0.5.0 // indirect
	cloud.google.com/go/iam v1.1.2 // indirect
	github.com/360EntSecGroup-Skylar/excelize v1.4.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.1.1 // indirect
	github.com/PuerkitoBio/goquery v1.8.1 // indirect
	github.com/alecthomas/colour v0.1.0 // indirect
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20240626203959-61d1e3462e30 // indirect
	github.com/andybalholm/cascadia v1.3.2 // indirect
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
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beevik/etree v1.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cavaliergopher/cpio v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/crewjam/httperr v0.2.0 // indirect
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	github.com/dlclark/regexp2 v1.7.0 // indirect
	github.com/dustmop/soup v1.1.2-0.20190516214245-38228baa104e // indirect
	github.com/geoffgarside/ber v1.1.0 // indirect
	github.com/go-jose/go-jose/v4 v4.0.4 // indirect
	github.com/golang/gddo v0.0.0-20210115222349-20d68f94ee1f // indirect
	github.com/golang/glog v1.2.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.1 // indirect
	github.com/googleapis/gax-go/v2 v2.12.0 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/gorilla/securecookie v1.1.1 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hillu/go-yara/v4 v4.3.2 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/lestrrat-go/strftime v1.0.5 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattermost/xml-roundtrip-validator v0.1.0 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/paulmach/orb v0.10.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/rivo/uniseg v0.4.2 // indirect
	github.com/russellhaering/goxmldsig v1.3.0 // indirect
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/spf13/cast v1.3.1 // indirect
	github.com/tklauser/go-sysconf v0.3.9 // indirect
	github.com/tklauser/numcpus v0.3.0 // indirect
	github.com/ulikunitz/xz v0.5.11 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.mongodb.org/mongo-driver v1.12.1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.uber.org/goleak v1.2.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/term v0.25.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241015192408-796eee8c2d53 // indirect
	www.velocidex.com/golang/binparsergen v0.1.1-0.20240404114946-8f66c7cf586e // indirect
)

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

// Remove search for html end block. This allows inserting unbalanced
// HTML tags into the markdown
replace github.com/russross/blackfriday/v2 => github.com/Velocidex/blackfriday/v2 v2.0.2-0.20200811050547-4f26a09e2b3b

go 1.23

toolchain go1.23.1

// Needed for syntax highlighting VQL. Removes extra fat.
replace github.com/alecthomas/chroma => github.com/Velocidex/chroma v0.6.8-0.20200418131129-82edc291369c

replace github.com/go-errors/errors => github.com/Velocidex/errors v0.0.0-20221019164655-9ace6bf61e26

//replace github.com/bradleyjkemp/sigma-go => ../sigma-go
