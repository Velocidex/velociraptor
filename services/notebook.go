package services

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func GetNotebookManager(config_obj *config_proto.Config) (NotebookManager, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).NotebookManager()
}

type NotebookManager interface {
	GetNotebook(ctx context.Context, notebook_id string) (
		*api_proto.NotebookMetadata, error)

	GetSharedNotebooks(ctx context.Context,
		username string, offset, count uint64) ([]*api_proto.NotebookMetadata, error)

	NewNotebook(ctx context.Context,
		username string, in *api_proto.NotebookMetadata) (
		*api_proto.NotebookMetadata, error)

	NewNotebookCell(ctx context.Context,
		in *api_proto.NotebookCellRequest, username string) (
		*api_proto.NotebookMetadata, error)

	UpdateNotebook(ctx context.Context, in *api_proto.NotebookMetadata) error

	UpdateShareIndex(notebook *api_proto.NotebookMetadata) error

	GetNotebookCell(ctx context.Context,
		notebook_id, cell_id string) (*api_proto.NotebookCell, error)

	// Update the cell and recalculate it.
	UpdateNotebookCell(ctx context.Context,
		notebook_metadata *api_proto.NotebookMetadata,
		user_name string,
		in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error)

	// Cancel a current operation
	CancelNotebookCell(ctx context.Context, notebook_id, cell_id string) error

	CheckNotebookAccess(
		notebook *api_proto.NotebookMetadata, user string) bool

	UploadNotebookAttachment(ctx context.Context,
		in *api_proto.NotebookFileUploadRequest) (
		*api_proto.NotebookFileUploadResponse, error)
}
