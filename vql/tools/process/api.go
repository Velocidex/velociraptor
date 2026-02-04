package process

import (
	"context"
	"fmt"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type IProcessTracker interface {
	Get(ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool)
	Peek(ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool)

	Enrich(ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool)
	Stats() Stats
	Processes(ctx context.Context, scope vfilter.Scope) []*ProcessEntry
	Children(ctx context.Context, scope vfilter.Scope,
		id string, max_items int64) []*ProcessEntry
	CallChain(ctx context.Context, scope vfilter.Scope,
		id string, max_items int64) []*ProcessEntry

	// Listen to the update stream from the tracker.
	Updates() chan *UpdateProcessEntry
}

// The process entry should be emitted by the various queries to update the tracker.
type UpdateProcessEntry struct {
	Id         string            `vfilter:"required,field=id,doc=Process ID."`
	ParentId   string            `vfilter:"optional,field=parent_id,doc=The parent's process ID."`
	UpdateType string            `vfilter:"optional,field=update_type,doc=What this row represents."`
	StartTime  time.Time         `vfilter:"optional,field=start_time,doc=Timestamp for start,end updates"`
	EndTime    time.Time         `vfilter:"optional,field=end_time,doc=Timestamp for start,end updates"`
	Data       *ordereddict.Dict `vfilter:"optional,field=data,doc=Arbitrary key/value to associate with the process"`
}

func (self *UpdateProcessEntry) GetRealId() string {
	if self.StartTime.IsZero() || self.StartTime.Unix() < 0 {
		return self.Id + "-0"
	}
	return fmt.Sprintf("%v-%d", self.Id, uint64(self.StartTime.Unix()))
}

// The tracked process entry stores a process entry in the tracker and
// is emitted in various calls e.g. get, processes etc. It contains
// additional metadata which can be accessed by the VQL query.
type ProcessEntry struct {
	Id           string    `json:"Id"`                     // The tracker process ID
	RealId       string    `json:"RealId,omitempty"`       // For a link entry, this points to the real process ID.
	ParentId     string    `json:"ParentId"`               // The parent's process ID
	StartTime    time.Time `json:"StartTime"`              // When the process started
	LastSyncTime time.Time `json:"LastSyncTime,omitempty"` // Timestamp of the last full sync in which we saw this process
	EndTime      time.Time `json:"EndTime"`                // When the process is known to have exited
	JSONData     string    `json:"JSONData"`               // Serialized key/value to associate with the process"`
	Children     []string  `json:"Children,omitempty"`     // List of children IDs for this process"`

	// The original PID of the process
	pid string

	// A cached decoded version of the JSON data. For efficiency we
	// keep the data payload encoded as much as possible so we can
	// avoid unnecessary decode/encode cycles for disk based LRUs.
	data *ordereddict.Dict
}

func (self *ProcessEntry) AddChild(child_id string, max_items int) bool {
	if len(self.Children) > max_items {
		return false
	}

	if utils.InString(self.Children, child_id) {
		return false
	}

	self.Children = append(self.Children, child_id)
	return true
}

func (self *ProcessEntry) Data() *ordereddict.Dict {
	if self.data != nil {
		return self.data
	}

	self.data = ordereddict.NewDict()
	err := json.Unmarshal([]byte(self.JSONData), self.data)
	if err != nil {
		self.data.Set("Error", err.Error())
	}

	self.data.Update("Pid", self.Id).
		Update("Ppid", self.ParentId).
		Update("StartTime", self.StartTime).
		Update("EndTime", self.EndTime)

	return self.data
}

// This is used to encode the entry in VQL - we hide a lot of fields
// which should not be visible from VQL.
func (self *ProcessEntry) MarshalJSON() ([]byte, error) {
	children := self.Children
	if children == nil {
		children = []string{}
	}

	res := ordereddict.NewDict().
		Set("Id", self.Id).
		Set("ParentId", self.ParentId).
		Set("StartTime", self.StartTime).
		Set("EndTime", self.EndTime).
		Set("Data", self.Data()).
		Set("Children", children)

	return res.MarshalJSON()
}

func NewProcessEntryFromUpdate(update *UpdateProcessEntry) (*ProcessEntry, error) {
	now := utils.GetTime().Now().UTC()

	// Index by both PID and RealID
	real_id := update.GetRealId()

	serialized, err := json.MarshalString(update.Data)
	if err != nil {
		return nil, err
	}

	// NOTE: We assume that processes can not be reparented at
	// runtime. This may not be true on all OSs.

	// Build new process record based on the update
	return &ProcessEntry{
		Id:           real_id,
		pid:          update.Id,
		ParentId:     update.ParentId,
		StartTime:    update.StartTime,
		LastSyncTime: now,
		JSONData:     serialized,
	}, nil
}
