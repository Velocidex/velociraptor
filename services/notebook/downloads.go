package notebook

import (
	"context"
	"errors"
	"os"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type AttachmentManagerImpl struct {
	config_obj    *config_proto.Config
	NotebookStore NotebookStore
}

func NewAttachmentManager(config_obj *config_proto.Config,
	NotebookStore NotebookStore) AttachmentManager {
	return &AttachmentManagerImpl{
		config_obj:    config_obj,
		NotebookStore: NotebookStore,
	}
}

func (self *AttachmentManagerImpl) GetAvailableDownloadFiles(
	ctx context.Context, notebook_id string) (*api_proto.AvailableDownloads, error) {

	export_manager, err := services.GetExportManager(self.config_obj)
	if err != nil {
		return nil, err
	}

	return export_manager.GetAvailableDownloadFiles(ctx, self.config_obj,
		services.ContainerOptions{
			Type:       services.NotebookExport,
			NotebookId: notebook_id,
		})
}

func (self *AttachmentManagerImpl) GetAvailableUploadFiles(notebook_id string) (
	*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	file_store_factory := file_store.GetFileStore(self.config_obj)

	notebook, err := self.NotebookStore.GetNotebook(notebook_id)
	if err != nil {
		return nil, err
	}

	for _, cell_metadata := range notebook.CellMetadata {
		cell_manager := notebook_path_manager.Cell(
			cell_metadata.CellId, cell_metadata.CurrentVersion)

		upload_directory := cell_manager.UploadsDir()

		err := api.Walk(file_store_factory, cell_manager.UploadsDir(),
			func(ps api.FSPathSpec, info os.FileInfo) error {
				ps = path_specs.ToAnyType(ps)

				// Build the vfs path by showing the relative path of
				// the path spec relative to the uploads directory in
				// the cell. Uploads may be nested in arbitrary paths.
				vfs_path_spec := path_specs.NewUnsafeFilestorePath(
					ps.Components()[len(upload_directory.Components()):]...).
					SetType(ps.Type())

				result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
					Name: ps.Base(),
					Size: uint64(info.Size()),
					Date: info.ModTime().UTC().Format(time.RFC3339),
					Stats: &api_proto.ContainerStats{
						Components: ps.Components(),
						Type:       api.GetExtensionForFilestore(ps),
						VfsPath:    vfs_path_spec.AsClientPath(),
					},
				})
				return nil
			})
		if err != nil {
			return nil, err
		}
	}

	attachment_directory := notebook_path_manager.AttachmentDirectory()
	attachment_directory_components := len(attachment_directory.Components())

	// Also include attachments
	items, _ := file_store_factory.ListDirectory(attachment_directory)
	for _, item := range items {
		ps := path_specs.ToAnyType(item.PathSpec())

		// Build the vfs path by showing the relative path of
		// the path spec relative to the attachment
		vfs_path_spec := path_specs.NewUnsafeFilestorePath(
			ps.Components()[attachment_directory_components:]...).
			SetType(ps.Type())

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name: ps.Base(),
			Size: uint64(item.Size()),
			Date: item.ModTime().UTC().Format(time.RFC3339),
			Stats: &api_proto.ContainerStats{
				Components: ps.Components(),
				Type:       api.GetExtensionForFilestore(ps),
				VfsPath:    vfs_path_spec.AsClientPath(),
			},
		})
	}

	return result, nil
}

func (self *NotebookManager) RemoveNotebookAttachment(ctx context.Context,
	notebook_id string, components []string) error {
	return self.AttachmentManager.RemoveAttachment(ctx, notebook_id, components)
}

func (self *AttachmentManagerImpl) RemoveAttachment(ctx context.Context,
	notebook_id string, components []string) error {

	if len(components) == 0 {
		return errors.New("Attachment components empty")
	}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	attachment_path := path_specs.NewUnsafeFilestorePath(components...).
		SetType(api.PATH_TYPE_FILESTORE_ANY)
	if !path_specs.IsSubPath(
		notebook_path_manager.NotebookDirectory(),
		attachment_path) {
		return errors.New("Attachment must be within the notebook directory")
	}

	file_store_factory := file_store.GetFileStore(self.config_obj)
	return file_store_factory.Delete(attachment_path)
}

func (self *AttachmentManagerImpl) StoreAttachment(
	notebook_id, filename string, data []byte) (api.FSPathSpec, error) {
	full_path := paths.NewNotebookPathManager(notebook_id).
		Attachment(filename)
	file_store_factory := file_store.GetFileStore(self.config_obj)
	fd, err := file_store_factory.WriteFileWithCompletion(
		full_path, utils.SyncCompleter)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	err = fd.Truncate()
	if err != nil {
		return nil, err
	}

	_, err = fd.Write(data)
	return full_path, err
}
