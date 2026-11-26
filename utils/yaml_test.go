package utils

import (
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils/yaml"
)

type strictYAMLConfig struct {
	Name string `json:"display_name"`
}

func TestYAMLConfigRoundTrip(t *testing.T) {
	for _, name := range []string{"client.config.yaml", "server.config.yaml"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "http_comms", "test_data", name))
			if err != nil {
				t.Fatal(err)
			}
			config := &config_proto.Config{}
			if err := YamlUnmarshalStrict(data, config); err != nil {
				t.Fatal(err)
			}
			if config.GetVersion().GetBuildTime() == "" || len(config.GetClient().GetServerUrls()) == 0 {
				t.Fatal("expected build_time and Client.server_urls from the fixture")
			}
			out, err := yaml.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			reloaded := &config_proto.Config{}
			if err := YamlUnmarshalStrict(out, reloaded); err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(config, reloaded) {
				t.Fatal("config changed during YAML round trip")
			}
		})
	}
}

func TestYamlUnmarshalStrict(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "valid configuration",
			input: "display_name: Velociraptor\n",
		},
		{
			name:    "unknown field",
			input:   "display_name: Velociraptor\nunknown: value\n",
			wantErr: true,
		},
		{
			name:    "duplicate field",
			input:   "display_name: Velociraptor\ndisplay_name: duplicate\n",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var config strictYAMLConfig
			err := YamlUnmarshalStrict([]byte(test.input), &config)
			if (err != nil) != test.wantErr {
				t.Fatalf("YamlUnmarshalStrict() error = %v, wantErr %v", err, test.wantErr)
			}
			if !test.wantErr && config.Name != "Velociraptor" {
				t.Fatalf("expected name to be decoded, got %q", config.Name)
			}
		})
	}
}
