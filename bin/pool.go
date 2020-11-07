/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"runtime"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"github.com/shirou/gopsutil/host"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config "www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	pool_client_command = app.Command(
		"pool_client", "Run a pool client for load testing.")

	pool_client_number = pool_client_command.Flag(
		"number", "Total number of clients to run.").Int()

	pool_client_writeback_dir = pool_client_command.Flag(
		"writeback_dir", "The directory to store all writebacks.").Default(".").
		ExistingDir()
)

func doPoolClient() {
	registerMockInfo()

	number_of_clients := *pool_client_number
	if number_of_clients <= 0 {
		number_of_clients = 2
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client_config, err := DefaultConfigLoader.
		WithRequiredClient().
		WithVerbose(true).
		LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	sm, err := startEssentialServices(client_config)
	kingpin.FatalIfError(err, "Starting services.")
	defer sm.Close()

	server.IncreaseLimits(client_config)

	// Make a copy of all the configs for each client.
	configs := make([]*config_proto.Config, 0, number_of_clients)
	serialized, _ := json.Marshal(client_config)

	for i := 0; i < number_of_clients; i++ {
		client_config := &config_proto.Config{}
		err := json.Unmarshal(serialized, &client_config)
		kingpin.FatalIfError(err, "Copying configs.")
		configs = append(configs, client_config)
	}

	for i := 0; i < number_of_clients; i++ {
		go func(i int) {
			client_config := configs[i]
			filename := fmt.Sprintf("pool_client.yaml.%d", i)
			client_config.Client.WritebackLinux = path.Join(
				*pool_client_writeback_dir, filename)

			client_config.Client.WritebackWindows = client_config.Client.WritebackLinux

			existing_writeback := &config_proto.Writeback{}
			writeback, err := config.WritebackLocation(client_config)
			kingpin.FatalIfError(err, "Unable to load writeback file")

			data, err := ioutil.ReadFile(writeback)

			// Failing to read the file is not an error - the file may not
			// exist yet.
			if err == nil {
				err = yaml.Unmarshal(data, existing_writeback)
				kingpin.FatalIfError(err, "Unable to load config file "+filename)
			}

			// Merge the writeback with the config.
			client_config.Writeback = existing_writeback

			// Make sure the config is ok.
			err = crypto.VerifyConfig(client_config)
			if err != nil {
				kingpin.FatalIfError(err, "Invalid config")
			}

			manager, err := crypto.NewClientCryptoManager(
				client_config, []byte(client_config.Writeback.PrivateKey))
			kingpin.FatalIfError(err, "Unable to parse config file")

			exe, err := executor.NewClientExecutor(ctx, client_config)
			kingpin.FatalIfError(err, "Can not create executor.")

			comm, err := http_comms.NewHTTPCommunicator(
				client_config,
				manager,
				exe,
				client_config.Client.ServerUrls,
				nil,
				utils.RealClock{},
			)
			kingpin.FatalIfError(err, "Can not create HTTPCommunicator.")

			// Run the client in the background.
			comm.Run(ctx)
		}(i)
	}

	// Block forever.
	<-ctx.Done()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "pool_client":
			doPoolClient()
		default:
			return false
		}
		return true
	})
}

func registerMockInfo() {
	vql.OverridePlugin(
		vfilter.GenericListPlugin{
			PluginName: "info",
			Function: func(
				scope *vfilter.Scope,
				args *ordereddict.Dict) []vfilter.Row {
				var result []vfilter.Row

				config_obj, ok := artifacts.GetServerConfig(scope)
				if !ok {
					return result
				}

				key, err := crypto.ParseRsaPrivateKeyFromPemStr(
					[]byte(config_obj.Writeback.PrivateKey))
				if err != nil {
					scope.Log("info: %s", err)
					return result
				}

				client_id := crypto.ClientIDFromPublicKey(&key.PublicKey)

				me, _ := os.Executable()
				info, err := host.Info()
				if err != nil {
					scope.Log("info: %s", err)
					return result
				}

				fqdn := fmt.Sprintf("%s.%s", info.Hostname, client_id)
				item := ordereddict.NewDict().
					Set("Hostname", fqdn).
					Set("Uptime", info.Uptime).
					Set("BootTime", info.BootTime).
					Set("Procs", info.Procs).
					Set("OS", info.OS).
					Set("Platform", info.Platform).
					Set("PlatformFamily", info.PlatformFamily).
					Set("PlatformVersion", info.PlatformVersion).
					Set("KernelVersion", info.KernelVersion).
					Set("VirtualizationSystem", info.VirtualizationSystem).
					Set("VirtualizationRole", info.VirtualizationRole).
					Set("Fqdn", fqdn).
					Set("Architecture", runtime.GOARCH).
					Set("Exe", me)
				result = append(result, item)
				return result
			},
		})
}
