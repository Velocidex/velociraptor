module www.velocidex.com/golang/velociraptor

require (
	cloud.google.com/go v0.88.0 // indirect
	cloud.google.com/go/pubsub v1.13.0
	cloud.google.com/go/storage v1.16.0
	github.com/Depado/bfchroma v1.2.0
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/Netflix/go-expect v0.0.0-20210722184520-ef0bf57d82b3 // indirect
	github.com/Showmax/go-fqdn v1.0.0
	github.com/StackExchange/wmi v0.0.0-20180116203802-5d049714c4a6 // indirect
	github.com/Velocidex/ahocorasick v0.0.0-20180712114356-e1c353eeaaee
	github.com/Velocidex/amsi v0.0.0-20200608120838-e5d93b76f119
	github.com/Velocidex/cgofuse v1.1.2
	github.com/Velocidex/etw v0.0.0-20210723072214-4d0cffd1ff22
	github.com/Velocidex/go-elasticsearch/v7 v7.3.1-0.20191001125819-fee0ef9cac6b
	github.com/Velocidex/go-yara v1.1.10-0.20210726130504-d5e402efc424
	github.com/Velocidex/json v0.0.0-20210402154432-68206e1293d0
	github.com/Velocidex/ordereddict v0.0.0-20210502082334-cf5d9045c0d1
	github.com/Velocidex/pkcs7 v0.0.0-20210524015001-8d1eee94a157
	github.com/Velocidex/sflags v0.3.1-0.20210402155316-b09f53df5162
	github.com/Velocidex/survey v1.8.7-0.20190926071832-2ff99cc7aa49
	github.com/Velocidex/yaml/v2 v2.2.5
	github.com/Velocidex/zip v0.0.0-20210101070220-e7ecefb7aad7
	github.com/ZachtimusPrime/Go-Splunk-HTTP/splunk/v2 v2.0.1
	github.com/alecthomas/assert v0.0.0-20170929043011-405dbfeb8e38
	github.com/alecthomas/chroma v0.7.2
	github.com/alecthomas/participle v0.7.1
	github.com/alecthomas/repr v0.0.0-20210611225437-1a2716eca9d6 // indirect
	github.com/alexmullins/zip v0.0.0-20180717182244-4affb64b04d0
	github.com/aws/aws-sdk-go v1.40.6
	github.com/clbanning/mxj v1.8.4
	github.com/coreos/go-oidc v2.2.1+incompatible
	github.com/creack/pty v1.1.13 // indirect
	github.com/crewjam/saml v0.4.5
	github.com/davecgh/go-spew v1.1.1
	github.com/dimchansky/utfbom v1.1.1
	github.com/dustin/go-humanize v1.0.0
	github.com/elastic/go-elasticsearch/v7 v7.3.0 // indirect
	github.com/elastic/go-libaudit v0.4.0
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/fastly/go-utils v0.0.0-20180712184237-d95a45783239 // indirect
	github.com/go-ole/go-ole v1.2.4
	github.com/go-sql-driver/mysql v1.5.0
	github.com/golang-jwt/jwt v3.2.1+incompatible
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.3
	github.com/google/rpmpack v0.0.0-20200615183209-0c831d19bd44
	github.com/google/uuid v1.3.0
	github.com/gorilla/csrf v1.6.2
	github.com/gorilla/schema v1.1.0
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.5.0
	github.com/hanwen/go-fuse v1.0.1-0.20190726130028-2f298055551b
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hillu/go-ntdll v0.0.0-20210404124636-a6f426aa8d92
	github.com/hinshun/vt10x v0.0.0-20180809195222-d55458df857c // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/jehiah/go-strftime v0.0.0-20171201141054-1d33003b3869 // indirect
	github.com/jmoiron/sqlx v1.2.1-0.20190826204134-d7d95172beb5
	github.com/jonboulle/clockwork v0.2.2 // indirect
	github.com/juju/ratelimit v1.0.1
	github.com/kierdavis/dateparser v0.0.0-20171227112021-81e70b820720
	github.com/kr/pty v1.1.8 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lestrrat-go/file-rotatelogs v2.2.0+incompatible
	github.com/lestrrat-go/strftime v0.0.0-20190725011945-5c849dd2c51d // indirect
	github.com/lib/pq v1.2.0
	github.com/magefile/mage v1.11.0
	github.com/mattn/go-colorable v0.1.7 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/mattn/go-pointer v0.0.0-20180825124634-49522c3f3791
	github.com/mattn/go-runewidth v0.0.9 // indirect
	github.com/mattn/go-sqlite3 v1.14.6
	github.com/microcosm-cc/bluemonday v1.0.15
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/olekukonko/tablewriter v0.0.4
	github.com/pkg/errors v0.9.1
	github.com/pkg/sftp v1.12.0
	github.com/pquerna/cachecontrol v0.0.0-20200921180117-858c6e7e6b7e // indirect
	github.com/processout/grpc-go-pool v1.2.1
	github.com/prometheus/client_golang v1.2.1
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/qri-io/starlib v0.5.0
	github.com/rifflock/lfshook v0.0.0-20180920164130-b9218ef580f5
	github.com/robertkrimen/otto v0.0.0-20210614181706-373ff5438452
	github.com/russross/blackfriday/v2 v2.0.1
	github.com/sebdah/goldie v1.0.0
	github.com/sebdah/goldie/v2 v2.5.3
	github.com/sergi/go-diff v1.1.0
	github.com/shirou/gopsutil v3.21.6+incompatible
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/tebeka/strftime v0.1.3 // indirect
	github.com/tink-ab/tempfile v0.0.0-20180226111222-33beb0518f1a
	github.com/tklauser/go-sysconf v0.3.7 // indirect
	github.com/vjeantet/grok v1.0.0
	github.com/xor-gate/ar v0.0.0-20170530204233-5c72ae81e2b7 // indirect
	github.com/xor-gate/debpkg v0.0.0-20181217150151-a0c70a3d4213
	go.starlark.net v0.0.0-20210602144842-1cdb82c9e17a
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97
	golang.org/x/mod v0.4.2
	golang.org/x/net v0.0.0-20210716203947-853a461950ff
	golang.org/x/oauth2 v0.0.0-20210628180205-a41e5a781914
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c
	golang.org/x/time v0.0.0-20210611083556-38a9dc6acbc6
	google.golang.org/api v0.51.0
	google.golang.org/genproto v0.0.0-20210722135532-667f2b7c528f
	google.golang.org/grpc v1.39.0
	google.golang.org/protobuf v1.27.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
	gopkg.in/sourcemap.v1 v1.0.5 // indirect
	gopkg.in/square/go-jose.v2 v2.5.1 // indirect
	howett.net/plist v0.0.0-20201203080718-1454fab16a06
	www.velocidex.com/golang/evtx v0.0.2-0.20201104041743-4b6cdb206c95
	www.velocidex.com/golang/go-ese v0.0.0-20200111070159-4b7484475321
	www.velocidex.com/golang/go-ntfs v0.1.2-0.20201111050421-bbba6f6a13d3
	www.velocidex.com/golang/go-pe v0.1.1-0.20210524015317-07c8b305094e
	www.velocidex.com/golang/go-prefetch v0.0.0-20200722101157-37e4751dd5ca
	www.velocidex.com/golang/oleparse v0.0.0-20190327031422-34195d413196
	www.velocidex.com/golang/regparser v0.0.0-20190625082115-b02dc43c2500
	www.velocidex.com/golang/vfilter v0.0.0-20210727160300-eadf120417ae
	www.velocidex.com/golang/vtypes v0.0.0-20210728123451-f085971da113
)

// replace www.velocidex.com/golang/go-pe => /home/mic/projects/go-pe
// replace www.velocidex.com/golang/vfilter => /home/mic/projects/vfilter
// replace www.velocidex.com/golang/go-ntfs => /home/mic/projects/go-ntfs
// replace www.velocidex.com/golang/evtx => /home/mic/projects/evtx
// replace www.velocidex.com/golang/go-ese => /home/mic/projects/go-ese
// replace github.com/Velocidex/ordereddict => /home/mic/projects/ordereddict
// replace github.com/Velocidex/go-yara => /home/mic/projects/go-yara
// replace github.com/Velocidex/json => /home/mic/projects/json
// replace github.com/russross/blackfriday/v2 => /home/mic/projects/blackfriday
// replace www.velocidex.com/golang/vtypes => /home/mic/projects/vtypes

// replace github.com/Velocidex/zip => /home/mic/projects/zip
// replace github.com/Velocidex/sflags => /home/mic/projects/sflags

// replace github.com/Velocidex/etw => /home/mic/projects/etw

replace github.com/russross/blackfriday/v2 => github.com/Velocidex/blackfriday/v2 v2.0.2-0.20200811050547-4f26a09e2b3b

go 1.14

replace github.com/alecthomas/chroma => github.com/Velocidex/chroma v0.6.8-0.20200418131129-82edc291369c
