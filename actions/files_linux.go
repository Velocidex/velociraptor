package actions

import (
	"github.com/golang/protobuf/proto"
	"os"
	"syscall"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
)

func buildStatEntryFromFileInfo(stat os.FileInfo) *actions_proto.StatEntry {
	sys_stat, ok := stat.Sys().(*syscall.Stat_t)
	if ok {
		stat_reply := &actions_proto.StatEntry{
			StMode:    proto.Uint64(uint64(sys_stat.Mode)),
			StIno:     proto.Uint32(uint32(sys_stat.Ino)),
			StDev:     proto.Uint32(uint32(sys_stat.Dev)),
			StNlink:   proto.Uint32(uint32(sys_stat.Nlink)),
			StUid:     &sys_stat.Uid,
			StGid:     &sys_stat.Gid,
			StSize:    proto.Uint64(uint64(sys_stat.Size)),
			StAtime:   proto.Uint64(uint64(sys_stat.Atim.Sec)),
			StMtime:   proto.Uint64(uint64(sys_stat.Mtim.Sec)),
			StCtime:   proto.Uint64(uint64(sys_stat.Ctim.Sec)),
			StBlocks:  proto.Uint32(uint32(sys_stat.Blocks)),
			StBlksize: proto.Uint32(uint32(sys_stat.Blksize)),
		}
		return stat_reply
	}

	return nil
}
