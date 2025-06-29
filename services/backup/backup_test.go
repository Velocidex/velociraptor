package backup_test

import (
	"archive/zip"
	"context"
	"errors"
	"io/ioutil"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/backup"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"
)

type TestBackupProvider struct {
	name []string
	rows []*ordereddict.Dict

	// Restored rows from backup
	restored       []vfilter.Row
	restored_error error
}

func (self TestBackupProvider) ProviderName() string {
	return "TestProvider"
}

func (self TestBackupProvider) Name() []string {
	return self.name
}

func (self TestBackupProvider) BackupResults(
	ctx context.Context, wg *sync.WaitGroup,
	container services.BackupContainerWriter) (<-chan vfilter.Row, error) {

	output := make(chan vfilter.Row)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(output)

		output <- ordereddict.NewDict().
			Set("TestColumn", 1).
			Set("Foo", "Bar")
	}()

	return output, nil
}

func (self *TestBackupProvider) Restore(
	ctx context.Context,
	container services.BackupContainerReader,
	in <-chan vfilter.Row) (services.BackupStat, error) {

	if self.restored_error != nil {
		return services.BackupStat{
			Error:   self.restored_error,
			Message: self.restored_error.Error(),
		}, self.restored_error
	}

	for row := range in {
		self.restored = append(self.restored, row)
	}

	return services.BackupStat{
		Message: "All good!",
	}, nil
}

type BackupTestSuite struct {
	test_utils.TestSuite
}

func (self *BackupTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.BackupService = true
	self.ConfigObj.Defaults.BackupPeriodSeconds = -1

	self.TestSuite.SetupTest()

	client_info_manager, err := services.GetClientInfoManager(
		self.ConfigObj)
	assert.NoError(self.T(), err)

	client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: "C.12345",
		}})

}

func (self *BackupTestSuite) TestBackups() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(1000000000, 0)))
	defer closer()

	backup_service, err := services.GetBackupService(self.ConfigObj)
	assert.NoError(self.T(), err)

	provider := &TestBackupProvider{
		name: []string{"TestProvider.json"},
	}

	backup_service.Register(provider)

	// Now force a backup file
	export_path := path_specs.NewUnsafeFilestorePath(
		"backups", "backup_2001-09-09T01_46_40Z").
		SetType(api.PATH_TYPE_FILESTORE_DOWNLOAD_ZIP)
	stats, err := backup_service.(*backup.BackupService).
		CreateBackup(export_path)
	assert.NoError(self.T(), err)

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	// Backup file should be dependend on the mocked time.
	result := self.readBackupFile(export_path)
	prefix := "orgs/root/"
	test_provider, _ := result.Get(prefix + "TestProvider.json")
	golden := ordereddict.NewDict().
		Set("TestProvider.json", test_provider).
		Set("TestProvider Stats", filterStats(stats))

	opts := services.BackupRestoreOptions{}

	// Now restore the data from backup. NOTE: Each org restores only
	// its own data from the zip file. This allows the same zip file
	// to be shared between all the orgs.
	stats, err = backup_service.(*backup.BackupService).
		RestoreBackup(export_path, opts)
	assert.NoError(self.T(), err)

	golden.Set("RestoredTestProvider", provider.restored).
		Set("RestoredTestProviderStats", filterStats(stats))

	// Now restore the data from backup with an error
	provider.restored_error = errors.New("I have an error")
	provider.restored = nil

	stats, err = backup_service.(*backup.BackupService).
		RestoreBackup(export_path, opts)
	assert.NoError(self.T(), err)

	golden.Set("RestoredTestProvider With Error", provider.restored).
		Set("RestoredTestProviderStatsWithError", filterStats(stats))

	goldie.Assert(self.T(), "TestBackups",
		json.MustMarshalIndent(golden))
}

func filterStats(stats []services.BackupStat) (res []services.BackupStat) {
	for _, s := range stats {
		if s.Name == "TestProvider" {
			res = append(res, s)
		}
	}
	return res
}

func (self *BackupTestSuite) readBackupFile(
	export_path api.FSPathSpec) *ordereddict.Dict {
	file_store_factory := file_store.GetFileStore(self.ConfigObj)
	fd, err := file_store_factory.ReadFile(export_path)
	assert.NoError(self.T(), err)

	stats, err := fd.Stat()
	assert.NoError(self.T(), err)

	zip, err := zip.NewReader(
		utils.MakeReaderAtter(fd), stats.Size())
	assert.NoError(self.T(), err)

	files := ordereddict.NewDict()
	for _, f := range zip.File {
		member, err := f.Open()
		assert.NoError(self.T(), err)
		data, err := ioutil.ReadAll(member)
		assert.NoError(self.T(), err)
		files.Set(f.Name, strings.Split(string(data), "\n"))
	}

	return files
}

func TestBackupService(t *testing.T) {
	suite.Run(t, &BackupTestSuite{})
}
