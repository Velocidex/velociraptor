package backup

import (
	"archive/zip"
	"bufio"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type BackupService struct {
	mu         sync.Mutex
	ctx        context.Context
	wg         *sync.WaitGroup
	config_obj *config_proto.Config

	delay         time.Duration
	last_run      time.Time
	registrations []services.BackupProvider
	stats         []*BackupTrackerStats
}

func (self *BackupService) Start() {
	defer self.wg.Done()

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Starting <green>Backup Services</> for %v every %v",
		services.GetOrgName(self.config_obj), self.delay)

	for {
		self.mu.Lock()
		last_run := self.last_run
		self.mu.Unlock()

		select {
		case <-self.ctx.Done():
			return

		case <-utils.GetTime().After(utils.Jitter(self.delay)):
			// Avoid doing backups too quickly. This is mainly for
			// tests where the time is mocked for the After(delay)
			// above does not work.
			if utils.GetTime().Now().Sub(last_run) < time.Second {
				utils.SleepWithCtx(self.ctx, time.Minute)
				continue
			}

			export_path := paths.NewBackupPathManager().BackupFile()
			_, err := self.CreateBackup(export_path)
			if err != nil {
				logger := logging.GetLogger(
					self.config_obj, &logging.FrontendComponent)
				logger.Error("Backup Service: CreateBackup: %v", err)
			}
		}
	}
}

func (self *BackupService) CreateBackup(
	export_path api.FSPathSpec) (stats []services.BackupStat, err error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	self.last_run = utils.GetTime().Now()

	tracker_stats := NewBackupTrackerStats(export_path)
	defer func() {
		tracker_stats.Stats = append([]services.BackupStat{}, stats...)

		self.stats = append(self.stats, tracker_stats)
		if len(self.stats) > 10 {
			self.stats = self.stats[len(self.stats)-10:]
		}
	}()

	// Create a container to hold the backup
	file_store_factory := file_store.GetFileStore(self.config_obj)

	// Delay shutdown until the file actually hits the disk
	self.wg.Add(1)
	fd, err := file_store_factory.WriteFileWithCompletion(
		export_path, self.wg.Done)
	if err != nil {
		return nil, err
	}

	err = fd.Truncate()
	if err != nil {
		return nil, err
	}

	// Create a container with the file.
	container, err := reporting.NewContainerFromWriter(
		export_path.String(), self.config_obj, fd, "", 5, nil)
	if err != nil {
		fd.Close()
		return nil, err
	}

	defer func() {
		zip_stats := container.Stats()

		logger.Info("BackupService: <green>Completed Backup to %v (size %v) in %v</>",
			export_path.String(), zip_stats.TotalCompressedBytes,
			utils.GetTime().Now().Sub(self.last_run))

		stats = append(stats, services.BackupStat{
			Name: "BackupService",
			Message: fmt.Sprintf("Completed Backup to %v (size %v) in %v",
				export_path.String(), zip_stats.TotalCompressedBytes,
				utils.GetTime().Now().Sub(self.last_run)),
		})

	}()

	defer container.Close()

	org_container := &containerDelegate{
		Container: container,
	}

	// The root org is responsible for dumping all child orgs as well.
	if utils.IsRootOrg(self.config_obj.OrgId) {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return stats, err
		}

		// Ask all the other orgs to also write their backups in this
		// container.
		for _, org := range org_manager.ListOrgs() {
			backup, err := org_manager.Services(org.Id).BackupService()
			if err != nil {
				continue
			}

			prefix := fmt.Sprintf("orgs/%v", org.Id)
			org_container := &containerDelegate{
				Container: container,
				prefix:    prefix,
			}
			org_stats, _ := backup.(*BackupService).writeBackups(
				org_container, prefix)
			stats = append(stats, org_stats...)
		}

		// Not the root org, just backup this org only.
	} else if !utils.IsRootOrg(self.config_obj.OrgId) {
		prefix := fmt.Sprintf("orgs/%v", utils.GetOrgId(self.config_obj))
		org_stats, _ := self.writeBackups(org_container, prefix)
		stats = append(stats, org_stats...)
	}

	return stats, err
}

// Invoke each provider to produce the backup rows.
func (self *BackupService) writeBackups(
	container services.BackupContainerWriter,
	prefix string) (stats []services.BackupStat, err error) {

	// Now we can dump all providers into the file.
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)

	for _, provider := range self.registrations {
		dest := strings.Join(append([]string{prefix}, provider.Name()...), "/")
		stat := services.BackupStat{
			Name:  provider.ProviderName(),
			OrgId: utils.GetOrgId(self.config_obj),
		}

		rows, err := provider.BackupResults(self.ctx, self.wg, container)
		if err != nil {
			logger.Info("BackupService: <red>Error writing to %v: %v",
				dest, err)

			stat.Error = err
			stats = append(stats, stat)
			continue
		}

		// Write the results to the container now
		total_rows, err := container.WriteResultSet(
			self.ctx, self.config_obj, dest, rows)
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
	export_path api.FSPathSpec,
	opts services.BackupRestoreOptions) (stats []services.BackupStat, err error) {
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

	prefix := opts.Prefix
	if prefix == "" {
		prefix = fmt.Sprintf("orgs/%v", utils.GetOrgId(self.config_obj))
	}

	for _, provider := range self.registrations {
		if opts.ProviderRegex != nil && !opts.ProviderRegex.MatchString(
			provider.ProviderName()) {
			continue
		}

		stat, err := self.feedProvider(provider, zipDelegate{
			Reader: zip_reader,
			prefix: prefix,
		})
		if err != nil {
			dest := strings.Join(provider.Name(), "/")
			logger.Info("BackupService: <red>Error restoring to %v: %v",
				dest, err)
			stat.Name = provider.ProviderName()
			stat.Error = err
			stat.OrgId = utils.GetOrgId(self.config_obj)
		}
		stats = append(stats, stat)
	}

	return stats, nil
}

// Feed the provider from backup rows so it can restore the state.
func (self *BackupService) feedProvider(
	provider services.BackupProvider,
	container services.BackupContainerReader) (stat services.BackupStat, err error) {

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
		stat.OrgId = utils.GetOrgId(self.config_obj)

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
		stat, err := provider.Restore(sub_ctx, container, output)
		if err != nil {
			stat.Error = err
		}

		// Stop new rows to be written - we dont care any more.
		cancel()

		// Pass the result to the main routine.
		results <- stat
	}()

	// Now dump the rows into the provider.
	for row := range utils.ReadJsonFromFile(sub_ctx, reader) {
		select {
		case <-sub_ctx.Done():
			return stat, nil

		case output <- row:
		}
	}

	return stat, nil
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
		delay:      time.Hour * 24,
		last_run:   utils.GetTime().Now(),
	}

	// Every day

	if config_obj.Defaults != nil {
		// Backups are disabled.
		if config_obj.Defaults.BackupPeriodSeconds < 0 {
			return result
		}

		if config_obj.Defaults.BackupPeriodSeconds > 0 {
			result.delay = time.Duration(
				config_obj.Defaults.BackupPeriodSeconds) * time.Second
		}
	}

	wg.Add(1)
	go result.Start()

	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "Backups " + utils.GetOrgId(config_obj),
		Description:   "Show recent backup operations",
		ProfileWriter: result.ProfileWriter,
		Categories: []string{"Org", services.GetOrgName(config_obj),
			"Services"},
	})

	return result
}
