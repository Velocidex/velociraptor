package notebook

import (
	"context"
	"errors"
	"strings"
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
		parts := strings.SplitN(ps.Base(), "/", 2)
		if len(parts) < 2 {
			continue
		}

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name: parts[1],
			Size: uint64(item.Size()),
			Date: item.ModTime().UTC().Format(time.RFC3339),
			Type: api.GetExtensionForFilestore(ps),
			Stats: &api_proto.ContainerStats{
				Components: ps.Components(),
			},
		})
	}

	return result, nil
}

func (self *NotebookManager) RemoveNotebookAttachment(ctx context.Context,
	notebook_id string, components []string) error {
	return self.Store.RemoveAttachment(ctx, notebook_id, components)
}

func (self *NotebookStoreImpl) RemoveAttachment(ctx context.Context,
	notebook_id string, components []string) error {

	if len(components) == 0 {
		return errors.New("Attachment components empty")
	}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	attachment_path := notebook_path_manager.AttachmentDirectory().
		AddUnsafeChild(components[len(components)-1])

	file_store_factory := file_store.GetFileStore(self.config_obj)
	return file_store_factory.Delete(attachment_path)
}
