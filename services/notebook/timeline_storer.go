package notebook

import (
	"context"
	"errors"
	"os"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

type TimelineStorer struct {
	config_obj *config_proto.Config
}

func NewTimelineStorer(config_obj *config_proto.Config) *TimelineStorer {
	return &TimelineStorer{
		config_obj: config_obj,
	}
}

func (self *TimelineStorer) Set(
	ctx context.Context, notebook_id string,
	timeline *timelines_proto.SuperTimeline) error {

	timeline_path := paths.NewNotebookPathManager(notebook_id).
		SuperTimeline(timeline.Name)

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	return db.SetSubject(
		self.config_obj, timeline_path.Path(), timeline)
}

func (self *TimelineStorer) Get(
	ctx context.Context, notebook_id string,
	supertimeline string) (*timelines_proto.SuperTimeline, error) {

	timeline_path := paths.NewNotebookPathManager(notebook_id).
		SuperTimeline(supertimeline)

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	timeline := &timelines_proto.SuperTimeline{}
	err = db.GetSubject(self.config_obj, timeline_path.Path(), timeline)
	if err != nil {
		return nil, err
	}

	return timeline, nil
}

func (self *TimelineStorer) List(ctx context.Context,
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

func (self *TimelineStorer) Delete(ctx context.Context,
	notebook_id string, super_timeline string) error {
	filename := paths.NewNotebookPathManager(notebook_id).
		SuperTimeline(super_timeline).Path()
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	return db.DeleteSubject(self.config_obj, filename)
}

func (self *TimelineStorer) DeleteComponent(ctx context.Context,
	notebook_id string, super_timeline, del_component string) error {

	timeline, err := self.Get(ctx, notebook_id, super_timeline)
	if err != nil {
		return err
	}

	timeline_path := paths.NewNotebookPathManager(notebook_id).
		SuperTimeline(super_timeline)

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

	timeline.Timelines = new_timelines
	return self.Set(ctx, notebook_id, timeline)
}

func (self *TimelineStorer) GetTimeline(ctx context.Context, notebook_id string,
	super_timeline, component string) (*timelines_proto.Timeline, error) {
	supertimeline, err := self.Get(ctx, notebook_id, super_timeline)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		supertimeline = &timelines_proto.SuperTimeline{}
	}

	for _, t := range supertimeline.Timelines {
		if t.Id == component {
			return t, nil
		}
	}
	return nil, utils.Wrap(utils.NotFoundError, component)
}

// Add or update the super timeline record in the data store.
func (self *TimelineStorer) UpdateTimeline(ctx context.Context,
	notebook_id string, supertimeline string,
	timeline *timelines_proto.Timeline) (*timelines_proto.SuperTimeline, error) {

	super_timeline, err := self.Get(ctx, notebook_id, supertimeline)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		super_timeline = &timelines_proto.SuperTimeline{}
	}
	super_timeline.Name = supertimeline

	// Find the existing timeline or add a new one.
	var found bool
	for idx, component := range super_timeline.Timelines {
		if component.Id == timeline.Id {
			super_timeline.Timelines[idx] = proto.Clone(timeline).(*timelines_proto.Timeline)
			found = true
			break
		}
	}

	if !found {
		super_timeline.Timelines = append(super_timeline.Timelines,
			proto.Clone(timeline).(*timelines_proto.Timeline))
	}

	// Now delete the actual record.
	return super_timeline, self.Set(ctx, notebook_id, super_timeline)
}

func (self *TimelineStorer) GetAvailableTimelines(
	ctx context.Context, notebook_id string) []string {
	path_manager := paths.NewNotebookPathManager(notebook_id)
	result := []string{}
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil
	}

	files, err := db.ListChildren(self.config_obj, path_manager.SuperTimelineDir())
	if err != nil {
		return nil
	}

	for _, f := range files {
		if !f.IsDir() {
			result = append(result, f.Base())
		}
	}
	return result
}
