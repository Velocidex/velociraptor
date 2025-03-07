package services

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/vfilter"
)

type NotebookType int

const (
	// For GetNotebook
	DO_NOT_INCLUDE_UPLOADS = false
	INCLUDE_UPLOADS        = true
)

func GetNotebookManager(config_obj *config_proto.Config) (NotebookManager, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).NotebookManager()
}

type TimelineOptions struct {
	IncludeComponents, ExcludeComponents []string
	Filter                               string
	StartTime                            time.Time
}

type TimelineReader interface {
	Read(ctx context.Context) <-chan *ordereddict.Dict
	Stat() *timelines_proto.SuperTimeline
}

type NotebookSearchOptions struct {
	// Only show notebooks accessible to this username
	Username string

	// Only show notebooks with timelines
	Timelines bool
}

type NotebookManager interface {

	// Notebook management
	GetNotebook(ctx context.Context,
		notebook_id string,
		include_uploads bool) (*api_proto.NotebookMetadata, error)

	GetSharedNotebooks(
		ctx context.Context,
		username string) (api.FSPathSpec, error)

	GetAllNotebooks(ctx context.Context,
		opts NotebookSearchOptions) ([]*api_proto.NotebookMetadata, error)

	NewNotebook(ctx context.Context,
		username string, in *api_proto.NotebookMetadata) (
		*api_proto.NotebookMetadata, error)

	NewNotebookCell(ctx context.Context,
		in *api_proto.NotebookCellRequest, username string) (
		*api_proto.NotebookMetadata, error)

	DeleteNotebook(ctx context.Context,
		notebook_id string, progress chan vfilter.Row,
		really_do_it bool) error

	UpdateNotebook(ctx context.Context, in *api_proto.NotebookMetadata) error

	GetNotebookCell(ctx context.Context,
		notebook_id, cell_id, version string) (*api_proto.NotebookCell, error)

	ReformatVQL(ctx context.Context, vql string) (string, error)

	// Update the cell and recalculate it.
	UpdateNotebookCell(ctx context.Context,
		notebook_metadata *api_proto.NotebookMetadata,
		user_name string,
		in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error)

	// Revert the cell to a different version. If the version does not
	// exist we get an error.
	RevertNotebookCellVersion(ctx context.Context,
		notebook_id, cell_id, version string) (*api_proto.NotebookCell, error)

	// Cancel a current operation
	CancelNotebookCell(ctx context.Context, notebook_id, cell_id, version string) error

	CheckNotebookAccess(
		notebook *api_proto.NotebookMetadata, user string) bool

	// Attachments
	UploadNotebookAttachment(ctx context.Context,
		in *api_proto.NotebookFileUploadRequest) (
		*api_proto.NotebookFileUploadResponse, error)

	RemoveNotebookAttachment(ctx context.Context,
		notebook_id string, components []string) error

	// Timeline management

	// List all timelines in this notebook
	Timelines(ctx context.Context,
		notebook_id string) ([]*timelines_proto.SuperTimeline, error)

	// Read a timeline, merging a set of components from it.
	ReadTimeline(ctx context.Context, notebook_id string,
		timeline string, options TimelineOptions) (
		TimelineReader, error)

	// Add events to a timeline
	AddTimeline(ctx context.Context, scope vfilter.Scope,
		notebook_id string, supertimeline string,
		timeline *timelines_proto.Timeline,
		in <-chan vfilter.Row) (*timelines_proto.SuperTimeline, error)

	AnnotateTimeline(ctx context.Context, scope vfilter.Scope,
		notebook_id string, supertimeline string,
		message, principal string,
		timestamp time.Time, event *ordereddict.Dict) error

	// Add events to a timeline
	DeleteTimeline(ctx context.Context, scope vfilter.Scope,
		notebook_id string, supertimeline, component string) error
}
