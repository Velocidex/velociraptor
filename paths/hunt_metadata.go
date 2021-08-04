package paths

import (
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type HuntPathManager struct {
	path    api.PathSpec
	hunt_id string
}

func (self HuntPathManager) Path() api.PathSpec {
	return self.path
}

// Get the file store path for placing the download zip for the flow.
func (self HuntPathManager) GetHuntDownloadsFile(only_combined bool,
	base_filename string) api.PathSpec {
	suffix := ""
	if only_combined {
		suffix = "-summary"
	}

	return DOWNLOADS_ROOT.AddUnsafeChild(
		"hunts", self.hunt_id,
		base_filename+self.hunt_id+suffix).SetType(
		api.PATH_TYPE_FILESTORE_DOWNLOAD_ZIP)
}

func NewHuntPathManager(hunt_id string) *HuntPathManager {
	return &HuntPathManager{
		path:    HUNTS_ROOT.AddChild(hunt_id),
		hunt_id: hunt_id,
	}
}

func (self HuntPathManager) Stats() api.PathSpec {
	return self.path.AddChild("stats")
}

func (self HuntPathManager) HuntDirectory() api.PathSpec {
	return HUNTS_ROOT
}

// Get result set for storing participating clients.
func (self HuntPathManager) Clients() api.PathSpec {
	return HUNTS_ROOT.AddChild(self.hunt_id).SetType(
		api.PATH_TYPE_FILESTORE_JSON)
}

// Where to store client errors.
func (self HuntPathManager) ClientErrors() api.PathSpec {
	return HUNTS_ROOT.AddChild(self.hunt_id + "_errors").SetType(
		api.PATH_TYPE_FILESTORE_JSON)
}
