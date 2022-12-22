package notebook

import (
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
)

func (self *NotebookStoreImpl) GetAvailableTimelines(notebook_id string) []string {
	path_manager := paths.NewNotebookPathManager(notebook_id)
	result := []string{}
	db, err := datastore.GetDB(self.config_obj)
	files, err := db.ListChildren(self.config_obj, path_manager.SuperTimelineDir())
	if err != nil {
		return nil
	}

	for _, f := range files {
		if !f.IsDir() {
			result = append(result, f.Base())
		}
	}
	return result
}

func (self *NotebookStoreImpl) GetAvailableDownloadFiles(
	notebook_id string) (*api_proto.AvailableDownloads, error) {

	download_path := paths.NewNotebookPathManager(notebook_id).
		HtmlExport().Dir()

	return reporting.GetAvailableDownloadFiles(self.config_obj, download_path)
}

func (self *NotebookStoreImpl) GetAvailableUploadFiles(notebook_id string) (
	*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	file_store_factory := file_store.GetFileStore(self.config_obj)
	files, err := file_store_factory.ListDirectory(
		notebook_path_manager.AttachmentDirectory())
	if err != nil {
		return nil, err
	}

	for _, item := range files {
		ps := item.PathSpec()

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name:     item.Name(),
			Type:     api.GetExtensionForFilestore(ps),
			Path:     ps.AsClientPath(),
			Size:     uint64(item.Size()),
			Date:     item.ModTime().UTC().Format(time.RFC3339),
			Complete: true,
		})
	}

	return result, nil
}
