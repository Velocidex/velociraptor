package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	deaddisk_command = app.Command(
		"deaddisk", "Create a deaddisk configuration")

	deaddisk_command_output = deaddisk_command.Arg(
		"output", "Output file to write config on").
		Required().OpenFile(os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)

	deaddisk_command_hostname = deaddisk_command.Flag("hostname",
		"The hostname to impersonate").Default("Virtual Host").String()

	deaddisk_command_add_windows_disk = deaddisk_command.Flag(
		"add_windows_disk", "Add a Windows Hard Disk Image").String()

	deaddisk_command_add_windows_directory = deaddisk_command.Flag(
		"add_windows_directory", "Add a Windows mounted directory").String()

	standardRegistryMounts = []struct {
		prefix, path, key_path string
	}{
		{"HKEY_LOCAL_MACHINE\\Software", "/Windows/System32/Config/SOFTWARE", "/"},
		{"HKEY_LOCAL_MACHINE\\System", "/Windows/System32/Config/SYSTEM", "/"},
		{"HKEY_LOCAL_MACHINE\\System\\CurrentControlSet",
			"/Windows/System32/Config/SYSTEM", "/ControlSet001"},
	}
)

func addWindowsDirectory(
	directory_path string, config_obj *config_proto.Config) error {
	addCommonPermissions(config_obj)

	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(&LogWriter{config_obj}, "", 0),
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	addCommonShadowAccessors(config_obj)
	impersonationClause(config_obj, "windows", *deaddisk_command_hostname)

	scope.Log("Adding windows mounted directory at %v", directory_path)

	// Mount the directory as the "file", "auto" and "ntfs" accessor
	config_obj.Remappings = append(config_obj.Remappings,
		&config_proto.RemappingConfig{
			Type: "mount",
			Description: fmt.Sprintf(
				"Mount the directory %v on the C: drive (NTFS)",
				directory_path),
			From: &config_proto.MountPoint{
				Accessor: "file",
				Prefix:   directory_path,
			},
			On: &config_proto.MountPoint{
				Accessor: "ntfs",
				Prefix:   "\\\\.\\C:",
				PathType: "ntfs",
			},
		})

	config_obj.Remappings = append(config_obj.Remappings,
		&config_proto.RemappingConfig{
			Type: "mount",
			Description: fmt.Sprintf(
				"Mount the directory %v on the C: drive (FILE Accessor)",
				directory_path),
			From: &config_proto.MountPoint{
				Accessor: "file",
				Prefix:   directory_path,
			},
			On: &config_proto.MountPoint{
				Accessor: "file",
				Prefix:   "C:",
				PathType: "windows",
			},
		})

	config_obj.Remappings = append(config_obj.Remappings,
		&config_proto.RemappingConfig{
			Type: "mount",
			Description: fmt.Sprintf(
				"Mount the directory %v on the C: drive (Auto Accessor)",
				directory_path),
			From: &config_proto.MountPoint{
				Accessor: "file",
				Prefix:   directory_path,
			},
			On: &config_proto.MountPoint{
				Accessor: "auto",
				Prefix:   "C:",
				PathType: "windows",
			},
		})

	// Now add some registry mounts
	for _, definition := range standardRegistryMounts {
		hive_path := filepath.Join(directory_path, definition.path)
		scope.Log("Checking for hive at %v", hive_path)
		_, err := os.Stat(hive_path)
		if err != nil {
			continue
		}

		config_obj.Remappings = append(config_obj.Remappings,
			&config_proto.RemappingConfig{
				Type: "mount",
				Description: fmt.Sprintf(
					"Map the %s Registry hive on %s (Prefixed at %v)",
					hive_path, definition.prefix, definition.key_path),
				From: &config_proto.MountPoint{
					Accessor: "raw_reg",
					Prefix: fmt.Sprintf(`{
  "Path": %q,
  "DelegateAccessor": "file",
  "DelegatePath": %q
}`, definition.key_path, hive_path),
					PathType: "registry",
				},
				On: &config_proto.MountPoint{
					Accessor: "registry",
					Prefix:   definition.prefix,
					PathType: "registry",
				},
			})
	}
	return nil
}

func addWindowsHardDisk(image string, config_obj *config_proto.Config) error {
	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(&LogWriter{config_obj}, "", 0),
		Env: ordereddict.NewDict().
			Set(vql_subsystem.ACL_MANAGER_VAR,
				vql_subsystem.NewRoleACLManager("administrator")).
			Set("ImagePath", image),
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	scope.Log("Enumerating partitions using Windows.Forensics.PartitionTable")
	query := `
SELECT *
FROM Artifact.Windows.Forensics.PartitionTable(ImagePath=ImagePath)
`
	vqls, err := vfilter.MultiParse(query)
	if err != nil {
		return fmt.Errorf("Unable to parse VQL Query: %w", err)
	}

	ctx, cancel := InstallSignalHandler(nil, scope)
	defer cancel()

	for _, vql := range vqls {
		for row := range vql.Eval(ctx, scope) {
			// Here we are looking for a partition with a Windows
			// directory
			if checkForName(scope, row, "TopLevelDirectory", "Windows") {
				addWindowsPartition(config_obj, scope, image, row)
			}
		}
	}

	addCommonShadowAccessors(config_obj)

	return nil
}

func doDeadDisk() error {
	full_config_obj, err := APIConfigLoader.WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	sm, err := startEssentialServices(full_config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	config_obj := &config_proto.Config{
		Remappings: full_config_obj.Remappings,
	}

	if *deaddisk_command_add_windows_disk != "" {
		abs_path, err := filepath.Abs(*deaddisk_command_add_windows_disk)
		if err != nil {
			return err
		}

		err = addWindowsHardDisk(abs_path, config_obj)
		if err != nil {
			return err
		}
	}

	if *deaddisk_command_add_windows_directory != "" {
		abs_path, err := filepath.Abs(*deaddisk_command_add_windows_directory)
		if err != nil {
			return err
		}

		err = addWindowsDirectory(abs_path, config_obj)
		if err != nil {
			return err
		}
	}

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		return err
	}
	_, err = (*deaddisk_command_output).Write(res)
	return err
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case deaddisk_command.FullCommand():
			FatalIfError(deaddisk_command, doDeadDisk)

		default:
			return false
		}
		return true
	})
}

func checkForName(
	scope vfilter.Scope, row vfilter.Row,
	column string, regex string) bool {
	top_level_any, pres := scope.Associative(row, column)
	if pres {
		TopLevelDirectory, ok := top_level_any.([]vfilter.Any)
		if ok {
			re := regexp.MustCompile(regex)
			scope.Log("Searching for a Windows directory at the top level")
			for _, i := range TopLevelDirectory {
				i_str, ok := i.(string)
				if !ok {
					continue
				}

				if re.MatchString(i_str) {
					return true
				}
			}

		}
	}
	return false
}

func addPermission(config_obj *config_proto.Config, perm string) {
	var permission_clause *config_proto.RemappingConfig
	for _, item := range config_obj.Remappings {
		if item.Type == "permissions" {
			permission_clause = item
			break
		}
	}

	if permission_clause == nil {
		permission_clause = &config_proto.RemappingConfig{
			Type: "permissions",
		}

		config_obj.Remappings = append(config_obj.Remappings, permission_clause)
	}

	perm = strings.ToUpper(perm)
	if !utils.InString(permission_clause.Permissions, perm) {
		permission_clause.Permissions = append(
			permission_clause.Permissions, perm)
	}
}

func impersonationClause(
	config_obj *config_proto.Config, os_type string,
	hostname string) {
	var impersonation_clause *config_proto.RemappingConfig
	for _, item := range config_obj.Remappings {
		if item.Type == "impersonation" {
			impersonation_clause = item
			break
		}
	}

	if impersonation_clause == nil {
		impersonation_clause = &config_proto.RemappingConfig{
			Type: "impersonation",
		}

		config_obj.Remappings = append(config_obj.Remappings, impersonation_clause)
	}

	impersonation_clause.Os = os_type
	impersonation_clause.Hostname = hostname

	// Disable plugins that normally need live mode to work.
	impersonation_clause.DisabledPlugins = []string{
		"users", "certificates", "handles", "pslist", "interfaces",
		"modules", "netstat", "partitions", "proc_dump", "proc_yara", "vad",
		"winobj", "wmi",
	}
	impersonation_clause.DisabledFunctions = []string{
		"amsi", "lookupSID", "token"}

	switch os_type {
	case "windows":
		impersonation_clause.Env = append(impersonation_clause.Env,
			&actions_proto.VQLEnv{Key: "SystemRoot", Value: "C:\\Windows"},
			&actions_proto.VQLEnv{Key: "WinDir", Value: "C:\\Windows"},
		)
	}
}

func addCommonShadowAccessors(config_obj *config_proto.Config) {
	for _, item := range []string{"zip", "raw_reg", "data"} {
		config_obj.Remappings = append(config_obj.Remappings,
			&config_proto.RemappingConfig{
				Type: "shadow",
				From: &config_proto.MountPoint{
					Accessor: item,
				},
				On: &config_proto.MountPoint{
					Accessor: item,
				},
			})
	}
}

func addCommonPermissions(config_obj *config_proto.Config) {
	// Add some basic permissions
	// For actually collecting artifacts
	addPermission(config_obj, "COLLECT_CLIENT")

	// For reading filesystem accessors
	addPermission(config_obj, "FILESYSTEM_READ")

	// For writing collection zip
	addPermission(config_obj, "FILESYSTEM_WRITE")

	// If running on the server (e.g. with velociraptor gui) we need
	// to be able to do server things
	addPermission(config_obj, "READ_RESULTS")
	addPermission(config_obj, "MACHINE_STATE")
	addPermission(config_obj, "SERVER_ADMIN")
}

func addWindowsPartition(
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	image string,
	row vfilter.Row) {
	addCommonPermissions(config_obj)

	partition_start := vql_subsystem.GetIntFromRow(scope, row, "StartOffset")

	scope.Log("Adding windows partition at offset %v", partition_start)

	impersonationClause(config_obj, "windows", *deaddisk_command_hostname)

	mount_point := &config_proto.MountPoint{
		Accessor: "raw_ntfs",
		Prefix: fmt.Sprintf(`{
  "DelegateAccessor": "offset",
  "Delegate": {
    "DelegateAccessor": "file",
    "DelegatePath": %q,
    "Path":"%d"
  },
  "Path": "/"
}
`, image, partition_start),
	}

	// Add an NTFS mount accessible via the "ntfs" accessor
	config_obj.Remappings = append(config_obj.Remappings,
		&config_proto.RemappingConfig{
			Type: "mount",
			Description: fmt.Sprintf(
				"Mount the partition %v (offset %v) on the C: drive (NTFS)",
				image, partition_start),
			From: mount_point,
			On: &config_proto.MountPoint{
				Accessor: "ntfs",
				Prefix:   "\\\\.\\C:",
				PathType: "ntfs",
			},
		})

	// Add a "file" mount so operations of the file accessor
	// transparently use the ntfs accessor.
	config_obj.Remappings = append(config_obj.Remappings,
		&config_proto.RemappingConfig{
			Type: "mount",
			Description: fmt.Sprintf(
				"Mount the partition %v (offset %v) on the C: drive (File Accessor)",
				image, partition_start),
			From: mount_point,
			On: &config_proto.MountPoint{
				Accessor: "file",
				Prefix:   "C:",
				PathType: "windows",
			},
		})

	// Also mount the auto accessor for the default.
	config_obj.Remappings = append(config_obj.Remappings,
		&config_proto.RemappingConfig{
			Type: "mount",
			Description: fmt.Sprintf(
				"Mount the partition %v (offset %v) on the C: drive (Auto Accessor)",
				image, partition_start),
			From: mount_point,
			On: &config_proto.MountPoint{
				Accessor: "auto",
				Prefix:   "C:",
				PathType: "windows",
			},
		})

	// Now add some registry mounts
	for _, definition := range standardRegistryMounts {
		config_obj.Remappings = append(config_obj.Remappings,
			&config_proto.RemappingConfig{
				Type: "mount",
				Description: fmt.Sprintf(
					"Map the %s Registry hive on %s (Prefixed at %v)",
					definition.path, definition.prefix, definition.key_path),
				From: &config_proto.MountPoint{
					Accessor: "raw_reg",
					Prefix: fmt.Sprintf(`{
  "Path": %q,
  "DelegateAccessor": "raw_ntfs",
  "Delegate": {
    "DelegateAccessor":"offset",
    "Delegate": {
      "DelegateAccessor": "file",
      "DelegatePath": %q,
      "Path": "%d"
    },
    "Path":%q
  }
}`, definition.key_path, image, partition_start, definition.path),
					PathType: "registry",
				},
				On: &config_proto.MountPoint{
					Accessor: "registry",
					Prefix:   definition.prefix,
					PathType: "registry",
				},
			})
	}
}
