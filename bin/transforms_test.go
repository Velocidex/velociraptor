package main

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var (
	transformTC = []struct {
		in  []string
		out []string
		err string
	}{
		{
			// Just regular args without the -r flag.
			in: []string{"-config", "x.yaml", "artifacts",
				"collect", "Generic.Client.Info"},
			out: []string{"-config", "x.yaml", "artifacts",
				"collect", "Generic.Client.Info"},
		},
		{
			// Use the -r flag with no parameters
			in: []string{"-config", "x.yaml", "-r",
				"Generic.Client.Info"},
			out: []string{"-config", "x.yaml", "artifacts",
				"collect", "Generic.Client.Info"},
		},
		{
			// Use -r with artifact parameters
			in: []string{"-config", "x.yaml", "-r",
				"Generic.Client.Info", "--Foo", "Bar"},
			out: []string{"-config", "x.yaml", "artifacts",
				"collect", "Generic.Client.Info", "--args", "Foo=Bar"},
		},
		{
			// Use -r with artifact parameters with =
			in: []string{"-config", "x.yaml", "-r",
				"Generic.Client.Info", "--Foo=Bar"},
			out: []string{"-config", "x.yaml", "artifacts",
				"collect", "Generic.Client.Info", "--args", "Foo=Bar"},
		},
		{
			// Use -r with artifact parameters and -o flag
			in: []string{"-config", "x.yaml", "-r",
				"Generic.Client.Info", "--Foo", "Bar",
				"-o", "/tmp/test.zip"},
			out: []string{"-config", "x.yaml",
				"artifacts", "collect", "Generic.Client.Info",
				"--args", "Foo=Bar", "--output", "/tmp/test.zip"},
		},
		{
			// Use -r with artifact parameters and remote flags
			in: []string{"-api_config", "x.yaml", "-r",
				"Generic.Client.Info", "--Foo", "Bar",
				"-o", "/tmp/test.zip", "--org_id", "XYZ"},
			out: []string{"-api_config", "x.yaml",
				"artifacts", "collect", "Generic.Client.Info",
				"--args", "Foo=Bar", "--output", "/tmp/test.zip",
				"--org_id", "XYZ"},
		},
		{
			// Use -r with artifact parameters and -h flag
			in: []string{"-config", "x.yaml", "-r",
				"Generic.Client.Info", "--Foo", "Bar",
				"-h"},
			out: []string{"-config", "x.yaml",
				"artifacts", "collect", "--cli_help_mode",
				"Generic.Client.Info"},
		},

		// Handle errors:
		{
			in:  []string{"-config", "x.yaml", "-r"},
			err: "Expecting an artifact name to follow the --run flag",
		},
		{
			in:  []string{"-config", "x.yaml", "-r", "XXX", "--foo"},
			err: "Expecting a value to follow flag `--foo`",
		},
		{
			in:  []string{"-config", "x.yaml", "-r", "XXX", "--output"},
			err: "Expecting a value to follow `--output` flag",
		},
	}
)

func TestTransforms(t *testing.T) {
	for _, tc := range transformTC {
		out, err := transformArgv(tc.in)
		if tc.err != "" {
			assert.Error(t, err)
			assert.Regexp(t, tc.err, err.Error())
			continue
		}
		assert.NoError(t, err)
		assert.Equal(t, tc.out, out)
	}
}
