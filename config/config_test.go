package config

import (
	"testing"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

func TestConfig(t *testing.T) {
	config_obj, err := LoadConfig("test_data/server.local.yaml")
	if err != nil {
		t.Fatal(err)
	}
	utils.Debug(config_obj)
}
