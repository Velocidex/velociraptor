package timelines

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

type SuperTimelineReader struct {
	*timelines_proto.SuperTimeline

	readers []ITimelineReader

	reader_factory ITimelineReader
}

func (self *SuperTimelineReader) Stat() *timelines_proto.SuperTimeline {
	result := proto.Clone(self.SuperTimeline).(*timelines_proto.SuperTimeline)
	return result
}

func (self *SuperTimelineReader) Close() {
	for _, reader := range self.readers {
		reader.Close()
	}
}

func (self *SuperTimelineReader) SeekToTime(timestamp time.Time) {
	for _, reader := range self.readers {
		_ = reader.SeekToTime(timestamp)
	}
}

// Gets the smallest item or null if no items available.
func (self *SuperTimelineReader) getSmallest(
	ctx context.Context,
	slots []*TimelineItem, chans []<-chan TimelineItem) *TimelineItem {

	var smallest *TimelineItem
	var smallest_idx int

	for idx := 0; idx < len(slots); idx++ {
		// Backfill slot if needed
		if slots[idx] == nil {
			select {
			case <-ctx.Done():
				return nil

			case item, ok := <-chans[idx]:
				if !ok {
					// Channel is closed, try the
					// next slot.
					continue
				}

				// Store the item in the slot.
				slots[idx] = &item
			}
		}

		// Check if the item is smallest than the result.
		if smallest == nil || slots[idx].Time.Before(smallest.Time) {
			smallest = slots[idx]
			smallest_idx = idx
		}
	}

	// No smallest found
	if smallest == nil {
		return nil
	}

	// Take the smallest and backfill
	slots[smallest_idx] = nil
	return smallest
}

func (self *SuperTimelineReader) Read(ctx context.Context) <-chan TimelineItem {
	output_chan := make(chan TimelineItem)

	go func() {
		defer close(output_chan)

		slots := make([]*TimelineItem, len(self.readers))
		chans := make([]<-chan TimelineItem, len(self.readers))

		// Fill in the initial set
		for idx, reader := range self.readers {
			chans[idx] = reader.Read(ctx)
		}

		for {
			smallest := self.getSmallest(ctx, slots, chans)
			if smallest == nil {
				return
			}

			output_chan <- *smallest
		}
	}()

	return output_chan
}

func (self SuperTimelineReader) New(ctx context.Context,
	config_obj *config_proto.Config,
	storer ISuperTimelineStorer,
	notebook_id, super_timeline string,
	include_components []string,
	exclude_components []string) (ISuperTimelineReader, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	result := &SuperTimelineReader{
		SuperTimeline:  &timelines_proto.SuperTimeline{},
		reader_factory: &TimelineReader{},
	}

	path_manager := paths.NewNotebookPathManager(notebook_id).
		SuperTimeline(super_timeline)

	err = db.GetSubject(config_obj, path_manager.Path(), result.SuperTimeline)
	if err != nil {
		// SuperTimeline does not exist yet, just make an
		// empty one.
		result.SuperTimeline.Name = path_manager.Name
		err = db.SetSubject(config_obj, path_manager.Path(), result)
		if err != nil {
			return nil, err
		}
	}

	// Open all the readers.
	for _, timeline := range result.Timelines {
		if len(include_components) > 0 &&
			!utils.InString(include_components, timeline.Id) {
			continue
		}

		if utils.InString(exclude_components, timeline.Id) {
			continue
		}
		// Transform the timeline event based on the timeline
		// specifications. This allows us to re-define standard fields
		// like timestamp, message and timestamp_description.
		transformer := timelineTransformer{timeline}

		// We are going to use this timeline.
		timeline.Active = true

		reader, err := result.reader_factory.New(
			config_obj, transformer, path_manager.GetChild(timeline.Id))
		if err != nil {
			// We cant read the component - it may not be there, just
			// ignore it.
			continue
		}
		result.readers = append(result.readers, reader)
	}
	return result, nil
}

type SuperTimelineWriter struct {
	// Protected by mutex
	*timelines_proto.SuperTimeline

	mu              sync.Mutex
	config_obj      *config_proto.Config
	notebook_id     string
	timeline_storer ISuperTimelineStorer
}

func (self *SuperTimelineWriter) New(
	ctx context.Context, config_obj *config_proto.Config,
	storer ISuperTimelineStorer,
	notebook_id, name string) (result ISuperTimelineWriter, err error) {

	res := &SuperTimelineWriter{
		config_obj:      config_obj,
		notebook_id:     notebook_id,
		timeline_storer: storer,
	}

	res.SuperTimeline, err = storer.Get(ctx, notebook_id, name)
	if err != nil {

		// If the file does not exist, we create a new empty super
		// timeline inside the notebook.
		if errors.Is(err, os.ErrNotExist) {
			res.SuperTimeline = &timelines_proto.SuperTimeline{
				Name: name,
			}

			err = storer.Set(ctx, notebook_id, res.SuperTimeline)

		} else {
			return nil, err
		}
	}
	return res, err
}

func (self *SuperTimelineWriter) Close(ctx context.Context) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.timeline_storer.Set(ctx, self.notebook_id, self.SuperTimeline)
}

func (self *SuperTimelineWriter) AddChild(
	timeline *timelines_proto.Timeline, completer func()) (ITimelineWriter, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if timeline.Id == "" {
		return nil, errors.New("SuperTimelineWriter: Must specify a component name")
	}

	path_manager := paths.NewNotebookPathManager(self.notebook_id).
		SuperTimeline(self.SuperTimeline.Name)
	new_timeline_path_manager := path_manager.GetChild(timeline.Id)

	var writer *TimelineWriter
	var err error

	writer, err = NewTimelineWriter(
		self.config_obj,
		new_timeline_path_manager,

		// When we are complete we update the timeline stats
		func() {
			self.mu.Lock()
			defer self.mu.Unlock()

			// The writer.Close() will wait for this completion
			// function to return.
			defer writer.wg.Done()

			if completer != nil {
				defer completer()
			}

			stats := writer.Stats()

			// Only add a new child if it is not already in there.
			for _, item := range self.Timelines {
				if item.Id == timeline.Id {
					item.StartTime = stats.StartTime
					item.EndTime = stats.EndTime
					item.TimestampColumn = timeline.TimestampColumn
					item.MessageColumn = timeline.MessageColumn
					item.TimestampDescriptionColumn = timeline.TimestampDescriptionColumn
					return
				}
			}

			item := &timelines_proto.Timeline{
				Id:                         timeline.Id,
				StartTime:                  stats.StartTime,
				EndTime:                    stats.EndTime,
				TimestampColumn:            timeline.TimestampColumn,
				MessageColumn:              timeline.MessageColumn,
				TimestampDescriptionColumn: timeline.TimestampDescriptionColumn,
			}
			self.Timelines = append(self.Timelines, item)
		},
		result_sets.TruncateMode)
	if err != nil {
		return nil, err
	}

	// For the completer to run before closing.
	writer.wg.Add(1)

	return writer, err
}
