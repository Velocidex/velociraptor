package services

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/vfilter"
)

// Callers may register a backup provider to be included in the backup
type BackupProvider interface {
	// The name of this provider
	ProviderName() string

	// The name of the result saved in the container
	Name() []string

	// Providers may write result sets into the backup. This will be
	// called by the backup service to obtain a channel over which we
	// can write the backup file (named in Name() above).
	BackupResults(
		ctx context.Context,
		wg *sync.WaitGroup) (<-chan vfilter.Row, error)

	// This is the opposite of backup - it allows a provider to
	// recover from an existing backup. Typcially providers need to
	// clear their data and read new data from this channel. The
	// provider may return stats about its operation.
	Restore(ctx context.Context, in <-chan vfilter.Row) (BackupStat, error)
}

// Alows each provider to report the stats of the most recent
// operation.
type BackupStat struct {
	// Name of provider
	Name    string
	Error   error
	Message string
}

type BackupService interface {
	Register(provider BackupProvider)
	RestoreBackup(export_path api.FSPathSpec) ([]BackupStat, error)
	CreateBackup(export_path api.FSPathSpec) ([]BackupStat, error)
}

func GetBackupService(config_obj *config_proto.Config) (BackupService, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).BackupService()
}
