package launcher

import (
	"context"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

/*
  For fast access in the GUI we keep a flows index. The index has flow
  summaries in sorted order stores as a regular result set. The GUI
  can then access the summaries index and apply transformations like
  sorting and filtering in the same way as any other result set.

  The flow index summary only contains flow metadata which does not
  change over time. The full records are retrieved as individual
  records from the datastore.

  For example, say there are 1000 flows in a client. There will be
  1000 separate flow context records:

  - clients/C.123/collections/F.CRFRQ7GTMH9DC.json.db
  - clients/C.123/collections/F.CRMITU9T2MQN8.json.db
  ....

  There is also a summary index as a regular result set:
  - clients/C.123/flow_index.json

  The GUI asks to see the first page which retrieves 10 records from
  the summary, the server then reads the first 10 full records based
  on the Flow Ids returned from the index.

  This speeds up filtering, sorting etc of the flow objects because we
  always deal with the index in an efficient way.

  How to keep the index in sync with the flows?

  When adding a new flow, the new summary is appended to the end of
  the index. This means that the next time the GUI requests a
  transformation of the index (e.g. sorted by timestamp or filtered)
  the server will automatically rebuild the transformed index. There
  is nothing more we need to do - this is the simple case as the new
  flow will natually fall at the end of the index (the index is sorted
  by creation time).

  When a flow is deleted this is more complicated:
  1. The collection object is removed from the data store.
  2. The deletion is written in the flow journal file by appending it to the end.

  Eventually we need to remove the deleted flow from the index result
  set completely, but we can not do this immediately because if the
  user deletes many flows quickly, the extra overheads of rebuilding
  the index for each deletion is not reasonable for IO performance on
  slow filesystems.

  Therefore we delay the index rebuild into a housekeeping thread
  which runs periodically:

  1. Read all deletions from the journal file
  2. Read all flow summary objects from the index
  3. Write them out into a new index file, if they are not in the deleted set.

  Immediately after deleted the flow, the flow will not appear as part
  of the GUI listing (because the full object is still missing). So
  before the house keeping thread runs the GUI will show e.g. 9 or 8
  rows in the same page when requested 10 rows, until the next
  housekeeping job fixes the index. This is considered an acceptable
  tradeoff.

*/

// The journal keeps a list of deleted flows to be removed from the
// main index.
func (self *FlowStorageManager) writeFlowJournal(
	config_obj *config_proto.Config, client_id, flow_id string) error {
	// Serialize access to the journal with the housekeeping thread.
	self.flow_journal_mu.Lock()
	defer self.flow_journal_mu.Unlock()

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	return journal.AppendToResultSet(config_obj,
		paths.FLOWS_JOUNRNAL,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("Type", "delete").
			Set("ClientId", client_id).
			Set("FlowId", flow_id)},
		services.JournalOptions{
			Sync: true,
		})
}

func (self *FlowStorageManager) clearJournal(
	config_obj *config_proto.Config) error {

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		paths.FLOWS_JOUNRNAL, json.DefaultEncOpts(),
		utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return err
	}
	rs_writer.Close()

	return nil
}

func (self *FlowStorageManager) RemoveFlowsFromJournal(
	ctx context.Context, config_obj *config_proto.Config) error {

	self.flow_journal_mu.Lock()
	defer self.flow_journal_mu.Unlock()

	file_store_factory := file_store.GetFileStore(config_obj)

	// ClientId: Set(FlowId)
	id_map := make(map[string]map[string]bool)

	journal_reader, err := result_sets.NewResultSetReader(
		file_store_factory, paths.FLOWS_JOUNRNAL)

	// No journal there is nothing to do.
	if err != nil {
		return nil
	}
	for row := range journal_reader.Rows(ctx) {
		client_id, _ := row.GetString("ClientId")
		if client_id == "" {
			continue
		}

		flow_id, _ := row.GetString("FlowId")
		if flow_id == "" {
			continue
		}

		client_set, pres := id_map[client_id]
		if !pres {
			client_set = make(map[string]bool)
			id_map[client_id] = client_set
		}

		client_set[flow_id] = true
	}
	journal_reader.Close()

	var r_err error
	for client_id, flows := range id_map {
		err := self.RemoveClientFlowsFromIndex(ctx, config_obj, client_id, flows)
		if err != nil {
			r_err = err
		}
	}

	// Only clear the journal if all reindex operations are successful.
	if r_err == nil {
		return self.clearJournal(config_obj)
	}

	return r_err
}
