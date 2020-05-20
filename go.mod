module www.velocidex.com/golang/velociraptor

require (
	cloud.google.com/go v0.47.0 // indirect
	cloud.google.com/go/bigquery v1.1.0 // indirect
	cloud.google.com/go/storage v1.1.1
	github.com/Depado/bfchroma v1.2.0
	github.com/Masterminds/goutils v1.1.0 // indirect
	github.com/Masterminds/semver v1.4.2 // indirect
	github.com/Masterminds/sprig v2.20.0+incompatible
	github.com/Netflix/go-expect v0.0.0-20190729225929-0e00d9168667 // indirect
	github.com/Showmax/go-fqdn v0.0.0-20180501083314-6f60894d629f
	github.com/Velocidex/ahocorasick v0.0.0-20180712114356-e1c353eeaaee
	github.com/Velocidex/cgofuse v1.1.2
	github.com/Velocidex/go-elasticsearch/v7 v7.3.1-0.20191001125819-fee0ef9cac6b
	github.com/Velocidex/go-yara v1.1.10-0.20200414034554-457848df11f9
	github.com/Velocidex/ordereddict v0.0.0-20200428070154-4cd604876fe5
	github.com/Velocidex/survey v1.8.7-0.20190926071832-2ff99cc7aa49
	github.com/Velocidex/yaml/v2 v2.2.5
	github.com/ZachtimusPrime/Go-Splunk-HTTP v0.0.0-20200420213219-094ff9e8d788
	github.com/alecthomas/assert v0.0.0-20170929043011-405dbfeb8e38
	github.com/alecthomas/chroma v0.7.2
	github.com/alecthomas/participle v0.4.4
	github.com/alecthomas/repr v0.0.0-20200325044227-4184120f674c // indirect
	github.com/alexmullins/zip v0.0.0-20180717182244-4affb64b04d0
	github.com/aws/aws-sdk-go v1.26.7
	github.com/c-bata/go-prompt v0.2.3
	github.com/clbanning/mxj v1.8.4
	github.com/creack/pty v1.1.9 // indirect
	github.com/crewjam/saml v0.3.1
	github.com/davecgh/go-spew v1.1.1
	github.com/daviddengcn/go-colortext v0.0.0-20180409174941-186a3d44e920
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dustin/go-humanize v1.0.0
	github.com/elastic/go-elasticsearch/v7 v7.3.0 // indirect
	github.com/elastic/go-libaudit v0.4.0
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/fastly/go-utils v0.0.0-20180712184237-d95a45783239 // indirect
	github.com/go-ole/go-ole v1.2.4
	github.com/go-sql-driver/mysql v1.5.0
	github.com/golang/mock v1.4.3
	github.com/golang/protobuf v1.4.0-rc.4.0.20200313231945-b860323f09d0
	github.com/golang/snappy v0.0.1
	github.com/golangplus/bytes v0.0.0-20160111154220-45c989fe5450 // indirect
	github.com/golangplus/fmt v0.0.0-20150411045040-2a5d6d7d2995 // indirect
	github.com/golangplus/testing v0.0.0-20180327235837-af21d9c3145e // indirect
	github.com/google/shlex v0.0.0-20181106134648-c34317bd91bf
	github.com/google/uuid v1.1.1 // indirect
	github.com/gorilla/csrf v1.6.2
	github.com/gorilla/schema v1.1.0
	github.com/grpc-ecosystem/grpc-gateway v1.11.3
	github.com/hanwen/go-fuse v1.0.1-0.20190726130028-2f298055551b
	github.com/hillu/go-ntdll v0.0.0-20190226223014-dd4204aa705e
	github.com/hillu/go-yara v1.2.2 // indirect
	github.com/hinshun/vt10x v0.0.0-20180809195222-d55458df857c // indirect
	github.com/huandu/xstrings v1.2.0 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/jehiah/go-strftime v0.0.0-20171201141054-1d33003b3869 // indirect
	github.com/jmoiron/sqlx v1.2.0
	github.com/jstemmer/go-junit-report v0.9.1 // indirect
	github.com/juju/ratelimit v1.0.1
	github.com/kierdavis/dateparser v0.0.0-20171227112021-81e70b820720
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/kr/pretty v0.2.0 // indirect
	github.com/kr/pty v1.1.8 // indirect
	github.com/lestrrat-go/file-rotatelogs v2.2.0+incompatible
	github.com/lestrrat-go/strftime v0.0.0-20190725011945-5c849dd2c51d // indirect
	github.com/lib/pq v1.2.0 // indirect
	github.com/magefile/mage v1.9.0
	github.com/mattn/go-pointer v0.0.0-20180825124634-49522c3f3791
	github.com/mattn/go-runewidth v0.0.8 // indirect
	github.com/mattn/go-sqlite3 v1.11.0
	github.com/mattn/go-tty v0.0.0-20190424173100-523744f04859 // indirect
	github.com/microcosm-cc/bluemonday v1.0.2
	github.com/olekukonko/tablewriter v0.0.2
	github.com/pkg/errors v0.9.1
	github.com/pkg/term v0.0.0-20190109203006-aa71e9d9e942 // indirect
	github.com/processout/grpc-go-pool v1.2.1
	github.com/prometheus/client_golang v1.2.1
	github.com/rifflock/lfshook v0.0.0-20180920164130-b9218ef580f5
	github.com/robertkrimen/otto v0.0.0-20180617131154-15f95af6e78d
	github.com/russross/blackfriday/v2 v2.0.1
	github.com/sebdah/goldie v1.0.0
	github.com/sergi/go-diff v1.0.0
	github.com/shirou/gopsutil v0.0.0-20190627142359-4c8b404ee5c5
	github.com/sirupsen/logrus v1.4.2
	github.com/stretchr/testify v1.5.1
	github.com/tebeka/strftime v0.1.3 // indirect
	github.com/tink-ab/tempfile v0.0.0-20180226111222-33beb0518f1a
	github.com/vjeantet/grok v1.0.0
	github.com/xor-gate/ar v0.0.0-20170530204233-5c72ae81e2b7 // indirect
	github.com/xor-gate/debpkg v0.0.0-20181217150151-a0c70a3d4213
	golang.org/x/crypto v0.0.0-20200414173820-0848c9571904
	golang.org/x/exp v0.0.0-20191014171548-69215a2ee97e // indirect
	golang.org/x/mod v0.2.0 // indirect
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e // indirect
	golang.org/x/sys v0.0.0-20200501145240-bc7a7d42d5c3
	golang.org/x/tools v0.0.0-20200207131002-533eb2654509 // indirect
	google.golang.org/api v0.11.0
	google.golang.org/appengine v1.6.5 // indirect
	google.golang.org/genproto v0.0.0-20200409111301-baae70f3302d
	google.golang.org/grpc v1.28.1
	google.golang.org/protobuf v1.21.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
	gopkg.in/sourcemap.v1 v1.0.5 // indirect
	gopkg.in/yaml.v2 v2.2.8 // indirect
	howett.net/plist v0.0.0-20181124034731-591f970eefbb
	www.velocidex.com/golang/evtx v0.0.2-0.20200505002428-02e98e3e472b
	www.velocidex.com/golang/go-ese v0.0.0-20200111070159-4b7484475321
	www.velocidex.com/golang/go-ntfs v0.0.0-20200110083657-950cbe916617
	www.velocidex.com/golang/go-pe v0.1.1-0.20191103232346-ac12e8190bb6
	www.velocidex.com/golang/go-prefetch v0.0.0-20190703150313-0469fa2f85cf
	www.velocidex.com/golang/oleparse v0.0.0-20190327031422-34195d413196
	www.velocidex.com/golang/regparser v0.0.0-20190625082115-b02dc43c2500
	www.velocidex.com/golang/vfilter v0.0.0-20200520014446-e58bd882ee1a
	www.velocidex.com/golang/vtypes v0.0.0-20180924145839-b0d509f8925b
)

// replace www.velocidex.com/golang/vfilter => /home/mic/projects/vfilter
// replace www.velocidex.com/golang/go-ntfs => /home/mic/projects/go-ntfs
// replace www.velocidex.com/golang/evtx => /home/mic/projects/evtx
// replace www.velocidex.com/golang/go-ese => /home/mic/projects/go-ese
// replace github.com/Velocidex/ordereddict => /home/mic/projects/ordereddict

// replace www.velocidex.com/golang/go-yara => /home/mic/projects/go-yara

go 1.13

replace github.com/alecthomas/chroma => github.com/Velocidex/chroma v0.6.8-0.20200418131129-82edc291369c
