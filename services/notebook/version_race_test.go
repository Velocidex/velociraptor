package notebook_test

import (
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

// The notebook store version is used by GetSharedNotebooks to decide
// whether the cached index file is stale: the index is served when
// `index_mtime >= store_version`. The store version is the max of a
// monotonic counter (bumped on every mutation) and all notebooks'
// ModifiedTime. ModifiedTime is second-granularity (proto int64
// seconds), so a purely time-based version masked mutations landing in
// the same second as the last index write, serving a stale index.
//
// The version is now bumped monotonically on every mutation: it is set
// to max(current_version, now) + 1, guaranteeing it always moves ahead
// regardless of clock resolution or clock going backwards.
func (self *ACLTestSuite) TestDeleteInSameSecondBumpsVersion() {
	mock_clock := utils.NewMockClock(time.Unix(10, 0))
	defer utils.MockTime(mock_clock)()

	notebook_manager_any, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)
	notebook_manager := notebook_manager_any.(*notebook.NotebookManager)

	// Set a notebook: ModifiedTime is set to Now().Unix() = 10.
	new_notebook := &api_proto.NotebookMetadata{
		NotebookId: "N.12345",
		Creator:    "Creator",
	}
	err = notebook_manager.Store.SetNotebook(new_notebook)
	assert.NoError(self.T(), err)

	// The version must be strictly ahead of the notebook's ModifiedTime
	// (in nanoseconds), so a subsequent mutation in the same second is
	// not masked.
	assert.True(self.T(), notebook_manager.Store.Version() > int64(10*1000000000),
		"version must be ahead of ModifiedTime after a set")

	// Simulate the index being written at this point, then a delete
	// landing in the SAME second (500ms later - still second 10).
	mock_clock.Sleep(500 * time.Millisecond)

	err = notebook_manager.Store.DeleteNotebook(
		self.Ctx, "N.12345", nil, true)
	assert.NoError(self.T(), err)

	// The delete must bump the version beyond the notebook's ModifiedTime
	// (in nanoseconds), otherwise GetSharedNotebooks would serve the stale
	// index still containing the deleted notebook.
	version := notebook_manager.Store.Version()
	assert.True(self.T(), version > int64(10*1000000000),
		"delete in the same second must bump the store version beyond "+
			"ModifiedTime (got %v, want > %v)",
		version, int64(10*1000000000))
}

// TestModifyAfterDeleteSameSecond covers the reverse direction: a
// notebook modification landing in the same second as a previous delete.
// ModifiedTime truncates to Unix() seconds, so without the monotonic
// bump the version would stay at the delete's timestamp and the index
// written in between would keep being served (missing the new notebook).
func (self *ACLTestSuite) TestModifyAfterDeleteSameSecond() {
	mock_clock := utils.NewMockClock(time.Unix(10, 0))
	defer utils.MockTime(mock_clock)()

	notebook_manager_any, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)
	notebook_manager := notebook_manager_any.(*notebook.NotebookManager)

	// Set a notebook: ModifiedTime = 10 (seconds).
	err = notebook_manager.Store.SetNotebook(&api_proto.NotebookMetadata{
		NotebookId: "N.12345", Creator: "Creator"})
	assert.NoError(self.T(), err)

	// Delete it 500ms later (still second 10).
	mock_clock.Sleep(500 * time.Millisecond)
	err = notebook_manager.Store.DeleteNotebook(
		self.Ctx, "N.12345", nil, true)
	assert.NoError(self.T(), err)
	version_after_delete := notebook_manager.Store.Version()

	// Index gets rebuilt here (mtime >= version -> served).

	// Now modify another notebook 200ms later (still second 10!).
	// ModifiedTime truncates to Unix() = 10 seconds.
	mock_clock.Sleep(200 * time.Millisecond)
	err = notebook_manager.Store.SetNotebook(&api_proto.NotebookMetadata{
		NotebookId: "N.67890", Creator: "Creator"})
	assert.NoError(self.T(), err)

	// The modification MUST bump the version, otherwise the index written
	// after the delete is still served and N.67890 never appears.
	version_after_modify := notebook_manager.Store.Version()
	assert.True(self.T(), version_after_modify > version_after_delete,
		"modify in the same second as a delete must bump the version "+
			"(got %v, want > %v)", version_after_modify, version_after_delete)
}