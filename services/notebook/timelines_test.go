package notebook_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/rand"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

func (self *NotebookManagerTestSuite) TestNotebookManagerTimelines() {
	assert.Retry(self.T(), 10, time.Second,
		self._TestNotebookManagerTimelines)
}

func (self *NotebookManagerTestSuite) _TestNotebookManagerTimelines(t *assert.R) {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(1715775587, 0)))
	defer closer()

	// Mock out cell ID generation for tests
	gen := utils.IncrementalIdGenerator(0)
	defer utils.SetIdGenerator(&gen)()

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(t, err)

	golden := ordereddict.NewDict()

	// Create a notebook the usual way - timelines are attached to the
	// notebook so we need somewhere to store them for this test.
	var notebook *api_proto.NotebookMetadata
	vtesting.WaitUntil(2*time.Second, t, func() bool {
		notebook, err = notebook_manager.NewNotebook(
			self.Ctx, "admin", &api_proto.NotebookMetadata{
				Name: "Timeline Notebook",
			})
		return err == nil
	})

	assert.Equal(t, len(notebook.CellMetadata), 1)
	golden.Set("Notebook Metadata", notebook)

	// Feed some data to the timeline.
	in := make(chan types.Row)
	go func() {
		defer close(in)

		for i := 1724123887; i < 1724123987; i += 10 {
			in <- ordereddict.NewDict().
				Set("MessageColumn", fmt.Sprintf("Message %v", i)).
				Set("Time", i).Set("Foo", "Bar").
				Set("description", fmt.Sprintf("Description %v", i))
		}
	}()

	timeline := &timelines_proto.Timeline{
		Id:                         "test",
		TimestampColumn:            "Time",
		MessageColumn:              "MessageColumn",
		TimestampDescriptionColumn: "description",
	}

	scope := vql_subsystem.MakeScope()
	super, err := notebook_manager.AddTimeline(self.Ctx, scope,
		notebook.NotebookId, "supertimeline", timeline, in)
	assert.NoError(t, err)

	golden.Set("Supertimeline", super)

	// test_utils.GetMemoryFileStore(t, self.ConfigObj).Debug()

	// Read the timeline out again.
	reader, err := notebook_manager.ReadTimeline(
		self.Ctx, notebook.NotebookId, "supertimeline",
		services.TimelineOptions{
			IncludeComponents: []string{"test"},
		})
	assert.NoError(t, err)

	events := []vfilter.Row{}
	for event := range reader.Read(self.Ctx) {
		events = append(events, event)
	}
	golden.Set("Events", events)

	goldie.Retry(t, self.T(), "TestNotebookManagerTimelines",
		json.MustMarshalIndent(golden))
}

func (self *NotebookManagerTestSuite) TestNotebookManagerTimelineAnnotations() {
	if testing.Short() {
		self.T().Skip("skipping test in short mode - too flakey on CI.")
		return
	}
	assert.Retry(self.T(), 10, time.Second,
		self._TestNotebookManagerTimelineAnnotations)
}

func (self *NotebookManagerTestSuite) _TestNotebookManagerTimelineAnnotations(
	t *assert.R) {
	defer rand.DisableRand()

	closer := utils.MockTime(utils.NewMockClock(time.Unix(1715775587, 0)))
	defer closer()

	closer2 := utils.MockGUID(53324)
	defer closer2()

	// Mock out cell ID generation for tests
	gen := utils.IncrementalIdGenerator(1)
	defer utils.SetIdGenerator(&gen)()

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(t, err)

	// Clear all previous notebooks
	all_notebooks, err := notebook_manager.GetAllNotebooks(self.Ctx,
		services.NotebookSearchOptions{
			Username:  "admin",
			Timelines: true,
		})
	assert.NoError(t, err)
	assert.Equal(t, len(all_notebooks), 0)

	for _, notebook := range all_notebooks {
		err := notebook_manager.DeleteNotebook(self.Ctx, notebook.NotebookId,
			nil, true)
		assert.NoError(t, err)
	}

	// Make sure they are cleared
	all_notebooks, err = notebook_manager.GetAllNotebooks(self.Ctx,
		services.NotebookSearchOptions{
			Username: "admin",
		})
	assert.NoError(t, err)
	assert.Equal(t, len(all_notebooks), 0)

	golden := ordereddict.NewDict()

	// Create a notebook the usual way - timelines are attached to the
	// notebook so we need somewhere to store them for this test.
	var notebook *api_proto.NotebookMetadata
	vtesting.WaitUntil(2*time.Second, t, func() bool {
		notebook, err = notebook_manager.NewNotebook(
			self.Ctx, "admin", &api_proto.NotebookMetadata{
				Name: "Timeline Annotation",
			})
		return err == nil
	})

	assert.Equal(t, len(notebook.CellMetadata), 1)
	golden.Set("Notebook Metadata", notebook)

	scope := vql_subsystem.MakeScope()
	err = notebook_manager.AnnotateTimeline(self.Ctx, scope,
		notebook.NotebookId, "supertimeline",
		"Foo is suspicious at being Bar!", "admin",
		time.Unix(1715775587, 0),
		ordereddict.NewDict().
			Set("Message", "Original Event Message 1").
			Set("OriginalEventField", "Extra field 1").
			Set("Foo", "Bar 1"))
	assert.NoError(t, err)

	// Make sure the timeline is added automatically
	notebook_metadata, err := notebook_manager.GetNotebook(self.Ctx,
		notebook.NotebookId, services.INCLUDE_UPLOADS)
	assert.NoError(t, err)

	golden.Set("Notebook Metadata After Annotation", notebook_metadata)

	// Check that GetAllNotebooks() returns this notebook now.
	all_notebooks, err = notebook_manager.GetAllNotebooks(self.Ctx,
		services.NotebookSearchOptions{
			Username:  "admin",
			Timelines: true,
		})
	assert.NoError(t, err)

	if len(all_notebooks) != 1 {
		json.Dump(all_notebooks)
	}

	assert.Equal(t, len(all_notebooks), 1)
	assert.Equal(t, all_notebooks[0].NotebookId, notebook.NotebookId)

	// Check that GetAllNotebooks() returns only notebooks for this
	// user.
	all_notebooks, err = notebook_manager.GetAllNotebooks(self.Ctx,
		services.NotebookSearchOptions{
			Username:  "someuser",
			Timelines: true,
		})
	assert.NoError(t, err)
	assert.Equal(t, len(all_notebooks), 0)

	read_all_events := func() (events []vfilter.Row) {
		// Read the timeline out again.
		reader, err := notebook_manager.ReadTimeline(
			self.Ctx, notebook.NotebookId, "supertimeline",
			services.TimelineOptions{
				IncludeComponents: []string{constants.TIMELINE_ANNOTATION},
			})
		assert.NoError(t, err)

		for event := range reader.Read(self.Ctx) {
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
		// This is normally the event that is being annotated in
		// full. It will be copied into the Annotation event.
		ordereddict.NewDict().
			Set("Message", "Original Event Message 2").
			Set("OriginalEventField", "Extra field 2").
			Set("Foo", "Older Bar 2"))
	assert.NoError(t, err)

	golden.Set("Next Annotation", read_all_events())

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	// Make sure that timeline metadata is updated
	timelines_metadata, err := notebook_manager.Timelines(
		self.Ctx, notebook.NotebookId)
	assert.NoError(t, err)

	golden.Set("Timelines Metadata", timelines_metadata)

	// Now update the first annotation.
	first_event := ordereddict.NewDict()
	first_event.MergeFrom(read_all_events()[0].(*ordereddict.Dict))
	golden.Set("First Event to update", first_event.Copy())

	timestamp_any, pres := first_event.Get("Timestamp")
	assert.True(t, pres)

	timestamp, ok := timestamp_any.(time.Time)
	assert.True(t, ok)

	// Update the event at the same timestamp.
	err = notebook_manager.AnnotateTimeline(self.Ctx, scope,
		notebook.NotebookId, "supertimeline",
		"Updated First Annotation - all other fields remain", "admin",
		timestamp, first_event)
	assert.NoError(t, err)

	golden.Set("Updated Annotations", read_all_events())

	goldie.Retry(t, self.T(), "TestNotebookManagerTimelineAnnotations",
		goldie.RemoveLines("_AnnotatedAt|modified_time|cell_id|notebook_id|current_version|\"[0-9][0-9]\"",
			json.MustMarshalIndent(golden)))

}
