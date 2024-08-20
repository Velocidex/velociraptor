package notebook_test

import (
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
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

	test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	// Read the timeline out again.
	events, err := notebook_manager.ReadTimeline(self.Ctx, notebook.NotebookId,
		"supertimeline", time.Time{}, []string{"test"}, nil)
	assert.NoError(self.T(), err)

	for event := range events {
		json.Dump(event)
	}

	json.Dump(golden)
}
