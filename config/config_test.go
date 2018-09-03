package config

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/utils"
)

func TestConfig(t *testing.T) {
	config_obj, err := LoadConfig("test_data/server.local.yaml")
	if err != nil {
		t.Fatal(err)
	}
	utils.Debug(config_obj)
}
