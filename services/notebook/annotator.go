package notebook

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
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

type SuperTimelineAnnotatorImpl struct {
	config_obj                 *config_proto.Config
	SuperTimelineStorer        timelines.ISuperTimelineStorer
	SuperTimelineReaderFactory timelines.ISuperTimelineReader
	SuperTimelineWriterFactory timelines.ISuperTimelineWriter
}

func NewSuperTimelineAnnotatorImpl(
	config_obj *config_proto.Config,
	SuperTimelineStorer timelines.ISuperTimelineStorer,
	SuperTimelineReaderFactory timelines.ISuperTimelineReader,
	SuperTimelineWriterFactory timelines.ISuperTimelineWriter,
) timelines.ISuperTimelineAnnotator {
	return &SuperTimelineAnnotatorImpl{
		config_obj:                 config_obj,
		SuperTimelineStorer:        SuperTimelineStorer,
		SuperTimelineReaderFactory: SuperTimelineReaderFactory,
		SuperTimelineWriterFactory: SuperTimelineWriterFactory,
	}
}

func (self *SuperTimelineAnnotatorImpl) AnnotateTimeline(
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
	writer, err := timelines.NewTimelineWriter(self.config_obj, path_manager,
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
		defer func() {
			_, err := self.SuperTimelineStorer.UpdateTimeline(
				ctx, notebook_id, supertimeline, timeline)
			if err != nil {
				logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
				logger.Error("<red>SuperTimelineStorer UpdateTimeline</> %v", err)
			}
		}()

		// Timelines have to be sorted, so we force them to be sorted
		// by the key.
		sorter := sorter.MergeSorter{ChunkSize: 10000}
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
					_ = writer.Write(ts, vfilter.RowToDict(ctx, scope, row))
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
	if !timestamp.IsZero() && timestamp.After(epoch) {
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
	super_path_manager := paths.NewNotebookPathManager(notebook_id).
		SuperTimeline(supertimeline)

	// The super timeline reader emits timeline items.
	super_reader, err := self.SuperTimelineReaderFactory.New(ctx,
		self.config_obj, self.SuperTimelineStorer,
		notebook_id, supertimeline,
		[]string{constants.TIMELINE_ANNOTATION}, nil)
	if err != nil {
		return err
	}

	// We need to convert those to a straight dicts.
	reader := &TimelineReader{
		ISuperTimelineReader: super_reader,
		filter:               &TimelineFilter{},
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

	// Move the new files on top of the old
	dest := super_path_manager.GetChild(constants.TIMELINE_ANNOTATION)
	file_store_factory := file_store.GetFileStore(self.config_obj)
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

func (self *SuperTimelineAnnotatorImpl) ensureAnnotationComponent(
	ctx context.Context, scope vfilter.Scope,
	notebook_id string, supertimeline string) error {

	super_timeline, err := self.SuperTimelineStorer.Get(ctx, notebook_id, supertimeline)
	if err != nil {
		// If timeline does not exist, just make it.
		super_timeline = &timelines_proto.SuperTimeline{
			Name: supertimeline,
		}
	}

	for _, c := range super_timeline.Timelines {
		if c.Id == constants.TIMELINE_ANNOTATION {
			// We already have an annotation component
			return nil
		}
	}

	// If we get here we dont have the Annotation component so just
	// add it.
	timeline := &timelines_proto.Timeline{
		Id:              constants.TIMELINE_ANNOTATION,
		TimestampColumn: constants.TIMELINE_DEFAULT_KEY,
		MessageColumn:   constants.TIMELINE_DEFAULT_MESSAGE,
	}

	_, err = self.SuperTimelineStorer.UpdateTimeline(
		ctx, notebook_id, supertimeline, timeline)
	return err
}
