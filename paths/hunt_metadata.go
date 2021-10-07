package paths

import (
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type HuntPathManager struct {
	path    api.DSPathSpec
	hunt_id string
}

func (self HuntPathManager) Path() api.DSPathSpec {
	return self.path
}

// Get the file store path for placing the download zip for the flow.
func (self HuntPathManager) GetHuntDownloadsFile(only_combined bool,
	base_filename string, locked bool) api.FSPathSpec {
	suffix := ""
	if only_combined {
		suffix = "-summary"
	}
	filename := base_filename + self.hunt_id + suffix
	if locked {
		filename += "_locked"
	}

	return DOWNLOADS_ROOT.AddUnsafeChild(
		"hunts", self.hunt_id, filename).SetType(
		api.PATH_TYPE_FILESTORE_DOWNLOAD_ZIP)
}

func NewHuntPathManager(hunt_id string) *HuntPathManager {
	return &HuntPathManager{
		path:    HUNTS_ROOT.AddChild(hunt_id),
		hunt_id: hunt_id,
	}
}

func (self HuntPathManager) Stats() api.DSPathSpec {
	return self.path.AddChild("stats")
}

func (self HuntPathManager) HuntDirectory() api.DSPathSpec {
	return HUNTS_ROOT
}

// Get result set for storing participating clients.
func (self HuntPathManager) Clients() api.FSPathSpec {
	return HUNTS_ROOT.AddChild(self.hunt_id).AsFilestorePath()
}

// Where to store client errors.
func (self HuntPathManager) ClientErrors() api.FSPathSpec {
	return HUNTS_ROOT.AddChild(self.hunt_id + "_errors").
		AsFilestorePath()
}
