package notebook

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/timelines"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/sorter"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

const (
	// Annotation fields are hidden by default.
	AnnotationID = "_AnnotationID"
	AnnotatedBy  = "_AnnotatedBy"
	AnnotatedAt  = "_AnnotatedAt"
)

var (
	epoch = time.Unix(0, 0)
)

func (self *NotebookManager) Timelines(ctx context.Context,
	notebook_id string) ([]*timelines_proto.SuperTimeline, error) {
	return self.Store.Timelines(ctx, notebook_id)
}

func (self *NotebookManager) ReadTimeline(ctx context.Context, notebook_id string,
	timeline string, options services.TimelineOptions) (
	services.TimelineReader, error) {
	return self.Store.ReadTimeline(ctx, notebook_id, timeline, options)
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
	notebook_id string, supertimeline, component string) error {
	return self.Store.DeleteTimeline(
		ctx, scope, notebook_id, supertimeline, component)
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
	supertimeline string, options services.TimelineOptions) (
	services.TimelineReader, error) {

	// Make sure the timeline exists in the notebook.
	notebook_metadata, err := self.GetNotebook(notebook_id)
	if err != nil {
		return nil, err
	}

	if !utils.InString(notebook_metadata.Timelines, supertimeline) {
		notebook_metadata.Timelines = append(notebook_metadata.Timelines,
			supertimeline)
		err := self.SetNotebook(notebook_metadata)
		if err != nil {
			return nil, err
		}
	}

	super_path_manager := paths.NewNotebookPathManager(notebook_id).
		SuperTimeline(supertimeline)

	reader, err := timelines.NewSuperTimelineReader(self.config_obj,
		super_path_manager, options.IncludeComponents,
		options.ExcludeComponents)
	if err != nil {
		return nil, err
	}

	if !options.StartTime.IsZero() {
		reader.SeekToTime(options.StartTime)
	}

	// Filter the rows based on the user's options.
	filter, err := NewTimelineFilter(options)
	if err != nil {
		return nil, err
	}

	return &TimelineReader{
		SuperTimelineReader: reader,
		filter:              filter,
	}, nil
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

	timeline.StartTime = 0

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
			if timeline.StartTime == 0 {
				timeline.StartTime = ts.UnixNano()
			}
			timeline.EndTime = ts.UnixNano()
		}
	}

	return self.UpdateTimeline(ctx, notebook_id, supertimeline, timeline)
}

func (self *NotebookStoreImpl) DeleteTimeline(
	ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline, del_component string) error {

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

	var new_timelines []*timelines_proto.Timeline
	for _, component := range timeline.Timelines {
		if del_component != "" && del_component != component.Id {
			new_timelines = append(new_timelines, component)
			continue
		}

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

	if len(new_timelines) != 0 {
		timeline.Timelines = new_timelines
		return db.SetSubject(
			self.config_obj, timeline_path.Path(), timeline)
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
			TimestampColumn: constants.TIMELINE_DEFAULT_KEY,
			MessageColumn:   constants.TIMELINE_DEFAULT_MESSAGE,
		}, in)
	return err
}

// Add or update the super timeline record in the data store.
func (self *NotebookStoreImpl) UpdateTimeline(ctx context.Context,
	notebook_id string, supertimeline string,
	timeline *timelines_proto.Timeline) (*timelines_proto.SuperTimeline, error) {

	timeline_path := paths.NewNotebookPathManager(notebook_id).
		SuperTimeline(supertimeline)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	super_timeline := &timelines_proto.SuperTimeline{}
	err = db.GetSubject(self.config_obj, timeline_path.Path(), super_timeline)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	super_timeline.Name = supertimeline

	// Find the existing timeline or add a new one.
	var existing_timeline *timelines_proto.Timeline
	for _, component := range super_timeline.Timelines {
		if component.Id == timeline.Id {
			existing_timeline = component
			break
		}
	}

	if existing_timeline == nil {
		existing_timeline = timeline
		super_timeline.Timelines = append(super_timeline.Timelines, timeline)
	} else {
		// Make a copy
		*existing_timeline = *timeline
	}

	// Now delete the actual record.
	return super_timeline, db.SetSubject(
		self.config_obj, timeline_path.Path(), super_timeline)
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

	// The GUID is used to allow replacing items in the timeline:

	// 1. When an item is **added** to the timeline, the GUID is empty
	//    and gets assigned a new value.

	// 2. When updating the item, the GUID is propagated, and we
	//    filter the second duplicate of the same GUID. This allows us to
	//    replace the GUID.

	// 3. When deleting the item, the timestamp is set to 0, while the
	//    GUID is propagated. We never write items with timestamp of 0.
	guid, pres := event.GetString(AnnotationID)
	if !pres {
		guid = GetGUID()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer writer.Close()

		// The Annotation timeline is always the same.
		timeline := &timelines_proto.Timeline{
			Id:              constants.TIMELINE_ANNOTATION,
			TimestampColumn: constants.TIMELINE_DEFAULT_KEY,
			MessageColumn:   constants.TIMELINE_DEFAULT_MESSAGE,
		}
		defer self.UpdateTimeline(ctx, notebook_id, supertimeline, timeline)

		// Timelines have to be sorted, so we force them to be sorted
		// by the key.
		sorter := sorter.MergeSorter{10000}
		sorted_chan := sorter.Sort(ctx, scope, in,
			constants.TIMELINE_DEFAULT_KEY, false /* desc */)

		for row := range sorted_chan {
			key, pres := scope.Associative(row, constants.TIMELINE_DEFAULT_KEY)
			if !pres {
				continue
			}

			if !utils.IsNil(key) {
				ts, err := functions.TimeFromAny(ctx, scope, key)
				if err == nil {
					writer.Write(ts, vfilter.RowToDict(ctx, scope, row))
				}

				if timeline.StartTime == 0 {
					timeline.StartTime = ts.Unix()
				}
				timeline.EndTime = ts.Unix()
			}
		}
	}()

	// If the event does not already have a message we replace it with
	// the annotation message.
	_, pres = event.GetString("Message")
	if !pres {
		event.Set("Message", message)
	}

	// Add the annotation event only if the time is valid.
	if timestamp.After(epoch) {
		row := event.Update(constants.TIMELINE_DEFAULT_KEY, timestamp).
			Set("Notes", message).
			Set(AnnotatedBy, principal).
			Set(AnnotatedAt, utils.GetTime().Now()).
			Set(AnnotationID, guid)

		// Push it into the sorter
		in <- row
	}

	seen := make(map[string]bool)
	seen[guid] = true

	// Now read all the current events and replay them in order to
	// sort.  NOTE: This is not very efficient way but we dont expect
	// too many annotations so for now this is good enough.
	reader, err := self.ReadTimeline(ctx, notebook_id, supertimeline,
		services.TimelineOptions{
			IncludeComponents: []string{constants.TIMELINE_ANNOTATION},
		})
	if err != nil {
		return err
	}

	for row := range reader.Read(ctx) {
		guid, pres := row.GetString(AnnotationID)
		if pres {
			// Filter out seen GUIDs to allow replacing old items with
			// new items.
			_, pres = seen[guid]
			if pres {
				continue
			}
			seen[guid] = true
		}

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

type TimelineReader struct {
	*timelines.SuperTimelineReader

	filter *TimelineFilter
}

func (self *TimelineReader) Read(ctx context.Context) <-chan *ordereddict.Dict {
	output_chan := make(chan *ordereddict.Dict)

	go func() {
		defer close(output_chan)
		defer self.Close()

		for event := range self.SuperTimelineReader.Read(ctx) {
			if event.Row == nil {
				continue
			}

			if self.filter.ShouldFilter(&event) {
				continue
			}

			// Enforce a column order on the result.
			row := ordereddict.NewDict().
				Set(constants.TIMELINE_DEFAULT_KEY, event.Time).
				Set("Message", event.Message).
				Set("Description", event.TimestampDescription)

			for _, k := range event.Row.Keys() {
				switch k {
				case constants.TIMELINE_DEFAULT_KEY, "Message", "Description":
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

	return output_chan

}

func GetGUID() string {
	buff := make([]byte, 8)
	binary.LittleEndian.PutUint64(buff, uint64(utils.GetGUID()))
	return base64.StdEncoding.EncodeToString(buff)
}
