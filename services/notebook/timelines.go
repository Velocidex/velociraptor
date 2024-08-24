package notebook

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/timelines"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/sorter"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

func (self *NotebookManager) Timelines(ctx context.Context,
	notebook_id string) ([]*timelines_proto.SuperTimeline, error) {
	return self.Store.Timelines(ctx, notebook_id)
}

func (self *NotebookManager) ReadTimeline(ctx context.Context, notebook_id string,
	timeline string, start time.Time,
	include_components, exclude_components []string) (
	<-chan *ordereddict.Dict, error) {
	return self.Store.ReadTimeline(ctx, notebook_id, timeline, start,
		include_components, exclude_components)
}

func (self *NotebookManager) AddTimeline(
	ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string,
	timeline *timelines_proto.Timeline,
	in <-chan vfilter.Row) (*timelines_proto.SuperTimeline, error) {
	return self.Store.AddTimeline(
		ctx, scope, notebook_id, supertimeline, timeline, in)
}

func (self *NotebookManager) AnnotateTimeline(ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string,
	message, principal string,
	timestamp time.Time, event *ordereddict.Dict) error {
	return self.Store.AnnotateTimeline(
		ctx, scope, notebook_id, supertimeline, message, principal, timestamp, event)
}

func (self *NotebookManager) DeleteTimeline(ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string) error {
	return self.Store.DeleteTimeline(ctx, scope, notebook_id, supertimeline)
}

func (self *NotebookStoreImpl) Timelines(ctx context.Context,
	notebook_id string) ([]*timelines_proto.SuperTimeline, error) {
	dir := paths.NewNotebookPathManager(notebook_id).SuperTimelineDir()
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	urns, err := db.ListChildren(self.config_obj, dir)
	if err != nil {
		return nil, err
	}

	result := make([]*timelines_proto.SuperTimeline, 0, len(urns))
	for _, urn := range urns {
		timeline := &timelines_proto.SuperTimeline{}
		err = db.GetSubject(self.config_obj, urn, timeline)
		if err == nil {
			result = append(result, timeline)
		}

	}
	return result, nil
}

func (self *NotebookStoreImpl) ReadTimeline(ctx context.Context, notebook_id string,
	timeline string, start_time time.Time,
	include_components, exclude_components []string) (
	<-chan *ordereddict.Dict, error) {

	// Make sure the timeline exists in the notebook.
	notebook_metadata, err := self.GetNotebook(notebook_id)
	if err != nil {
		return nil, err
	}

	if !utils.InString(notebook_metadata.Timelines, timeline) {
		notebook_metadata.Timelines = append(notebook_metadata.Timelines,
			timeline)
		err := self.SetNotebook(notebook_metadata)
		if err != nil {
			return nil, err
		}
	}

	super_path_manager := paths.NewNotebookPathManager(notebook_id).
		SuperTimeline(timeline)

	reader, err := timelines.NewSuperTimelineReader(self.config_obj,
		super_path_manager, include_components, exclude_components)
	if err != nil {
		return nil, err
	}

	if !start_time.IsZero() {
		reader.SeekToTime(start_time)
	}

	output_chan := make(chan *ordereddict.Dict)
	go func() {
		defer close(output_chan)
		defer reader.Close()

		for event := range reader.Read(ctx) {
			if event.Row == nil {
				continue
			}

			// Enforce a column order on the result.
			row := ordereddict.NewDict().
				Set("Timestamp", event.Time).
				Set("Message", event.Message).
				Set("Description", event.TimestampDescription)

			for _, k := range event.Row.Keys() {
				switch k {
				case "Timestamp", "TimestampDesc", "Message", "Description":
				default:
					v, _ := event.Row.Get(k)
					row.Set(k, v)
				}
			}

			row.Set("_Source", event.Source)

			select {
			case <-ctx.Done():
				return

			case output_chan <- row:
			}
		}
	}()

	return output_chan, nil
}

func (self *NotebookStoreImpl) AddTimeline(
	ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string,
	timeline *timelines_proto.Timeline,
	in <-chan vfilter.Row) (*timelines_proto.SuperTimeline, error) {
	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)

	super, err := timelines.NewSuperTimelineWriter(
		self.config_obj, notebook_path_manager.SuperTimeline(supertimeline))
	if err != nil {
		return nil, err
	}
	defer super.Close()

	// make a new timeline to store in the super timeline.
	var writer *timelines.TimelineWriter

	writer, err = super.AddChild(timeline, utils.BackgroundWriter)
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	writer.Truncate()

	subscope := scope.Copy()
	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	key := timeline.TimestampColumn
	if key == "" {
		key = "_ts"
	}

	// Timelines have to be sorted, so we force them to be sorted
	// by the key.
	sorter := sorter.MergeSorter{10000}
	sorted_chan := sorter.Sort(sub_ctx, subscope, in, key, false /* desc */)

	for row := range sorted_chan {
		key, pres := scope.Associative(row, key)
		if !pres {
			continue
		}

		if !utils.IsNil(key) {
			ts, err := functions.TimeFromAny(ctx, scope, key)
			if err == nil {
				writer.Write(ts, vfilter.RowToDict(sub_ctx, subscope, row))
			}
		}
	}

	notebook_metadata, err := self.GetNotebook(notebook_id)
	if err != nil {
		return nil, err
	}

	// The notebook should hold a reference to all the supertimelines.
	if !utils.InString(notebook_metadata.Timelines, supertimeline) {
		notebook_metadata.Timelines = append(notebook_metadata.Timelines,
			supertimeline)

		err = self.SetNotebook(notebook_metadata)
		if err != nil {
			return nil, err
		}
	}

	return super.SuperTimeline, nil
}

func (self *NotebookStoreImpl) DeleteTimeline(ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string) error {

	timeline_path := paths.NewNotebookPathManager(notebook_id).
		SuperTimeline(supertimeline)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	timeline := &timelines_proto.SuperTimeline{}
	err = db.GetSubject(self.config_obj, timeline_path.Path(), timeline)
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(self.config_obj)

	for _, component := range timeline.Timelines {
		// Now delete all the filestore files associated with the timeline.
		child := timeline_path.GetChild(component.Id)

		err = file_store_factory.Delete(child.Path())
		if err != nil {
			continue
		}

		err = file_store_factory.Delete(child.Index())
		if err != nil {
			continue
		}
	}

	// Now delete the actual record.
	err = db.DeleteSubject(self.config_obj, timeline_path.Path())
	if err != nil {
		return err
	}

	notebook_metadata, err := self.GetNotebook(notebook_id)
	if err != nil {
		return err
	}

	notebook_metadata.Timelines = utils.FilterSlice(
		notebook_metadata.Timelines, supertimeline)

	return self.SetNotebook(notebook_metadata)
}

func (self *NotebookStoreImpl) ensureAnnotationComponent(
	ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string) error {

	timelines, err := self.Timelines(ctx, notebook_id)
	if err != nil {
		return err
	}

	for _, t := range timelines {
		if t.Name == supertimeline {
			for _, c := range t.Timelines {
				if c.Id == constants.TIMELINE_ANNOTATION {
					return nil
				}
			}
		}
	}

	// If we get here we dont have the Annotation component.
	in := make(chan types.Row)
	close(in)
	_, err = self.AddTimeline(ctx, scope, notebook_id, supertimeline,
		&timelines_proto.Timeline{
			Id:              constants.TIMELINE_ANNOTATION,
			TimestampColumn: "Timestamp",
			MessageColumn:   "Message",
		}, in)
	return err
}

// An annotation is just another timeline that is added to the super
// timeline.
func (self *NotebookStoreImpl) AnnotateTimeline(
	ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string,
	message, principal string,
	timestamp time.Time, event *ordereddict.Dict) (err error) {

	// First ensure the timeline actually has an annotation component.
	err = self.ensureAnnotationComponent(
		ctx, scope, notebook_id, supertimeline)
	if err != nil {
		return err
	}

	time_key := "Timestamp"

	// Re-sort the annotation timeline into a tempfile.
	tmp_path := paths.NewTempPathManager("").Path()
	path_manager := paths.NewTimelinePathManager(
		tmp_path.Base(), tmp_path)

	var wg sync.WaitGroup

	// Write synchronously because we need to see the annotation
	// immediately visible in the GUI so it needs to be done before we
	// return.
	file_store_factory := file_store.GetFileStore(self.config_obj)
	writer, err := timelines.NewTimelineWriter(file_store_factory, path_manager,
		utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return err
	}

	in := make(chan types.Row)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer writer.Close()

		// Timelines have to be sorted, so we force them to be sorted
		// by the key.
		sorter := sorter.MergeSorter{10000}
		sorted_chan := sorter.Sort(ctx, scope, in, time_key, false /* desc */)

		for row := range sorted_chan {
			key, pres := scope.Associative(row, time_key)
			if !pres {
				continue
			}

			if !utils.IsNil(key) {
				ts, err := functions.TimeFromAny(ctx, scope, key)
				if err == nil {
					writer.Write(ts, vfilter.RowToDict(ctx, scope, row))
				}
			}
		}
	}()

	// Add the annotation event
	row := ordereddict.NewDict().
		Set("Timestamp", timestamp).
		Set("Message", message).
		Set("User", principal).
		Set("Annotated At", utils.GetTime().Now()).
		Set("Event", event)

	// Push it into the sorter
	in <- row

	// Now read all the current events and replay them in order to
	// sort.  NOTE: This is not very efficient way but we dont expect
	// too many annotations so for now this is good enough.
	rows, err := self.ReadTimeline(ctx, notebook_id, supertimeline,
		time.Time{}, []string{constants.TIMELINE_ANNOTATION}, nil)
	if err != nil {
		return err
	}

	for row := range rows {
		select {
		case <-ctx.Done():
			return nil
		case in <- row:
		}
	}

	// Done... wait for the sorter to finish
	close(in)
	wg.Wait()

	// Move the temp files over the original.
	super_path_manager := paths.NewNotebookPathManager(notebook_id).
		SuperTimeline(supertimeline)

	// Move the new files on top of the old
	dest := super_path_manager.GetChild(constants.TIMELINE_ANNOTATION)
	err = file_store_factory.Move(tmp_path, dest.Path())
	if err != nil {
		return err
	}

	// Now also move the indexes
	err = file_store_factory.Move(
		tmp_path.SetType(api.PATH_TYPE_FILESTORE_JSON_TIME_INDEX),
		dest.Index())

	return err
}
