package reporting

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/vfilter"
)

var (
	ContainerTracker = NewContainerTracker()
)

type WriterInfo struct {
	Name             string
	TmpFile          string
	CompressedSize   int
	UncompressedSize int
	Created          time.Time
	LastWrite        time.Time
	Closed           time.Time
}

type ContainerInfo struct {
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

func (self *_ContainerTracker) UpdateContainerWriter(
	container_id, writer_id uint64, cb func(info *WriterInfo)) {

	self.UpdateContainer(container_id, func(info *ContainerInfo) {
		writer_info, pres := info.InFlightWriters[writer_id]
		if !pres {
			writer_info = &WriterInfo{}
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
			InFlightWriters: make(map[uint64]*WriterInfo),
		}
		self.containers[id] = record
	}

	cb(record)
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
	return &_ContainerTracker{
		containers: make(map[uint64]*ContainerInfo),
	}
}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "ExportContainers",
		Description:   "Report the state of current exports",
		ProfileWriter: ContainerTracker.WriteMetrics,
	})
}
