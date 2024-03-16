package paths

import (
	"strings"
	"time"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

type BackupsPathManager struct{}

func (self BackupsPathManager) CustomBackup(
	name string) api.FSPathSpec {
	return BACKUPS_ROOT.AddUnsafeChild(name).
		SetType(api.PATH_TYPE_FILESTORE_DOWNLOAD_ZIP)
}

func (self BackupsPathManager) BackupFile() api.FSPathSpec {
	now := utils.GetTime().Now().UTC()
	return BACKUPS_ROOT.AddChild("backup_" +
		strings.Replace(now.Format(time.RFC3339), ":", "_", -1)).
		SetType(api.PATH_TYPE_FILESTORE_DOWNLOAD_ZIP)
}

func NewBackupPathManager() BackupsPathManager {
	return BackupsPathManager{}
}
