package notebook_test

import (
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

func (self *NotebookManagerTestSuite) TestNotebookManagerTimelines() {
	// Mock out cell ID generation for tests
	gen := utils.IncrementalIdGenerator(0)
	utils.SetIdGenerator(&gen)

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	golden := ordereddict.NewDict()

	// Create a notebook the usual way - timelines are attached to the
	// notebook so we need somewhere to store them for this test.
	var notebook *api_proto.NotebookMetadata
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		notebook, err = notebook_manager.NewNotebook(
			self.Ctx, "admin", &api_proto.NotebookMetadata{
				Name: "Timeline Notebook",
			})
		return err == nil
	})

	assert.Equal(self.T(), len(notebook.CellMetadata), 1)
	golden.Set("Notebook Metadata", notebook)

	// Feed some data to the timeline.
	in := make(chan types.Row)
	go func() {
		defer close(in)

		for i := 1724123887; i < 1724123987; i += 10 {
			in <- ordereddict.NewDict().
				Set("Time", i).Set("Foo", "Bar")
		}
	}()

	timeline := &timelines_proto.Timeline{
		TimestampColumn:            "Time",
		MessageColumn:              "MessageColumn",
		TimestampDescriptionColumn: "description",
	}

	scope := vql_subsystem.MakeScope()
	super, err := notebook_manager.AddTimeline(self.Ctx, scope,
		notebook.NotebookId, "supertimeline", timeline, in)
	assert.NoError(self.T(), err)

	golden.Set("Supertimeline", super)

	//	test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	// Read the timeline out again.
	events_chan, err := notebook_manager.ReadTimeline(
		self.Ctx, notebook.NotebookId,
		"supertimeline", time.Time{}, []string{"test"}, nil)
	assert.NoError(self.T(), err)

	events := []vfilter.Row{}
	for event := range events_chan {
		events = append(events, event)
	}
	golden.Set("Events", events)

	goldie.Assert(self.T(), "TestNotebookManagerTimelines",
		json.MustMarshalIndent(golden))

}

func (self *NotebookManagerTestSuite) TestNotebookManagerTimelineAnnotations() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(1715775587, 0)))
	defer closer()

	// Mock out cell ID generation for tests
	gen := utils.IncrementalIdGenerator(0)
	utils.SetIdGenerator(&gen)

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	golden := ordereddict.NewDict()

	// Create a notebook the usual way - timelines are attached to the
	// notebook so we need somewhere to store them for this test.
	var notebook *api_proto.NotebookMetadata
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		notebook, err = notebook_manager.NewNotebook(
			self.Ctx, "admin", &api_proto.NotebookMetadata{
				Name: "Timeline Annotation",
			})
		return err == nil
	})

	assert.Equal(self.T(), len(notebook.CellMetadata), 1)
	golden.Set("Notebook Metadata", notebook)

	scope := vql_subsystem.MakeScope()
	err = notebook_manager.AnnotateTimeline(self.Ctx, scope,
		notebook.NotebookId, "supertimeline",
		"Foo is suspicious at being Bar!", "admin",
		time.Unix(1715775587, 0),
		ordereddict.NewDict().Set("Foo", "Bar"))
	assert.NoError(self.T(), err)

	// Make sure the timeline is added automatically
	notebook_metadata, err := notebook_manager.GetNotebook(self.Ctx,
		notebook.NotebookId, services.INCLUDE_UPLOADS)
	assert.NoError(self.T(), err)

	golden.Set("Notebook Metadata After Annotation", notebook_metadata)

	read_all_events := func() (events []vfilter.Row) {
		// Read the timeline out again.
		events_chan, err := notebook_manager.ReadTimeline(
			self.Ctx, notebook.NotebookId,
			"supertimeline", time.Unix(0, 0),
			[]string{constants.TIMELINE_ANNOTATION}, nil)
		assert.NoError(self.T(), err)

		for event := range events_chan {
			events = append(events, event)
		}
		return events
	}
	golden.Set("Events", read_all_events())

	// Add another annotation before the first one.
	err = notebook_manager.AnnotateTimeline(self.Ctx, scope,
		notebook.NotebookId, "supertimeline",
		"An Earlier Foo is suspicious at being Older Bar!", "mike",
		time.Unix(1711775587, 0),
		ordereddict.NewDict().Set("Foo", "Older Bar"))
	assert.NoError(self.T(), err)

	golden.Set("Next Annotation", read_all_events())

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	goldie.Assert(self.T(), "TestNotebookManagerTimelineAnnotations",
		json.MustMarshalIndent(golden))

}
