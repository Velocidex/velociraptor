package actions

import (
	"os"
	"syscall"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
)

func buildStatEntryFromFileInfo(stat os.FileInfo) *actions_proto.StatEntry {
	sys_stat, ok := stat.Sys().(*syscall.Stat_t)
	if ok {
		stat_reply := &actions_proto.StatEntry{
			StMode:    uint64(sys_stat.Mode),
			StIno:     uint32(sys_stat.Ino),
			StDev:     uint32(sys_stat.Dev),
			StNlink:   uint32(sys_stat.Nlink),
			StUid:     sys_stat.Uid,
			StGid:     sys_stat.Gid,
			StSize:    uint64(sys_stat.Size),
			StAtime:   uint64(sys_stat.Atimespec.Sec),
			StMtime:   uint64(sys_stat.Mtimespec.Sec),
			StCtime:   uint64(sys_stat.Ctimespec.Sec),
			StBlocks:  uint32(sys_stat.Blocks),
			StBlksize: uint32(sys_stat.Blksize),
		}
		return stat_reply
	}

	return nil
}
