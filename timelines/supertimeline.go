package timelines

import (
	"context"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
)

// A Supertimeline is a collection of individual timelines
type SuperTimelinePathManager struct {
	Name string
}

func (self *SuperTimelinePathManager) Path() string {
	return constants.TIMELINE_URN + self.Name
}

func (self *SuperTimelinePathManager) NewChild() *TimelinePathManager {
	return &TimelinePathManager{
		Name:  NewTimelineId(),
		Super: self.Name,
	}
}

func (self *SuperTimelinePathManager) GetChild(name string) *TimelinePathManager {
	return &TimelinePathManager{
		Name:  name,
		Super: self.Name,
	}
}

type SuperTimelineReader struct {
	*timelines_proto.SuperTimeline

	readers []*TimelineReader
}

func (self *SuperTimelineReader) Close() {
	for _, reader := range self.readers {
		reader.Close()
	}
}

func (self *SuperTimelineReader) SeekToTime(timestamp time.Time) {
	for _, reader := range self.readers {
		reader.SeekToTime(timestamp)
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
		if smallest == nil || slots[idx].Time < smallest.Time {
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

func NewSuperTimelineReader(
	config_obj *config_proto.Config,
	path_manager *SuperTimelinePathManager) (*SuperTimelineReader, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	result := &SuperTimelineReader{SuperTimeline: &timelines_proto.SuperTimeline{}}
	err = db.GetSubject(config_obj, path_manager.Path(), result.SuperTimeline)
	if err != nil {
		return nil, err
	}

	// Open all the readers.
	for _, name := range result.Timelines {
		reader, err := NewTimelineReader(config_obj, path_manager.GetChild(name))
		if err != nil {
			result.Close()
			return nil, err
		}
		result.readers = append(result.readers, reader)
	}

	return result, nil
}

type SuperTimelineWriter struct {
	*timelines_proto.SuperTimeline
	config_obj   *config_proto.Config
	path_manager *SuperTimelinePathManager
}

func (self *SuperTimelineWriter) Close() {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return
	}
	db.SetSubject(self.config_obj, self.path_manager.Path(), self.SuperTimeline)
}

func (self *SuperTimelineWriter) AddChild() (*TimelineWriter, error) {
	new_timeline_path_manager := self.path_manager.NewChild()
	writer, err := NewTimelineWriter(self.config_obj, new_timeline_path_manager)
	self.Timelines = append(self.Timelines, new_timeline_path_manager.Name)
	return writer, err
}

func NewSuperTimelineWriter(
	config_obj *config_proto.Config,
	name string) (*SuperTimelineWriter, error) {

	self := &SuperTimelineWriter{
		SuperTimeline: &timelines_proto.SuperTimeline{},
		config_obj:    config_obj,
		path_manager:  &SuperTimelinePathManager{name},
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	err = db.GetSubject(config_obj, self.path_manager.Path(), self.SuperTimeline)
	if err != nil {
		self.SuperTimeline.Name = name
	}

	return self, nil
}
