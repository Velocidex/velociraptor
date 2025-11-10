package notebook

import (
	"context"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Compose the notebook store from various components.
type NotebookStoreImpl struct {
	config_obj *config_proto.Config

	// Keep an in memory cache of all global notebooks.
	mu               sync.Mutex
	global_notebooks map[string]*api_proto.NotebookMetadata

	// Keep the last time for a notebook deletion to ensure we update
	// the version when a notebook is deleted.
	last_deleted int64

	SuperTimelineStorer timelines.ISuperTimelineStorer
}

func MakeNotebookStore(
	config_obj *config_proto.Config,
	SuperTimelineStorer timelines.ISuperTimelineStorer) *NotebookStoreImpl {
	return &NotebookStoreImpl{
		config_obj:          config_obj,
		SuperTimelineStorer: SuperTimelineStorer,
	}
}

func NewNotebookStore(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	SuperTimelineStorer timelines.ISuperTimelineStorer) (*NotebookStoreImpl, error) {

	result := MakeNotebookStore(config_obj, SuperTimelineStorer)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if DO_NOT_SYNC_NOTEBOOKS_FOR_TEST.Load() {
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-utils.GetTime().After(utils.Jitter(time.Minute)):
				err := result.syncAllNotebooks()
				if err != nil {
					logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
					logger.Error("<red>syncAllNotebooks</> %v", err)
				}
			}
		}
	}()

	return result, result.syncAllNotebooks()
}

func (self *NotebookStoreImpl) Version() (res int64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	res = self.last_deleted

	for _, v := range self.global_notebooks {
		if v.ModifiedTime > res {
			res = v.ModifiedTime
		}
	}
	return res
}

func (self *NotebookStoreImpl) SetNotebook(in *api_proto.NotebookMetadata) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._SetNotebook(in)
}

func (self *NotebookStoreImpl) _SetNotebook(in *api_proto.NotebookMetadata) error {
	if utils.IsGlobalNotebooks(in.NotebookId) {
		self.global_notebooks[in.NotebookId] = in
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	notebook_path_manager := paths.NewNotebookPathManager(in.NotebookId)

	// Ensure the notebook reflects the last time it was set.
	in.ModifiedTime = utils.GetTime().Now().Unix()
	return db.SetSubject(self.config_obj, notebook_path_manager.Path(), in)
}

func (self *NotebookStoreImpl) GetNotebook(notebook_id string) (*api_proto.NotebookMetadata, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._GetNotebook(notebook_id)
}

func (self *NotebookStoreImpl) _GetNotebook(notebook_id string) (*api_proto.NotebookMetadata, error) {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	notebook := &api_proto.NotebookMetadata{}
	err = db.GetSubject(self.config_obj, notebook_path_manager.Path(),
		notebook)

	// Deduplicate cells
	cell_metadata := ordereddict.NewDict()
	for _, cell := range notebook.CellMetadata {
		_, pres := cell_metadata.Get(cell.CellId)
		if pres {
			continue
		}

		cell_metadata.Set(cell.CellId, cell)
	}

	notebook.CellMetadata = nil
	for _, v := range cell_metadata.Values() {
		notebook.CellMetadata = append(notebook.CellMetadata,
			v.(*api_proto.NotebookCell))
	}

	return notebook, err
}

// Update a notebook cell atomically.
func (self *NotebookStoreImpl) SetNotebookCell(
	notebook_id string, in *api_proto.NotebookCell) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	in.NotebookId = notebook_id

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	notebook_path_manager := paths.NewNotebookPathManager(
		notebook_id).Cell(in.CellId, in.CurrentVersion)
	err = db.SetSubject(self.config_obj, notebook_path_manager.Path(), in)
	if err != nil {
		return err
	}

	// Open the notebook and update the cell's timestamp.
	notebook, err := self._GetNotebook(notebook_id)
	if err != nil {
		return err
	}

	now := utils.GetTime().Now().UnixNano()

	// Update the cell's timestamp so the gui will refresh it.
	new_cell_md := []*api_proto.NotebookCell{}
	found := false
	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == in.CellId {
			// Replace the cell with the new cell
			cell_md = SummarizeCell(in)
			cell_md.Timestamp = now
			found = true
		}
		new_cell_md = append(new_cell_md, cell_md)
	}

	if !found {
		cell_md := proto.Clone(in).(*api_proto.NotebookCell)
		cell_md.Timestamp = now
		new_cell_md = append(new_cell_md, cell_md)
	}

	notebook.CellMetadata = new_cell_md
	return self._SetNotebook(notebook)
}

func (self *NotebookStoreImpl) RemoveNotebookCell(
	ctx context.Context, config_obj *config_proto.Config,
	notebook_id, cell_id, version string, output_chan chan *ordereddict.Dict) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	notebook_path_manager := paths.NewNotebookPathManager(
		notebook_id).Cell(cell_id, version)

	// Indiscriminately delete all the client's datastore files.
	err = datastore.Walk(config_obj, db, notebook_path_manager.DSDirectory(),
		datastore.WalkWithoutDirectories,
		func(filename api.DSPathSpec) error {
			if output_chan != nil {
				select {
				case <-ctx.Done():
					return nil

				case output_chan <- ordereddict.NewDict().
					Set("notebook_id", notebook_id).
					Set("type", "Notebook").
					Set("vfs_path", filename):
				}
			}

			return db.DeleteSubject(config_obj, filename)
		})
	if err != nil {
		return err
	}

	// Remove the empty directories
	_ = datastore.Walk(config_obj, db, notebook_path_manager.DSDirectory(),
		datastore.WalkWithDirectories,
		func(filename api.DSPathSpec) error {
			// Ignore errors so we can try to delete as much as possible
			_ = db.DeleteSubject(config_obj, filename)
			return nil
		})

	// Delete the filestore files.
	file_store_factory := file_store.GetFileStore(config_obj)
	_ = api.Walk(file_store_factory, notebook_path_manager.Directory(),
		func(filename api.FSPathSpec, info os.FileInfo) error {
			if output_chan != nil {
				select {
				case <-ctx.Done():
					return nil

				case output_chan <- ordereddict.NewDict().
					Set("notebook_id", notebook_id).
					Set("type", "Filestore").
					Set("vfs_path", filename):
				}
			}
			return file_store_factory.Delete(filename)
		})
	if err != nil {
		return err
	}

	// Open the notebook and remove the cell
	notebook, err := self._GetNotebook(notebook_id)
	if err != nil {
		return err
	}

	now := utils.GetTime().Now().UnixNano()

	// Update the cell's timestamp so the gui will refresh it.
	new_cell_md := []*api_proto.NotebookCell{}
	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == cell_id {
			new_versions := make([]string, 0, len(cell_md.AvailableVersions))
			for _, cell_version := range cell_md.AvailableVersions {
				if cell_version != version {
					new_versions = append(new_versions, cell_version)
				}
			}
			cell_md.AvailableVersions = new_versions
			cell_md.Timestamp = now
		}
		new_cell_md = append(new_cell_md, cell_md)
	}

	notebook.CellMetadata = new_cell_md
	return self._SetNotebook(notebook)
}

func (self *NotebookStoreImpl) GetNotebookCell(
	notebook_id, cell_id, version string) (*api_proto.NotebookCell, error) {

	// If the caller does not specify the version it means they want
	// the current version.
	if version == "" {
		notebook, err := self.GetNotebook(notebook_id)
		if err != nil {
			return nil, err
		}

		for _, cell_md := range notebook.CellMetadata {
			if cell_md.CellId == cell_id {
				version = cell_md.CurrentVersion
				break
			}
		}
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id).
		Cell(cell_id, version)
	notebook_cell := &api_proto.NotebookCell{}
	err = db.GetSubject(self.config_obj, notebook_path_manager.Path(),
		notebook_cell)

	// Ensure the cell carries its owner ID.
	notebook_cell.NotebookId = notebook_id

	return notebook_cell, err
}

func (self *NotebookStoreImpl) GetAllNotebooks(
	ctx context.Context, opts services.NotebookSearchOptions) (
	[]*api_proto.NotebookMetadata, error) {

	result := []*api_proto.NotebookMetadata{}
	self.mu.Lock()
	for _, notebook := range self.global_notebooks {
		if notebook.Hidden || notebook.NotebookId == "" {
			continue
		}

		if opts.Username != "" && !checkNotebookAccess(notebook, opts.Username) {
			continue
		}

		// We should check the number of timelines in each notebook.
		super_timelines := self.SuperTimelineStorer.GetAvailableTimelines(
			ctx, notebook.NotebookId)
		if opts.Timelines && len(super_timelines) == 0 {
			continue
		}

		out := proto.Clone(notebook).(*api_proto.NotebookMetadata)
		out.Timelines = super_timelines
		result = append(result, out)
	}
	self.mu.Unlock()

	sort.Slice(result, func(i, j int) bool {
		return result[i].NotebookId < result[j].NotebookId
	})

	return result, nil
}

func (self *NotebookStoreImpl) syncAllNotebooks() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	// List all available notebooks
	notebook_urns, err := db.ListChildren(self.config_obj, paths.NotebookDir())
	if err != nil {
		return err
	}

	requests := make([]*datastore.MultiGetSubjectRequest,
		0, len(notebook_urns))

	for _, urn := range notebook_urns {
		if urn.IsDir() {
			continue
		}
		requests = append(requests,
			datastore.NewMultiGetSubjectRequest(
				&api_proto.NotebookMetadata{}, urn, nil))
	}

	err = datastore.MultiGetSubject(self.config_obj, requests)
	if err != nil {
		return nil
	}

	// Update global notebook cache
	self.global_notebooks = make(map[string]*api_proto.NotebookMetadata)
	for _, res := range requests {
		if res.Error() != nil {
			continue
		}

		notebook := res.Message().(*api_proto.NotebookMetadata)
		if notebook.NotebookId == "" ||
			notebook.Hidden ||
			!utils.IsGlobalNotebooks(notebook.NotebookId) {
			continue
		}
		self.global_notebooks[notebook.NotebookId] = notebook
	}

	return nil
}
