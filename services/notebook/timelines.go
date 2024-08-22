package notebook

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/timelines"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/sorter"
	"www.velocidex.com/golang/vfilter"
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
	return db.DeleteSubject(self.config_obj, timeline_path.Path())
}
