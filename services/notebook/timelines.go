package notebook

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/datastore"
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
	notebook_id string, timeline string, component string,
	key string, in <-chan vfilter.Row) (*timelines_proto.SuperTimeline, error) {
	return self.Store.AddTimeline(
		ctx, scope, notebook_id, timeline, component, key, in)
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

		for item := range reader.Read(ctx) {
			select {
			case <-ctx.Done():
				return

			case output_chan <- item.Row.Set("_ts", item.Time):
			}
		}
	}()

	return output_chan, nil
}

func (self *NotebookStoreImpl) AddTimeline(
	ctx context.Context, scope vfilter.Scope,
	notebook_id string, timeline string, component string, key string,
	in <-chan vfilter.Row) (*timelines_proto.SuperTimeline, error) {
	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)

	super, err := timelines.NewSuperTimelineWriter(
		self.config_obj, notebook_path_manager.SuperTimeline(timeline))
	if err != nil {
		return nil, err
	}
	defer super.Close()

	// make a new timeline to store in the super timeline.
	writer, err := super.AddChild(component)
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	writer.Truncate()

	// Push data into the timeline in the background
	go func() {
		subscope := scope.Copy()
		sub_ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Timelines have to be sorted, so we force them to be sorted
		// by the key.
		sorter := sorter.MergeSorter{10000}
		sorted_chan := sorter.Sort(sub_ctx, subscope, in, key, false /* desc */)

		for row := range sorted_chan {
			key, pres := scope.Associative(row, key)
			if !pres {
				return
			}

			if !utils.IsNil(key) {
				ts, err := functions.TimeFromAny(ctx, scope, key)
				if err == nil {
					writer.Write(ts, vfilter.RowToDict(sub_ctx, subscope, row))
				}
			}
		}
	}()

	notebook_metadata, err := self.GetNotebook(notebook_id)
	if err != nil {
		return nil, err
	}

	notebook_metadata.Timelines = utils.DeduplicateStringSlice(
		append(notebook_metadata.Timelines, timeline))

	err = self.SetNotebook(notebook_metadata)
	if err != nil {
		return nil, err
	}

	return super.SuperTimeline, nil
}
