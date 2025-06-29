package notebook

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/timelines"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/sorter"
	"www.velocidex.com/golang/vfilter"
)

const (
	// Annotation fields are hidden by default.
	AnnotationID     = "_AnnotationID"
	AnnotatedBy      = "_AnnotatedBy"
	AnnotatedAt      = "_AnnotatedAt"
	AnnotationOGTime = "_OriginalTime"
)

var (
	epoch = time.Unix(0, 0)
)

func (self *NotebookManager) Timelines(ctx context.Context,
	notebook_id string) ([]*timelines_proto.SuperTimeline, error) {

	return self.SuperTimelineStorer.List(ctx, notebook_id)
}

func (self *NotebookManager) ReadTimeline(ctx context.Context, notebook_id string,
	supertimeline string, options services.TimelineOptions) (
	services.TimelineReader, error) {

	// Make sure the timeline exists in the notebook.
	notebook_metadata, err := self.Store.GetNotebook(notebook_id)
	if err != nil {
		return nil, err
	}

	if !utils.InString(notebook_metadata.Timelines, supertimeline) {
		notebook_metadata.Timelines = append(notebook_metadata.Timelines,
			supertimeline)
		err := self.Store.SetNotebook(notebook_metadata)
		if err != nil {
			return nil, err
		}
	}

	reader, err := self.SuperTimelineReaderFactory.New(ctx,
		self.config_obj, self.SuperTimelineStorer,
		notebook_id, supertimeline, options.IncludeComponents,
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

	return NewTimelineReader(reader, filter), nil
}

func (self *NotebookManager) AddTimeline(
	ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string,
	timeline *timelines_proto.Timeline,
	in <-chan vfilter.Row) (*timelines_proto.SuperTimeline, error) {

	super, err := self.SuperTimelineWriterFactory.New(ctx,
		self.config_obj, self.SuperTimelineStorer, notebook_id, supertimeline)
	if err != nil {
		return nil, err
	}
	defer super.Close(ctx)

	// make a new timeline to store in the super timeline.
	var writer timelines.ITimelineWriter

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
	sorter := sorter.MergeSorter{ChunkSize: 10000}
	sorted_chan := sorter.Sort(sub_ctx, subscope, in, key, false /* desc */)

	for row := range sorted_chan {
		key, pres := scope.Associative(row, key)
		if !pres {
			continue
		}

		if !utils.IsNil(key) {
			ts, err := functions.TimeFromAny(ctx, scope, key)
			if err == nil {
				err := writer.Write(ts, vfilter.RowToDict(sub_ctx, subscope, row))
				if err != nil {
					return nil, err
				}
			}
			if timeline.StartTime == 0 {
				timeline.StartTime = ts.UnixNano()
			}
			timeline.EndTime = ts.UnixNano()
		}
	}

	return self.SuperTimelineStorer.UpdateTimeline(
		ctx, notebook_id, supertimeline, timeline)
}

func (self *NotebookManager) AnnotateTimeline(ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string,
	message, principal string,
	timestamp time.Time, event *ordereddict.Dict) error {
	return self.SuperTimelineAnnotator.AnnotateTimeline(
		ctx, scope, notebook_id, supertimeline, message, principal, timestamp, event)
}

func (self *NotebookManager) DeleteTimeline(ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline, component string) error {
	return self.SuperTimelineStorer.DeleteComponent(
		ctx, notebook_id, supertimeline, component)
}

type TimelineReader struct {
	timelines.ISuperTimelineReader

	filter *TimelineFilter
}

func NewTimelineReader(reader timelines.ISuperTimelineReader,
	filter *TimelineFilter) *TimelineReader {
	return &TimelineReader{
		ISuperTimelineReader: reader,
		filter:               filter,
	}
}

func (self *TimelineReader) Read(ctx context.Context) <-chan *ordereddict.Dict {
	output_chan := make(chan *ordereddict.Dict)

	go func() {
		defer close(output_chan)
		defer self.Close()

		for event := range self.ISuperTimelineReader.Read(ctx) {
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
