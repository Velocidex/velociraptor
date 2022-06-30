package services

import (
	"context"
	"errors"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

var (
	mu               sync.Mutex
	notebook_manager NotebookManager
)

func GetNotebookManager() (NotebookManager, error) {
	mu.Lock()
	defer mu.Unlock()

	if notebook_manager == nil {
		return nil, errors.New("Notebook Manager not initialized")
	}

	return notebook_manager, nil
}

func RegisterNotebookManager(m NotebookManager) {
	mu.Lock()
	defer mu.Unlock()

	notebook_manager = m
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
