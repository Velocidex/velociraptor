module www.velocidex.com/golang/config_check

go 1.27.2

require (
	github.com/alecthomas/kingpin/v2 v2.4.0
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	www.velocidex.com/golang/velociraptor v0.0.0
)

require (
	github.com/Velocidex/json v0.0.0-20220224052537-92f3c0326e5a // indirect
	github.com/Velocidex/ordereddict v0.0.0-20261126150131-14d40b7b2eba // indirect
	github.com/Velocidex/ttlcache/v2 v2.9.1-0.20240517145123-a3f45e86e130 // indirect
	github.com/alecthomas/participle/v2 v2.1.4 // indirect
	github.com/alecthomas/repr v0.5.4 // indirect
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.18.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/valyala/fastjson v1.6.4 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v4 v4.0.0 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/net v0.63.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260724162435-b2f20204f0df // indirect
	google.golang.org/grpc v1.83.2 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	www.velocidex.com/golang/vfilter v0.0.0-20261126030731-e8d9b6c49516 // indirect
)

replace (
	github.com/go-errors/errors => github.com/Velocidex/errors v0.0.0-20221019164655-9ace6bf61e26
	go.yaml.in/yaml/v4 => github.com/Velocidex/go-yaml/v4 v4.0.0-20261126091409-7a2d92691f19
	www.velocidex.com/golang/velociraptor => ../../..
)
