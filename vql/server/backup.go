package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type BackupPluginArgs struct {
	Name string `vfilter:"required,field=name,doc=The name of the backup file."`
}

type BackupPlugin struct{}

func (self BackupPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("backup: %v", err)
			return
		}

		arg := &BackupPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("backup: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("backup: Command can only run on the server")
			return
		}

		backups, err := services.GetBackupService(config_obj)
		if err != nil {
			scope.Log("backup: %v", err)
			return
		}

		path_spec := paths.NewBackupPathManager().CustomBackup(arg.Name)
		stats, err := backups.CreateBackup(path_spec)
		if err != nil {
			scope.Log("backup: %v", err)
			return
		}

		for _, s := range stats {
			select {
			case <-ctx.Done():
				return
			case output_chan <- transformStat(s):
			}
		}
	}()

	return output_chan
}

func (self BackupPlugin) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "backup",
		Doc:     "Generates a backup file.",
		ArgType: type_map.AddType(scope, &BackupPluginArgs{}),
	}
}

type RestoreBackupPlugin struct{}

func (self RestoreBackupPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("backup_restore: %v", err)
			return
		}

		arg := &BackupPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("backup_restore: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("backup_restore: Command can only run on the server")
			return
		}

		backups, err := services.GetBackupService(config_obj)
		if err != nil {
			scope.Log("backup_restore: %v", err)
			return
		}

		path_spec := paths.NewBackupPathManager().CustomBackup(arg.Name)
		stats, err := backups.RestoreBackup(path_spec)
		if err != nil {
			scope.Log("backup_restore: %v", err)
			return
		}

		for _, s := range stats {
			select {
			case <-ctx.Done():
				return
			case output_chan <- transformStat(s):
			}
		}
	}()

	return output_chan
}

func transformStat(s services.BackupStat) *ordereddict.Dict {
	result := ordereddict.NewDict().
		Set("Name", s.Name).
		Set("Error", "").
		Set("Message", s.Message)

	if s.Error != nil {
		result.Update("Error", s.Error.Error())
	}

	return result
}

func (self RestoreBackupPlugin) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "backup_restore",
		Doc:     "Restore state from a backup file.",
		ArgType: type_map.AddType(scope, &BackupPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&BackupPlugin{})
	vql_subsystem.RegisterPlugin(&RestoreBackupPlugin{})
}
