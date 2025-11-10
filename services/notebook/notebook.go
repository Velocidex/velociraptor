package notebook

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path"
	"strings"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/reformat"
)

var (
	invalidNotebookId = errors.New("Invalid notebook id")
)

type NotebookManager struct {
	config_obj *config_proto.Config
	Store      NotebookStore

	SuperTimelineStorer        timelines.ISuperTimelineStorer
	SuperTimelineReaderFactory timelines.ISuperTimelineReader
	SuperTimelineWriterFactory timelines.ISuperTimelineWriter
	SuperTimelineAnnotator     timelines.ISuperTimelineAnnotator
	AttachmentManager          AttachmentManager
}

func (self *NotebookManager) GetNotebook(
	ctx context.Context, notebook_id string, include_uploads bool) (
	*api_proto.NotebookMetadata, error) {

	err := verifyNotebookId(notebook_id)
	if err != nil {
		return nil, err
	}

	notebook, err := self.Store.GetNotebook(notebook_id)
	if err != nil {
		return nil, err
	}

	// Global notebooks keep these internally.
	if include_uploads {
		// An error here just means there are no AvailableDownloads.
		notebook.AvailableDownloads, _ = self.AttachmentManager.GetAvailableDownloadFiles(
			ctx, notebook_id)
		notebook.AvailableUploads, _ = self.AttachmentManager.GetAvailableUploadFiles(
			notebook_id)
		notebook.Timelines = self.SuperTimelineStorer.GetAvailableTimelines(
			ctx, notebook_id)
	} else {
		notebook.AvailableUploads = nil
		notebook.AvailableDownloads = nil
		notebook.Timelines = nil
	}

	return notebook, nil
}

func (self *NotebookManager) NewNotebook(
	ctx context.Context, username string, in *api_proto.NotebookMetadata) (
	*api_proto.NotebookMetadata, error) {

	// Override these attributes
	in.Creator = username
	in.CreatedTime = utils.GetTime().Now().Unix()
	in.ModifiedTime = in.CreatedTime

	// Allow hunt notebooks to be created with a specified hunt ID.
	if !strings.HasPrefix(in.NotebookId, "N.H.") &&
		!strings.HasPrefix(in.NotebookId, "N.F.") &&
		!strings.HasPrefix(in.NotebookId, "N.E.") {
		in.NotebookId = NewNotebookId()
	}

	if in.NotebookId == "" {
		in.NotebookId = NewNotebookId()
	}

	err := self.Store.SetNotebook(in)
	if err != nil {
		return nil, err
	}

	err = self.CreateInitialNotebook(ctx, self.config_obj, in, username)
	if err != nil {
		return nil, err
	}

	// Get the freshest version of the notebook
	notebook, err := self.Store.GetNotebook(in.NotebookId)
	return notebook, err
}

func (self *NotebookManager) UpdateNotebook(
	ctx context.Context, in *api_proto.NotebookMetadata) error {
	err := verifyNotebookId(in.NotebookId)
	if err != nil {
		return err
	}

	in.ModifiedTime = utils.GetTime().Now().Unix()

	psuedo_artifact, out, err := CalculateNotebookArtifact(
		ctx, self.config_obj, in)
	if err != nil {
		return err
	}

	spec, err := CalculateSpecs(ctx, self.config_obj, psuedo_artifact, out)
	if err != nil {
		return err
	}

	// Update the requests based on the artifact specs.
	err = updateNotebookRequests(
		ctx, self.config_obj, psuedo_artifact, spec, out)
	if err != nil {
		return err
	}

	return self.Store.SetNotebook(out)
}

func (self *NotebookManager) GetNotebookCell(ctx context.Context,
	notebook_id, cell_id, version string) (*api_proto.NotebookCell, error) {

	err := verifyNotebookId(notebook_id)
	if err != nil {
		return nil, err
	}

	notebook_cell, err := self.Store.GetNotebookCell(notebook_id, cell_id, version)

	// Cell does not exist, make it a default cell.
	if errors.Is(err, os.ErrNotExist) {
		return &api_proto.NotebookCell{
			Input:             "",
			Output:            "",
			Data:              "{}",
			CellId:            cell_id,
			CurrentVersion:    version,
			AvailableVersions: []string{version},
			Type:              "Markdown",
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return notebook_cell, nil
}

// Cancel a current operation
func (self *NotebookManager) CancelNotebookCell(
	ctx context.Context, notebook_id, cell_id, version string) error {

	err := verifyNotebookId(notebook_id)
	if err != nil {
		return err
	}

	// Unset the calculating bit in the notebook in case the
	// renderer is not actually running (e.g. server restart).
	notebook_cell, err := self.Store.GetNotebookCell(notebook_id, cell_id, version)
	if err != nil || notebook_cell.CellId != cell_id {
		return errors.New("No such cell")
	}

	// Switch the cell into not calculating - this will force all
	// workers to exit.
	notebook_cell.Calculating = false
	notebook_cell.Error = "Cancelled!"
	// Make sure we write the cancel message ASAP
	err = self.Store.SetNotebookCell(notebook_id, notebook_cell)
	if err != nil {
		return err
	}

	// Notify the calculator immediately if we are in the same
	// process. This makes it more responsive.
	notifier, err := services.GetNotifier(self.config_obj)
	if err != nil {
		return err
	}
	return notifier.NotifyListener(
		ctx, self.config_obj, cell_id+version, "CancelNotebookCell")
}

func (self *NotebookManager) UploadNotebookAttachment(
	ctx context.Context, in *api_proto.NotebookFileUploadRequest) (
	*api_proto.NotebookFileUploadResponse, error) {
	decoded, err := base64.StdEncoding.DecodeString(in.Data)
	if err != nil {
		return nil, err
	}

	err = verifyNotebookId(in.NotebookId)
	if err != nil {
		return nil, err
	}

	filename := in.Filename
	if !in.DisableAttachmentId {
		filename = NewNotebookAttachmentId() + "-" + in.Filename
	}

	full_path, err := self.AttachmentManager.StoreAttachment(
		in.NotebookId, filename, decoded)
	if err != nil {
		return nil, err
	}

	frontend_service, err := services.GetFrontendManager(self.config_obj)
	if err != nil {
		return nil, err
	}

	public_url, err := frontend_service.GetBaseURL(self.config_obj)
	if err != nil {
		return nil, err
	}

	// Calculate the URL to the resource
	public_url.Path = path.Join(public_url.Path, full_path.AsClientPath())
	values := public_url.Query()
	values.Set("org_id", utils.NormalizedOrgId(self.config_obj.OrgId))
	public_url.RawQuery = values.Encode()

	result := &api_proto.NotebookFileUploadResponse{
		Url:      public_url.String(),
		Filename: filename,
	}

	result.MimeType = utils.GetMimeString(decoded, utils.AutoDetectMime(true))

	return result, nil
}

func NewNotebookManager(
	config_obj *config_proto.Config,
	Store NotebookStore,
	SuperTimelineStorer timelines.ISuperTimelineStorer,
	SuperTimelineReaderFactory timelines.ISuperTimelineReader,
	SuperTimelineWriterFactory timelines.ISuperTimelineWriter,
	SuperTimelineAnnotator timelines.ISuperTimelineAnnotator,
	AttachmentManager AttachmentManager,
) *NotebookManager {
	result := &NotebookManager{
		config_obj:                 config_obj,
		Store:                      Store,
		SuperTimelineStorer:        SuperTimelineStorer,
		SuperTimelineReaderFactory: SuperTimelineReaderFactory,
		SuperTimelineWriterFactory: SuperTimelineWriterFactory,
		SuperTimelineAnnotator:     SuperTimelineAnnotator,
		AttachmentManager:          AttachmentManager,
	}
	return result
}

func NewNotebookManagerService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.NotebookManager, error) {

	timeline_storer := NewTimelineStorer(config_obj)
	store, err := NewNotebookStore(ctx, wg, config_obj, timeline_storer)
	if err != nil {
		return nil, err
	}

	annotator := NewSuperTimelineAnnotatorImpl(config_obj, timeline_storer,
		&timelines.SuperTimelineReader{},
		&timelines.SuperTimelineWriter{})

	notebook_service := NewNotebookManager(config_obj, store,
		timeline_storer,
		&timelines.SuperTimelineReader{},
		&timelines.SuperTimelineWriter{},
		annotator,
		NewAttachmentManager(config_obj, store),
	)

	// Global Notebooks can be backed up.
	backup_service, err := services.GetBackupService(config_obj)
	if err == nil {
		backup_service.Register(&NotebookBackupProvider{
			notebook_manager: notebook_service,
			config_obj:       config_obj,
		})
	}

	return notebook_service, notebook_service.Start(ctx, config_obj, wg)
}

func (self *NotebookManager) ReformatVQL(
	ctx context.Context, vql string) (string, error) {

	scope := vql_subsystem.MakeScope()
	reformatted, err := reformat.ReFormatVQL(scope, vql, vfilter.DefaultFormatOptions)
	if err != nil {
		return "", err
	}

	return reformatted, nil
}

func verifyNotebookId(notebook_id string) error {
	if !strings.HasPrefix(notebook_id, "N.") {
		return invalidNotebookId
	}
	return nil
}
