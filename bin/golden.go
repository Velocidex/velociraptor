package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/sergi/go-diff/diffmatchpatch"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	logging "www.velocidex.com/golang/velociraptor/logging"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	golden_command = app.Command(
		"golden", "Run tests and compare against golden files.")

	golden_command_prefix = golden_command.Arg(
		"prefix", "Golden file prefix").Required().String()
)

type testFixture struct {
	Name       string
	Parameters map[string]string
	Queries    []string
}

func runTest(fixture *testFixture) (string, error) {
	config_obj := get_config_or_default()
	repository := getRepository(config_obj)

	env := vfilter.NewDict().
		Set("config", config_obj.Client).
		Set("server_config", config_obj)

	for k, v := range fixture.Parameters {
		env.Set(k, v)
	}

	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = logging.NewPlainLogger(config_obj,
		&logging.ToolComponent)

	result := ""
	for _, query := range fixture.Queries {
		result += query
		vql, err := vfilter.Parse(query)
		if err != nil {
			return "", err
		}

		result_chan := vfilter.GetResponseChannel(
			vql, context.Background(), scope, 1000, 100)
		for {
			query_result, ok := <-result_chan
			if !ok {
				break
			}
			result += string(query_result.Payload)
		}
	}

	return result, nil
}

func doGolden() {
	globs, err := filepath.Glob(fmt.Sprintf(
		"%s*.in.yaml", *golden_command_prefix))
	kingpin.FatalIfError(err, "Glob")

	failures := []string{}

	for _, filename := range globs {
		data, err := ioutil.ReadFile(filename)
		kingpin.FatalIfError(err, "Reading file")

		fixture := testFixture{}
		err = yaml.Unmarshal(data, &fixture)
		kingpin.FatalIfError(err, "Unmarshal input file")

		result, err := runTest(&fixture)
		kingpin.FatalIfError(err, "Running test")

		outfile := strings.Replace(filename, ".in.", ".out.", -1)
		old_data, err := ioutil.ReadFile(outfile)
		if err == nil {
			if string(old_data) != result {
				dmp := diffmatchpatch.New()
				diffs := dmp.DiffMain(
					string(old_data), result, false)
				fmt.Printf("Failed %v:\n", filename)
				fmt.Println(dmp.DiffPrettyText(diffs))
				failures = append(failures, filename)
			}
		} else {
			fmt.Printf("New file for  %v:\n", filename)
			fmt.Println(result)
			failures = append(failures, filename)
		}

		err = ioutil.WriteFile(
			outfile,
			[]byte(result), 0666)
		kingpin.FatalIfError(err, "Unable to write golden file")
	}

	if len(failures) > 0 {
		kingpin.Fatalf(
			"Failed! Some golden files did not match:%s\n",
			failures)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "golden":
			doGolden()
		default:
			return false
		}
		return true
	})
}
