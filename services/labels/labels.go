package labels

import (
	"context"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	metricLabelLRU = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "labeler_lru_total",
			Help: "Total labels cached",
		})
)

// When not running on the frontend we set a dummy labeler.
type Dummy struct{}

func (self Dummy) LastLabelTimestamp(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) uint64 {
	return 0
}

func (self Dummy) IsLabelSet(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, label string) bool {
	return false
}

func (self Dummy) SetClientLabel(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, label string) error {
	return nil
}

func (self Dummy) RemoveClientLabel(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, label string) error {
	return nil
}

func (self Dummy) GetClientLabels(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) []string {
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
	mu  sync.Mutex
	lru *ttlcache.Cache
}

// Assumption: We hold the lock entering this function.
func (self *Labeler) getRecord(
	ctx context.Context,
	config_obj *config_proto.Config, client_id string) (*CachedLabels, error) {
	cached_any, err := self.lru.Get(client_id)
	if err == nil {
		return cached_any.(*CachedLabels), nil
	}

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
	}

	client_info, err := client_info_manager.Get(ctx, client_id)
	if err != nil {
		return nil, err
	}

	cached := &CachedLabels{record: &api_proto.ClientLabels{
		Timestamp: client_info.LabelsTimestamp,
		Label:     client_info.Labels,
	}}

	cached.lower_labels = nil
	for _, label := range cached.record.Label {
		cached.lower_labels = append(cached.lower_labels,
			strings.ToLower(label))
	}

	// Now set back to the lru with lock
	return cached, self.lru.Set(client_id, cached)
}

func (self *Labeler) LastLabelTimestamp(
	ctx context.Context,
	config_obj *config_proto.Config, client_id string) uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	cached, err := self.getRecord(ctx, config_obj, client_id)
	if err != nil {
		return 0
	}

	return cached.record.Timestamp
}

func (self *Labeler) IsLabelSet(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, checked_label string) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	checked_label = strings.ToLower(checked_label)

	// This is a special label that all clients belong to. It is
	// used in the GUI to indicate all clients.
	if checked_label == "all" {
		return true
	}

	cached, err := self.getRecord(ctx, config_obj, client_id)
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

func (self *Labeler) notifyClient(
	ctx context.Context, config_obj *config_proto.Config,
	client_id, new_label, operation string) error {
	// Notify other frontends about this change.
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	journal.PushRowsToArtifactAsync(ctx, config_obj,
		ordereddict.NewDict().
			Set("client_id", client_id).
			Set("Operation", operation).
			Set("Label", new_label),
		"Server.Internal.Label")
	return nil
}

func (self *Labeler) SetClientLabel(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, new_label string) error {

	new_label = strings.TrimSpace(new_label)
	checked_label := strings.ToLower(new_label)

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	err = client_info_manager.ValidateClientId(client_id)
	if err != nil {
		return err
	}

	err = client_info_manager.Modify(ctx, client_id,
		func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
			if client_info == nil {
				client_info = &services.ClientInfo{ClientInfo: &actions_proto.ClientInfo{}}
				client_info.ClientId = client_id
			}

			// Label is already set. O(n) but n should be small.
			for _, l := range client_info.Labels {
				if checked_label == strings.ToLower(l) {
					// No change is needed the label is already in
					// there.
					return nil, nil
				}
			}

			client_info.Labels = append(client_info.Labels, new_label)
			client_info.LabelsTimestamp = uint64(
				utils.GetTime().Now().UnixNano())

			return client_info, nil
		})
	if err != nil {
		return err
	}

	// Remove the record from the LRU - we will retrieve it from the
	// client info manager later.
	_ = self.lru.Remove(client_id)

	// Notify any clients that labels are added.
	err = self.notifyClient(ctx, config_obj, client_id, new_label, "Add")
	if err != nil {
		return err
	}

	// Also adjust the index so client searches work. If there is no
	// indexing services it is not an error.
	indexer, err := services.GetIndexer(config_obj)
	if err == nil {
		return indexer.SetIndex(client_id, "label:"+new_label)
	}
	return nil
}

func (self *Labeler) RemoveClientLabel(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, new_label string) error {

	checked_label := strings.ToLower(new_label)

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	err = client_info_manager.Modify(ctx, client_id,
		func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
			if client_info == nil {
				client_info = &services.ClientInfo{ClientInfo: &actions_proto.ClientInfo{}}
				client_info.ClientId = client_id
			}

			new_labels := []string{}

			// Label is already set. O(n) but n should be small.
			for _, l := range client_info.Labels {
				if checked_label != strings.ToLower(l) {
					new_labels = append(new_labels, l)
				}
			}

			// Nothing was done - no change is needed.
			if len(client_info.Labels) == len(new_labels) {
				return nil, nil
			}

			client_info.Labels = new_labels
			client_info.LabelsTimestamp = uint64(
				utils.GetTime().Now().UnixNano())

			return client_info, nil
		})
	if err != nil {
		return err
	}

	// Remove the record from the LRU - we will retrieve it from the
	// client info manager later.
	_ = self.lru.Remove(client_id)

	err = self.notifyClient(ctx, config_obj, client_id, new_label, "Remove")
	if err != nil {
		return err
	}

	// Also adjust the index.
	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return err
	}

	return indexer.UnsetIndex(client_id, "label:"+new_label)
}

func (self *Labeler) GetClientLabels(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	cached, err := self.getRecord(ctx, config_obj, client_id)
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
		_ = self.lru.Remove(client_id)
	}
	return nil
}

func (self *Labeler) Start(ctx context.Context,
	config_obj *config_proto.Config, wg *sync.WaitGroup) error {

	expected_clients := int64(100)
	if config_obj.Frontend != nil && config_obj.Frontend.Resources != nil {
		expected_clients = config_obj.Frontend.Resources.ExpectedClients
	}

	self.lru = ttlcache.NewCache()
	self.lru.SetCacheSizeLimit(int(expected_clients))
	self.lru.SetNewItemCallback(
		func(key string, value interface{}) error {
			metricLabelLRU.Inc()
			return nil
		})
	self.lru.SetExpirationCallback(
		func(key string, value interface{}) error {
			metricLabelLRU.Dec()
			return nil
		})

	go func() {
		<-ctx.Done()
		self.lru.Close()
	}()

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	events, cancel := journal.Watch(
		ctx, "Server.Internal.Label", "Labeler")

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> Label service.")

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
					logger.Error("Label Service: %v", err)
				}
			}
		}
	}()

	return nil
}

func NewLabelerService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.Labeler, error) {

	if config_obj.Frontend == nil {
		return Dummy{}, nil
	}

	labeler := &Labeler{}
	return labeler, labeler.Start(ctx, config_obj, wg)
}
