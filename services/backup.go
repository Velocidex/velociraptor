package services

import (
	"context"
	"io"
	"io/fs"
	"regexp"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/vfilter"
)

// For writing.
type BackupContainerWriter interface {
	Create(name string, mtime time.Time) (io.WriteCloser, error)

	WriteResultSet(
		ctx context.Context,
		config_obj *config_proto.Config,
		dest string, in <-chan vfilter.Row) (total_rows int, err error)
}

// For reading.
type BackupContainerReader interface {
	Open(name string) (fs.File, error)
}

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
		wg *sync.WaitGroup,
		container BackupContainerWriter) (<-chan vfilter.Row, error)

	// This is the opposite of backup - it allows a provider to
	// recover from an existing backup. Typcially providers need to
	// clear their data and read new data from this channel. The
	// provider may return stats about its operation.
	Restore(ctx context.Context,
		container BackupContainerReader,
		in <-chan vfilter.Row) (BackupStat, error)
}

// Alows each provider to report the stats of the most recent
// operation.
type BackupStat struct {
	// Name of provider
	Name    string
	Error   error
	Message string

	// Which org owns this backup service.
	OrgId string

	Warnings []string
}

type BackupRestoreOptions struct {
	// By default the prefix in the zip is calculated based on the
	// current org id. This allows this to be overriden.
	Prefix string

	// Only restore matching providers
	ProviderRegex *regexp.Regexp
}

type BackupService interface {
	Register(provider BackupProvider)
	RestoreBackup(export_path api.FSPathSpec,
		opts BackupRestoreOptions) ([]BackupStat, error)
	CreateBackup(export_path api.FSPathSpec) ([]BackupStat, error)
}

func GetBackupService(config_obj *config_proto.Config) (BackupService, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).BackupService()
}
