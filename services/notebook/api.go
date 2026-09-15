package notebook

import (
	"context"
	"sync/atomic"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/vfilter"
)

var (
	// Parameter to RemoveNotebookCell to ignore progress reports.
	IGNORE_REPORT chan *ordereddict.Dict = nil

	DO_NOT_SYNC_NOTEBOOKS_FOR_TEST = atomic.Bool{}
)

type NotebookStore interface {
	// TODO: The following Get/Modify/Set pattern is not thread safe -
	// Enhance the API to allow safe modifications.
	SetNotebook(in *api_proto.NotebookMetadata) error
	GetNotebook(notebook_id string) (*api_proto.NotebookMetadata, error)
	DeleteNotebook(ctx context.Context,
		notebook_id string, progress chan vfilter.Row,
		really_do_it bool) error

	// Manage notebook cells
	SetNotebookCell(notebook_id string, in *api_proto.NotebookCell) error
	GetNotebookCell(notebook_id, cell_id, version string) (
		*api_proto.NotebookCell, error)

	// progress_chan receives information about deletion. It may be
	// nil if callers don't care about it.
	RemoveNotebookCell(
		ctx context.Context, config_obj *config_proto.Config,
		notebook_id, cell_id, version string,
		progress_chan chan *ordereddict.Dict) error

	GetAllNotebooks(ctx context.Context, opts services.NotebookSearchOptions) (
		[]*api_proto.NotebookMetadata, error)

	// The latest time of all the global notebooks. Used to work out
	// if we need to rebuild the notebook index.
	Version() int64

	// Get the next available version number to tag notebooks.
	GetNextVersion() int64

	// Returns a result set of all the notebooks which are either
	// owned or shared with the user.
	GetSharedNotebooks(
		ctx context.Context,
		username string) (api.FSPathSpec, error)
}

type AttachmentManager interface {
	GetAvailableUploadFiles(notebook_id string) (*api_proto.AvailableDownloads, error)

	GetAvailableDownloadFiles(
		ctx context.Context, notebook_id string) (*api_proto.AvailableDownloads, error)

	RemoveAttachment(ctx context.Context,
		notebook_id string, components []string) error

	StoreAttachment(notebook_id,
		filename string, data []byte) (api.FSPathSpec, error)
}
