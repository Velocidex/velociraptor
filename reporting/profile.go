package reporting

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	ContainerTracker = NewContainerTracker()
)

type WriterInfo struct {
	id uint64

	Name             string
	TmpFile          string
	CompressedSize   int
	UncompressedSize int
	Created          time.Time
	LastWrite        time.Time
	Closed           time.Time
}

type ContainerInfo struct {
	id              uint64
	Name            string
	BackingFile     string
	Stats           *api_proto.ContainerStats
	InFlightWriters map[uint64]*WriterInfo
	CreateTime      time.Time
	CloseTime       time.Time
}

type _ContainerTracker struct {
	mu         sync.Mutex
	containers map[uint64]*ContainerInfo
}

func (self *_ContainerTracker) GetActiveMembers(
	id uint64) []*api_proto.ContainerMemberStats {
	self.mu.Lock()
	defer self.mu.Unlock()

	record, pres := self.containers[id]
	if !pres {
		return nil
	}

	var res []*api_proto.ContainerMemberStats
	for _, w := range record.InFlightWriters {
		// Skip the files that are already closed.
		if !w.Closed.IsZero() {
			continue
		}
		res = append(res, &api_proto.ContainerMemberStats{
			Name:             w.Name,
			UncompressedSize: uint64(w.UncompressedSize),
			CompressedSize:   uint64(w.CompressedSize),
		})
	}
	return res
}

func (self *_ContainerTracker) UpdateContainerWriter(
	container_id, writer_id uint64, cb func(info *WriterInfo)) {

	self.UpdateContainer(container_id, func(info *ContainerInfo) {
		writer_info, pres := info.InFlightWriters[writer_id]
		if !pres {
			writer_info = &WriterInfo{
				id: writer_id,
			}
			info.InFlightWriters[writer_id] = writer_info
		}

		cb(writer_info)
	})
}

func (self *_ContainerTracker) UpdateContainer(
	id uint64, cb func(info *ContainerInfo)) {
	self.mu.Lock()
	defer self.mu.Unlock()

	record, pres := self.containers[id]
	if !pres {
		record = &ContainerInfo{
			id:              id,
			InFlightWriters: make(map[uint64]*WriterInfo),
		}
		self.containers[id] = record
	}

	cb(record)
}

func (self *_ContainerTracker) reap() {
	self.mu.Lock()
	defer self.mu.Unlock()

	containers := make([]*ContainerInfo, 0, len(self.containers))
	for _, c := range self.containers {
		containers = append(containers, c)
	}

	oldest := utils.GetTime().Now().Add(-time.Minute * 10)
	for _, c := range containers {
		// reap old containers completely.
		if !c.CloseTime.IsZero() &&
			c.CloseTime.Before(oldest) {
			delete(self.containers, c.id)
			continue
		}

		writers := make([]*WriterInfo, 0, len(c.InFlightWriters))
		for _, w := range c.InFlightWriters {
			writers = append(writers, w)
		}
		for _, w := range writers {
			if !w.Closed.IsZero() &&
				w.Closed.Before(oldest) {
				delete(c.InFlightWriters, w.id)
			}
		}
	}
}

func (self *_ContainerTracker) WriteMetrics(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row) {

	self.mu.Lock()
	defer self.mu.Unlock()

	for _, container := range self.containers {
		for _, w := range container.InFlightWriters {
			output_chan <- ordereddict.NewDict().
				Set("ContainerName", container.Name).
				Set("ZipCreateTime", container.CreateTime).
				Set("ZipCloseTime", container.CloseTime).
				Set("MemberName", w.Name).
				Set("MemberTmpFile", w.TmpFile).
				Set("MemberCreate", w.Created).
				Set("MemberCompressedSize", w.CompressedSize).
				Set("MemberUncompressedSize", w.UncompressedSize).
				Set("MemberLastWrite", w.LastWrite).
				Set("MemberClosed", w.Closed)
		}
	}
}

func NewContainerTracker() *_ContainerTracker {
	res := &_ContainerTracker{
		containers: make(map[uint64]*ContainerInfo),
	}

	go func() {
		for {
			time.Sleep(time.Minute)
			res.reap()
		}
	}()

	return res
}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "ExportContainers",
		Description:   "Report the state of current exports",
		ProfileWriter: ContainerTracker.WriteMetrics,
		Categories:    []string{"Global", "Services"},
	})
}
