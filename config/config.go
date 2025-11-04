/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
package config

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Embed build time constants into here for reporting client version.
// https://husobee.github.io/golang/compile/time/variables/2015/12/03/compile-time-const.html
var (
	build_time  string
	commit_hash string
	ci_run_url  string

	versionCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "velociraptor_build",
		Help: "Current version of running binary.",
	}, []string{"commit_hash", "build_time"})
)

func GetVersion() *config_proto.Version {
	return &config_proto.Version{
		Name:         "velociraptor",
		Version:      constants.VERSION,
		BuildTime:    build_time,
		Commit:       commit_hash,
		CiBuildUrl:   ci_run_url,
		Compiler:     runtime.Version(),
		System:       runtime.GOOS,
		Architecture: utils.GetArch(),
	}
}

// Create a default configuration object.
func GetDefaultConfig() *config_proto.Config {
	result := &config_proto.Config{
		Client: &config_proto.ClientConfig{
			WritebackDarwin: "/etc/velociraptor.writeback.yaml",
			WritebackLinux:  "/etc/velociraptor.writeback.yaml",
			WritebackWindows: "$ProgramFiles\\Velociraptor\\" +
				"velociraptor.writeback.yaml",
			Level2WritebackSuffix: ".bak",
			TempdirWindows:        "$ProgramFiles\\Velociraptor\\Tools",
			MaxPoll:               60,

			// By default restart the client if we are unable to
			// contact the server within this long. (NOTE - even a
			// failed connection will reset the counter, the nanny
			// will only fire if the client has failed in some way -
			// e.g. the communicator is stopped for some reason).
			NannyMaxConnectionDelay: 600,

			// Local ring buffer to queue messages to the
			// server. If the server is not available we
			// write these to disk so we can send them
			// next time we are online.
			LocalBuffer: &config_proto.RingBufferConfig{
				MemorySize:      50 * 1024 * 1024,
				DiskSize:        1024 * 1024 * 1024,
				FilenameLinux:   "/var/tmp/Velociraptor_Buffer.bin",
				FilenameWindows: "$TEMP/Velociraptor_Buffer.bin",
				FilenameDarwin:  "/var/tmp/Velociraptor_Buffer.bin",
			},

			// Specific instructions for the
			// windows service installer.
			WindowsInstaller: &config_proto.WindowsInstallerConfig{
				ServiceName: "Velociraptor",
				InstallPath: "$ProgramFiles\\Velociraptor\\" +
					"Velociraptor.exe",
				ServiceDescription: "Velociraptor service",
			},

			DarwinInstaller: &config_proto.DarwinInstallerConfig{
				ServiceName: "com.velocidex.velociraptor",
				InstallPath: "/usr/local/bin/velociraptor",
			},

			MaxUploadSize: constants.MAX_POST_SIZE,
		},
		API: &config_proto.APIConfig{
			// Bind port for gRPC endpoint - this should not
			// normally be exposed.
			BindAddress: "127.0.0.1",
			BindPort:    8001,
			BindScheme:  "tcp",
		},
		GUI: &config_proto.GUIConfig{
			// Bind port for GUI. If you expose this on a
			// reachable IP address you must enable TLS!
			BindAddress:  "127.0.0.1",
			BindPort:     8889,
			PublicUrl:    "https://localhost:8889/app/index.html",
			ReverseProxy: []*config_proto.ReverseProxyConfig{},
			Authenticator: &config_proto.Authenticator{
				Type: "Basic",
			},
			Links: []*config_proto.GUILink{
				{
					Text:    "Documentation",
					Url:     "https://docs.velociraptor.app/",
					NewTab:  true,
					Type:    "sidebar",
					IconUrl: VeloIconDataURL,
				},
			},
		},
		CA: &config_proto.CAConfig{},
		Frontend: &config_proto.FrontendConfig{
			Hostname: "localhost",

			// A public interface for clients to
			// connect to.
			BindAddress: "0.0.0.0",
			BindPort:    8000,
			DefaultClientMonitoringArtifacts: []string{
				// Essential for client resource telemetry.
				"Generic.Client.Stats",
			},
			DynDns: &config_proto.DynDNSConfig{},
			Resources: &config_proto.FrontendResourceControl{
				ExpectedClients:        30000, // Controls RSA cache size
				ConnectionsPerSecond:   100,   // QPS load shedding limit (>1000 disable)
				Concurrency:            0,     // By default 2 * CPU count
				TargetHeapSize:         0,     // (Disabled) Set to control concurrency to match target heap size.
				NotificationsPerSecond: 30,
				MaxUploadSize:          constants.MAX_MEMORY * 2,
			},
			GRPCPoolMaxSize: 100,
			GRPCPoolMaxWait: 60,
		},
		Datastore: &config_proto.DatastoreConfig{
			Implementation: "FileBaseDataStore",

			// Users would probably need to change
			// this to something more permanent.
			Location:           "/var/tmp/velociraptor/",
			FilestoreDirectory: "/var/tmp/velociraptor/",

			// This is the default but we add it here for clarity. Set
			// to none to disable.
			Compression: "zlib",
		},
		Logging: &config_proto.LoggingConfig{
			// Disable debug logging by default.
			Debug: &config_proto.LoggingRetentionConfig{
				Disabled: true,
			},
			Info: &config_proto.LoggingRetentionConfig{
				RotationTime: 7 * 24 * 60 * 60,   // 7 days
				MaxAge:       365 * 24 * 60 * 60, // One year
			},
			Error: &config_proto.LoggingRetentionConfig{
				RotationTime: 7 * 24 * 60 * 60,   // 7 days
				MaxAge:       365 * 24 * 60 * 60, // One year
			},
			SeparateLogsPerComponent: true,
		},
		Monitoring: &config_proto.MonitoringConfig{
			BindAddress: "127.0.0.1",
			BindPort:    8003,
		},
		ApiConfig: &config_proto.ApiClientConfig{},
		Defaults: &config_proto.Defaults{
			HuntExpiryHours:        24 * 7,
			NotebookCellTimeoutMin: 10,
		},
		Security: &config_proto.Security{},
	}

	// The client's version needs to keep in sync with the
	// server's version.
	result.Version = GetVersion()

	// Only record some info about the server version.
	result.Client.ServerVersion = &config_proto.Version{
		Version:   result.Version.Version,
		BuildTime: result.Version.BuildTime,
		Commit:    result.Version.Commit,
	}

	// On windows we need slightly different defaults.
	if runtime.GOOS == "windows" {
		result.Datastore.Location = "C:\\Windows\\Temp"
		result.Datastore.FilestoreDirectory = "C:\\Windows\\Temp"
	}

	return result
}

func init() {
	// Tag the metrics with a build time. This is useful in a cluster
	// to see if all nodes are upgraded.
	versionCounter.With(prometheus.Labels{
		"commit_hash": commit_hash, "build_time": build_time}).Inc()
}
