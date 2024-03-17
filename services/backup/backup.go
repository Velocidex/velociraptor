package backup

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type BackupService struct {
	mu         sync.Mutex
	ctx        context.Context
	wg         *sync.WaitGroup
	config_obj *config_proto.Config

	registrations []services.BackupProvider
}

func (self *BackupService) CreateBackup(
	export_path api.FSPathSpec) (stats []services.BackupStat, err error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	start := utils.GetTime().Now()

	// Create a container to hold the backup
	file_store_factory := file_store.GetFileStore(self.config_obj)

	// Delay shutdown until the file actually hits the disk
	self.wg.Add(1)
	fd, err := file_store_factory.WriteFileWithCompletion(
		export_path, self.wg.Done)
	if err != nil {
		return nil, err
	}

	fd.Truncate()

	// Create a container with the file.
	container, err := reporting.NewContainerFromWriter(
		self.config_obj, fd, "", 5, nil)
	if err != nil {
		fd.Close()
		return nil, err
	}

	defer func() {
		zip_stats := container.Stats()

		logger.Info("BackupService: <green>Completed Backup to %v (size %v) in %v</>",
			export_path.String(), zip_stats.TotalCompressedBytes,
			utils.GetTime().Now().Sub(start))

		stats = append(stats, services.BackupStat{
			Name: "BackupService",
			Message: fmt.Sprintf("Completed Backup to %v (size %v) in %v",
				export_path.String(), zip_stats.TotalCompressedBytes,
				utils.GetTime().Now().Sub(start)),
		})

	}()

	defer container.Close()

	// Now we can dump all providers into the file.
	scope := vql_subsystem.MakeScope()

	for _, provider := range self.registrations {
		dest := strings.Join(provider.Name(), "/")
		stat := services.BackupStat{
			Name: provider.ProviderName(),
		}

		rows, err := provider.BackupResults(self.ctx, self.wg)
		if err != nil {
			logger.Info("BackupService: <red>Error writing to %v: %v",
				dest, err)

			stat.Error = err
			stats = append(stats, stat)
			continue
		}

		// Write the results to the container now
		total_rows, err := container.WriteResultSet(self.ctx, self.config_obj,
			scope, reporting.ContainerFormatJson, dest, rows)
		if err != nil {
			logger.Info("BackupService: <red>Error writing to %v: %v",
				dest, err)
			stat.Error = err
			stats = append(stats, stat)
			continue
		}

		stat.Message = fmt.Sprintf("Wrote %v rows", total_rows)
		stats = append(stats, stat)
	}

	return stats, nil
}

// Opens a backup file and recovers all the data in it.
func (self *BackupService) RestoreBackup(
	export_path api.FSPathSpec) (stats []services.BackupStat, err error) {
	// Create a container to hold the backup
	file_store_factory := file_store.GetFileStore(self.config_obj)

	// Delay shutdown until the file actually hits the disk
	fd, err := file_store_factory.ReadFile(export_path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	fd_stats, err := fd.Stat()
	if err != nil {
		return nil, err
	}

	zip_reader, err := zip.NewReader(
		utils.MakeReaderAtter(fd), fd_stats.Size())
	if err != nil {
		return nil, err
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)

	for _, provider := range self.registrations {
		stat, err := self.feedProvider(provider, zip_reader)
		if err != nil {
			dest := strings.Join(provider.Name(), "/")
			logger.Info("BackupService: <red>Error restoring to %v: %v",
				dest, err)
			stat.Name = provider.ProviderName()
			stat.Error = err
		}
		stats = append(stats, stat)
	}

	return stats, nil
}

func (self *BackupService) feedProvider(
	provider services.BackupProvider,
	container *zip.Reader) (stat services.BackupStat, err error) {

	dest := strings.Join(provider.Name(), "/")
	member, err := container.Open(dest)
	if err != nil {
		return stat, err
	}
	defer member.Close()

	reader := bufio.NewReader(member)

	// Wait for the provider to finish before we go to the next
	// provider.
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	// The provider will return a result when done.
	results := make(chan services.BackupStat)
	defer func() {
		// Wait here until the provider is done.
		stat = <-results
		stat.Name = provider.ProviderName()
		if stat.Error != nil {
			err = stat.Error
		}
	}()

	// Feed rows into this channel so provider can restore backups.
	output := make(chan vfilter.Row)
	defer close(output)

	sub_ctx, cancel := context.WithCancel(self.ctx)

	// Feed the provider in the background
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(results)

		// Preserve the provider error as our return
		stat, err := provider.Restore(sub_ctx, output)
		if err != nil {
			stat.Error = err
		}

		// Stop new rows to be written - we dont care any more.
		cancel()

		// Pass the result to the main routine.
		results <- stat
	}()

	// Now dump the rows into the provider.
	for {
		row_data, err := reader.ReadBytes('\n')
		if len(row_data) == 0 || errors.Is(err, io.EOF) {
			return stat, nil
		}
		if err != nil {
			return stat, err
		}

		row := ordereddict.NewDict()
		err = json.Unmarshal(row_data, &row)
		if err != nil {
			return stat, err
		}

		select {
		case <-sub_ctx.Done():
			return stat, nil

		case output <- row:
		}
	}
}

func (self *BackupService) Register(provider services.BackupProvider) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.registrations = append(self.registrations, provider)
}

func NewBackupService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) services.BackupService {

	result := &BackupService{
		ctx:        ctx,
		wg:         wg,
		config_obj: config_obj,
	}

	// Every day
	delay := time.Hour * 24
	if config_obj.Defaults != nil {
		// Backups are disabled.
		if config_obj.Defaults.BackupPeriodSeconds < 0 {
			return result
		}

		if config_obj.Defaults.BackupPeriodSeconds > 0 {
			delay = time.Duration(
				config_obj.Defaults.BackupPeriodSeconds) * time.Second
		}
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Starting <green>Backup Services</> for %v",
		services.GetOrgName(config_obj))

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return

			case <-utils.GetTime().After(delay):
				export_path := paths.NewBackupPathManager().
					BackupFile()
				result.CreateBackup(export_path)
			}
		}
	}()

	return result
}
