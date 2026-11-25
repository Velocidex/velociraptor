//go:build sumo
// +build sumo

package plugins

// kafka_upload() adds the franz-go Kafka client, so like the other
// cloud upload plugins it is only built into the sumo binaries.
import (
	_ "www.velocidex.com/golang/velociraptor/vql/tools/kafka"
)
