package labels

import (
	"context"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
)

// When not running on the frontend we set a dummy labeler.
type Dummy struct{}

func (self Dummy) LastLabelTimestamp(client_id string) uint64 {
	return 0
}

func (self Dummy) IsLabelSet(client_id string, label string) bool {
	return false
}

func (self Dummy) SetClientLabel(client_id, label string) error {
	return nil
}

func (self Dummy) RemoveClientLabel(client_id, label string) error {
	return nil
}

func (self Dummy) GetClientLabels(client_id string) []string {
	return nil
}

type CachedLabels struct {
	record *api_proto.ClientLabels

	lower_labels []string
}

func (self CachedLabels) Size() int {
	return 1
}

type Labeler struct {
	mu         sync.Mutex
	config_obj *config_proto.Config
	lru        *cache.LRUCache

	Clock utils.Clock
}

// If an explicit record does not exist, we retrieve it from searching the index.
func (self *Labeler) getRecordFromIndex(client_id string) (*CachedLabels, error) {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	result := &CachedLabels{
		record: &api_proto.ClientLabels{
			// We treat index timestamps as 0 since they
			// are legacy - new labeling operations should
			// advance this.
			Timestamp: 0,
		},
	}

	for _, label := range db.SearchClients(
		self.config_obj, constants.CLIENT_INDEX_URN,
		client_id, "", 0, 1000) {
		if strings.HasPrefix(label, "label:") {
			result.record.Label = append(result.record.Label,
				strings.TrimPrefix(label, "label:"))
		}
	}

	// Set the record for next time.
	client_path_manager := paths.NewClientPathManager(client_id)
	err = db.SetSubject(self.config_obj,
		client_path_manager.Labels(), result.record)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (self *Labeler) getRecord(client_id string) (*CachedLabels, error) {
	cached_any, ok := self.lru.Get(client_id)
	if ok {
		return cached_any.(*CachedLabels), nil
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	cached := &CachedLabels{record: &api_proto.ClientLabels{}}
	client_path_manager := paths.NewClientPathManager(client_id)
	err = db.GetSubject(self.config_obj,
		client_path_manager.Labels(), cached.record)

	// If there is no record, calculate a new record from the
	// client index.
	if err != nil || cached.record.Timestamp == 0 {
		cached, err = self.getRecordFromIndex(client_id)
		if err != nil {
			return nil, err
		}
	}

	self.preCalculatedLowCase(cached)

	self.lru.Set(client_id, cached)

	return cached, nil
}

func (self *Labeler) preCalculatedLowCase(cached *CachedLabels) {
	cached.lower_labels = nil
	for _, label := range cached.record.Label {
		cached.lower_labels = append(cached.lower_labels,
			strings.ToLower(label))
	}
}

func (self *Labeler) LastLabelTimestamp(client_id string) uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	cached, err := self.getRecord(client_id)
	if err != nil {
		return 0
	}

	return cached.record.Timestamp
}

func (self *Labeler) IsLabelSet(client_id string, checked_label string) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	checked_label = strings.ToLower(checked_label)
	cached, err := self.getRecord(client_id)
	if err != nil {
		return false
	}

	for _, label := range cached.lower_labels {
		if checked_label == label {
			return true
		}
	}

	return false
}

func (self *Labeler) notifyClient(client_id, new_label, operation string) error {
	// Notify other frontends about this change.
	artifact_path_manager := result_sets.NewArtifactPathManager(
		self.config_obj, client_id, "", "Server.Internal.Label")
	return services.GetJournal().PushRows(artifact_path_manager, []*ordereddict.Dict{
		ordereddict.NewDict().
			Set("client_id", client_id).
			Set("Operation", operation).
			Set("Label", new_label),
	})
}

func (self *Labeler) SetClientLabel(client_id, new_label string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	checked_label := strings.ToLower(new_label)
	cached, err := self.getRecord(client_id)
	if err != nil {
		return err
	}

	// Store the label in the datastore.
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	for _, label := range cached.lower_labels {
		if checked_label == label {
			// Label already set but make sure the index is updated.
			return self.adjustIndex(client_id, new_label, db.SetIndex)
		}
	}

	cached.record.Timestamp = uint64(self.Clock.Now().UnixNano())
	cached.record.Label = append(cached.record.Label, new_label)
	cached.lower_labels = append(cached.lower_labels, checked_label)

	client_path_manager := paths.NewClientPathManager(client_id)
	err = db.SetSubject(self.config_obj,
		client_path_manager.Labels(), cached.record)
	if err != nil {
		return err
	}

	// Cache the new record.
	self.lru.Set(client_id, cached)

	self.notifyClient(client_id, new_label, "Add")

	// Also adjust the index so client searches work.
	return self.adjustIndex(client_id, new_label, db.SetIndex)
}

func (self *Labeler) RemoveClientLabel(client_id, new_label string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	checked_label := strings.ToLower(new_label)
	cached, err := self.getRecord(client_id)
	if err != nil {
		return err
	}

	new_labels := []string{}
	for _, label := range cached.record.Label {
		if checked_label != strings.ToLower(label) {
			new_labels = append(new_labels, label)
		}
	}

	cached.record.Timestamp = uint64(self.Clock.Now().UnixNano())
	cached.record.Label = new_labels

	self.preCalculatedLowCase(cached)

	// Store the label in the datastore.
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	err = db.SetSubject(self.config_obj,
		client_path_manager.Labels(), cached.record)
	if err != nil {
		return err
	}

	// Cache the new record.
	self.lru.Set(client_id, cached)

	self.notifyClient(client_id, new_label, "Remove")

	// Also adjust the index.
	return self.adjustIndex(client_id, new_label, db.UnsetIndex)
}

type indexManipulator func(config_obj *config_proto.Config,
	index_urn string, entity string, keywords []string) error

func (self *Labeler) adjustIndex(
	client_id, label string,
	index_func indexManipulator) error {
	if !strings.HasPrefix(label, "label:") {
		label = "label:" + label
	}
	err := index_func(
		self.config_obj,
		constants.CLIENT_INDEX_URN,
		client_id, []string{label})
	if err != nil {
		return err
	}
	err = index_func(
		self.config_obj,
		constants.CLIENT_INDEX_URN,
		label, []string{client_id})
	if err != nil {
		return err
	}
	return nil
}

func (self *Labeler) GetClientLabels(client_id string) []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	cached, err := self.getRecord(client_id)
	if err != nil {
		return nil
	}

	return cached.record.Label
}

// Receive notification from other frontends that client labels have
// changed for a particular client. For now we just dumbly flush the
// cache for the client which was modified - this forces us to hit up
// the database on next access and get fresh data.
func (self *Labeler) ProcessRow(
	ctx context.Context,
	row *ordereddict.Dict) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	client_id, pres := row.GetString("client_id")
	if pres {
		self.lru.Delete(client_id)
	}
	return nil
}

func (self *Labeler) Start(ctx context.Context, wg *sync.WaitGroup) error {
	self.lru = cache.NewLRUCache(self.config_obj.Frontend.ExpectedClients)

	// Wait in this func until we are ready to monitor.
	local_wg := &sync.WaitGroup{}
	local_wg.Add(1)

	wg.Add(1)
	go func() {
		defer wg.Done()

		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Info("Starting Label service.")

		events, cancel := services.GetJournal().Watch("Server.Internal.Label")
		defer cancel()

		local_wg.Done()

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-events:
				if !ok {
					return
				}
				err := self.ProcessRow(ctx, event)
				if err != nil {
					logger.Error("Enrollment Service: %v", err)
				}
			}
		}
	}()

	local_wg.Wait()

	return nil
}

func StartLabelService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	if config_obj.Frontend == nil {
		return nil
	}

	labeler := &Labeler{
		config_obj: config_obj,
		Clock:      &utils.RealClock{},
	}
	err := labeler.Start(ctx, wg)
	if err != nil {
		return err
	}

	services.RegisterLabeler(labeler)

	return nil
}

func init() {
	services.RegisterLabeler(Dummy{})
}
