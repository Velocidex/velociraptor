module www.velocidex.com/golang/velociraptor

require (
	cloud.google.com/go/pubsub v1.25.1
	cloud.google.com/go/storage v1.25.0
	github.com/Depado/bfchroma v1.3.0
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Showmax/go-fqdn v1.0.0
	github.com/Velocidex/ahocorasick v0.0.0-20180712114356-e1c353eeaaee
	github.com/Velocidex/amsi v0.0.0-20200608120838-e5d93b76f119
	github.com/Velocidex/etw v0.0.0-20210723072214-4d0cffd1ff22
	github.com/Velocidex/go-elasticsearch/v7 v7.3.1-0.20191001125819-fee0ef9cac6b
	github.com/Velocidex/go-magic v0.0.0-20211018155418-c5dc48282f28
	github.com/Velocidex/go-yara v1.1.10-0.20221202090138-c7dde4c43aa4
	github.com/Velocidex/grpc-go-pool v1.2.2-0.20211129003310-ece3b3fe13f4
	github.com/Velocidex/json v0.0.0-20220224052537-92f3c0326e5a
	github.com/Velocidex/pkcs7 v0.0.0-20210524015001-8d1eee94a157
	github.com/Velocidex/sflags v0.3.1-0.20210402155316-b09f53df5162
	github.com/Velocidex/ttlcache/v2 v2.9.1-0.20211116035050-ddd93fed62f5
	github.com/Velocidex/yaml/v2 v2.2.8
	github.com/Velocidex/zip v0.0.0-20210101070220-e7ecefb7aad7
	github.com/alecthomas/assert v1.0.0
	github.com/alecthomas/chroma v0.7.3
	github.com/alecthomas/participle v0.7.1
	github.com/alecthomas/repr v0.1.1 // indirect
	github.com/alexmullins/zip v0.0.0-20180717182244-4affb64b04d0
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de
	github.com/aws/aws-sdk-go v1.44.118
	github.com/clbanning/mxj v1.8.4
	github.com/crewjam/saml v0.4.8
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/dimchansky/utfbom v1.1.1
	github.com/dustin/go-humanize v1.0.0
	github.com/elastic/go-elasticsearch/v7 v7.3.0 // indirect
	github.com/elastic/go-libaudit v0.4.0
	github.com/go-ole/go-ole v1.2.6
	github.com/go-sql-driver/mysql v1.5.0
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.6.0
	github.com/google/btree v1.0.1
	github.com/google/rpmpack v0.0.0-20220411070212-51a1004ef6cb
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/google/uuid v1.3.0
	github.com/gorilla/csrf v1.6.2
	github.com/gorilla/schema v1.1.0
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.10.3
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hillu/go-ntdll v0.0.0-20220801201350-0d23f057ef1f
	github.com/hinshun/vt10x v0.0.0-20220301184237-5011da428d02 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/jmoiron/sqlx v1.3.4
	github.com/jonboulle/clockwork v0.3.0 // indirect
	github.com/juju/ratelimit v1.0.1
	github.com/lib/pq v1.2.0
	github.com/magefile/mage v1.11.0
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.16
	github.com/mattn/go-pointer v0.0.0-20180825124634-49522c3f3791
	github.com/mattn/go-sqlite3 v1.14.10
	github.com/microcosm-cc/bluemonday v1.0.16
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/panicwrap v1.0.0
	github.com/olekukonko/tablewriter v0.0.5
	github.com/oschwald/maxminddb-golang v1.8.0
	github.com/pkg/sftp v1.13.1
	github.com/prometheus/client_golang v1.12.1
	github.com/prometheus/client_model v0.2.0
	github.com/qri-io/starlib v0.5.0
	github.com/rifflock/lfshook v0.0.0-20180920164130-b9218ef580f5
	github.com/robertkrimen/otto v0.0.0-20210614181706-373ff5438452
	github.com/russross/blackfriday/v2 v2.0.1
	github.com/sebdah/goldie v1.0.0
	github.com/sebdah/goldie/v2 v2.5.3
	github.com/sergi/go-diff v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.8.1
	github.com/tink-ab/tempfile v0.0.0-20180226111222-33beb0518f1a
	github.com/vjeantet/grok v1.0.0
	github.com/xor-gate/ar v0.0.0-20170530204233-5c72ae81e2b7 // indirect
	github.com/xor-gate/debpkg v1.0.0
	go.starlark.net v0.0.0-20221010140840-6bf6f0955179
	golang.org/x/crypto v0.0.0-20221012134737-56aed061732a
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4
	golang.org/x/net v0.0.0-20221019024206-cb67ada4b0ad
	golang.org/x/sys v0.1.0
	golang.org/x/text v0.4.0 // indirect
	golang.org/x/time v0.0.0-20220609170525-579cf78fd858
	google.golang.org/api v0.96.0
	google.golang.org/genproto v0.0.0-20221018160656-63c7b68cfc55
	google.golang.org/grpc v1.50.1
	google.golang.org/protobuf v1.28.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
	gopkg.in/sourcemap.v1 v1.0.5 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
	howett.net/plist v1.0.0
	www.velocidex.com/golang/evtx v0.2.1-0.20220404133451-1fdf8be7325e
	www.velocidex.com/golang/go-ese v0.1.1-0.20220107095505-c38622559671
	www.velocidex.com/golang/go-ntfs v0.1.2-0.20221117122413-b97c856cb140
	www.velocidex.com/golang/go-pe v0.1.1-0.20220506020923-9fac492a9b0d
	www.velocidex.com/golang/go-prefetch v0.0.0-20220801101854-338dbe61982a
	www.velocidex.com/golang/oleparse v0.0.0-20220617011920-94df2342d0b7
	www.velocidex.com/golang/regparser v0.0.0-20221020153526-bbc758cbd18b
	www.velocidex.com/golang/vfilter v0.0.0-20221124045546-c666c341aec3
)

require (
	github.com/AlecAivazis/survey/v2 v2.3.6
	github.com/Masterminds/sprig/v3 v3.2.2
	github.com/Velocidex/file-rotatelogs v0.0.0-20211221020724-d12e4dae4e11
	github.com/Velocidex/ordereddict v0.0.0-20221110130714-6a7cb85851cd
	github.com/andybalholm/brotli v1.0.4
	github.com/clayscode/Go-Splunk-HTTP/splunk/v2 v2.0.1-0.20221027171526-76a36be4fa02
	github.com/coreos/go-oidc/v3 v3.4.0
	github.com/evanphx/json-patch/v5 v5.6.0
	github.com/glaslos/tlsh v0.2.0
	github.com/go-errors/errors v1.4.2
	github.com/golang-jwt/jwt/v4 v4.4.2
	github.com/hillu/go-archive-zip-crypto v0.0.0-20200712202847-bd5cf365dd44
	github.com/lpar/gzipped v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/rogpeppe/go-internal v1.9.0
	github.com/shirou/gopsutil/v3 v3.21.11
	golang.org/x/oauth2 v0.0.0-20221014153046-6fdb5e3db783
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.2.0
	www.velocidex.com/golang/vtypes v0.0.0-20220816192452-6a27ae078f12
)

require (
	cloud.google.com/go v0.104.0 // indirect
	cloud.google.com/go/compute v1.10.0 // indirect
	cloud.google.com/go/iam v0.4.0 // indirect
	github.com/360EntSecGroup-Skylar/excelize v1.4.1 // indirect
	github.com/Masterminds/semver/v3 v3.1.1 // indirect
	github.com/PuerkitoBio/goquery v1.8.0 // indirect
	github.com/alecthomas/colour v0.1.0 // indirect
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20211218093645-b94a6e3cc137 // indirect
	github.com/andybalholm/cascadia v1.3.1 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beevik/etree v1.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cavaliergopher/cpio v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/crewjam/httperr v0.2.0 // indirect
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	github.com/dlclark/regexp2 v1.7.0 // indirect
	github.com/dustmop/soup v1.1.2-0.20190516214245-38228baa104e // indirect
	github.com/golang/gddo v0.0.0-20210115222349-20d68f94ee1f // indirect
	github.com/golang/glog v1.0.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/martian/v3 v3.3.2 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.0 // indirect
	github.com/googleapis/gax-go/v2 v2.5.1 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/gorilla/securecookie v1.1.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/klauspost/compress v1.15.11 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/lestrrat-go/strftime v1.0.5 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattermost/xml-roundtrip-validator v0.1.0 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/paulmach/orb v0.1.5 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rivo/uniseg v0.4.2 // indirect
	github.com/russellhaering/goxmldsig v1.2.0 // indirect
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/spf13/cast v1.3.1 // indirect
	github.com/tklauser/go-sysconf v0.3.9 // indirect
	github.com/tklauser/numcpus v0.3.0 // indirect
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/valyala/fastjson v1.6.3 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.opencensus.io v0.23.0 // indirect
	go.uber.org/goleak v1.2.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/term v0.0.0-20221017184919-83659145692c // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	www.velocidex.com/golang/binparsergen v0.1.1-0.20220107080050-ae6122c5ed14 // indirect
)

// replace www.velocidex.com/golang/vfilter => /home/mic/projects/vfilter
// replace www.velocidex.com/golang/regparser => /home/mic/projects/regparser
// replace www.velocidex.com/golang/go-ntfs => /home/mic/projects/go-ntfs
// replace www.velocidex.com/golang/go-pe => /home/mic/projects/go-pe
// replace www.velocidex.com/golang/evtx => /home/mic/projects/evtx
// replace www.velocidex.com/golang/go-ese => /home/mic/projects/go-ese
// replace github.com/Velocidex/ordereddict => /home/mic/projects/ordereddict
// replace github.com/Velocidex/yaml/v2 => /home/mic/projects/yaml
// replace www.velocidex.com/golang/go-prefetch => /home/mic/projects/go-prefetch
// replace github.com/Velocidex/go-magic => /home/mic/projects/go-magic
// replace github.com/Velocidex/go-yara => /home/mic/projects/go-yara-velocidex
// replace github.com/Velocidex/json => /home/mic/projects/json
// replace github.com/russross/blackfriday/v2 => /home/mic/projects/blackfriday
// replace www.velocidex.com/golang/vtypes => /home/mic/projects/vtypes
// replace github.com/Velocidex/ttlcache/v2 => /home/mic/projects/ttlcache
// replace github.com/Velocidex/zip => /home/mic/projects/zip
// replace github.com/Velocidex/sflags => /home/mic/projects/sflags
// replace github.com/Velocidex/etw => /home/mic/projects/etw
// replace github.com/Velocidex/grpc-go-pool => /home/mic/projects/grpc-go-pool
// replace www.velocidex.com/golang/oleparse => /home/matt/git/oleparse
// replace github.com/go-errors/errors => /home/mic/projects/errors
// replace github.com/Velocidex/ttlcache/v2 => /home/mic/projects/ttlcache

// Remove search for html end block. This allows inserting unbalanced
// HTML tags into the markdown
replace github.com/russross/blackfriday/v2 => github.com/Velocidex/blackfriday/v2 v2.0.2-0.20200811050547-4f26a09e2b3b

go 1.18

// Needed for syntax highlighting VQL. Removes extra fat.
replace github.com/alecthomas/chroma => github.com/Velocidex/chroma v0.6.8-0.20200418131129-82edc291369c

// Fix broken version issue.
replace github.com/crewjam/saml v0.4.8 => github.com/Velocidex/saml v0.0.0-20221019055034-272f55e26c8d

replace github.com/go-errors/errors => github.com/Velocidex/errors v0.0.0-20221019164655-9ace6bf61e26
