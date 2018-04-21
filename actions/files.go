package actions

import (
	"syscall"
	"os"
	"io/ioutil"
	"www.velocidex.com/golang/velociraptor/context"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

func buildStatEntryFromFileInfo(stat os.FileInfo) *actions_proto.StatEntry {
	sys_stat, ok := stat.Sys().(*syscall.Stat_t)
	if ok {
		mode := uint64(sys_stat.Mode)
		inode := uint32(sys_stat.Ino)
		dev := uint32(sys_stat.Dev)
		nlink := uint32(sys_stat.Nlink)
		atime := uint64(sys_stat.Atim.Sec)
		mtime := uint64(sys_stat.Mtim.Sec)
		ctime := uint64(sys_stat.Ctim.Sec)
		size := uint64(sys_stat.Size)
		blocks := uint32(sys_stat.Blocks)
		blksize := uint32(sys_stat.Blksize)

		stat_reply := &actions_proto.StatEntry{
			StMode: &mode,
			StIno: &inode,
			StDev: &dev,
			StNlink: &nlink,
			StUid: &sys_stat.Uid,
			StGid: &sys_stat.Gid,
			StSize: &size,
			StAtime: &atime,
			StMtime: &mtime,
			StCtime: &ctime,
			StBlocks: &blocks,
			StBlksize: &blksize,
		}
		return stat_reply
	}

	return nil
}


type StatFile struct{}

func (self *StatFile) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	responder := NewResponder(msg)

	arg, pres := responder.GetArgs().(*actions_proto.ListDirRequest)
	if !pres {
		return responder.RaiseError("Request should be of type ListDirRequest")
	}
	path, err := GetPathFromPathSpec(arg.Pathspec)
	if err != nil {
		return responder.RaiseError(err.Error())
	}

	stat, err := os.Stat(*path)
	if err != nil {
		return responder.RaiseError(err.Error())
	}
	stat_reply := buildStatEntryFromFileInfo(stat)
	if stat_reply != nil {
		stat_reply.Pathspec = arg.Pathspec
		responder.AddResponse(stat_reply)
	}
	return responder.Return()
}


type ListDirectory struct{}

func (self *ListDirectory) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage) []*crypto_proto.GrrMessage {
	responder := NewResponder(msg)

	arg, pres := responder.GetArgs().(*actions_proto.ListDirRequest)
	if !pres {
		return responder.RaiseError("Request should be of type ListDirRequest")
	}

	path, err := GetPathFromPathSpec(arg.Pathspec)
	if err != nil {
		return responder.RaiseError(err.Error())
	}

	files, err := ioutil.ReadDir(*path)
	if err != nil {
		return responder.RaiseError(err.Error())
	}

	for _, stat := range files {
		stat_reply := buildStatEntryFromFileInfo(stat)
		if stat_reply != nil {
			new_pathspec := CopyPathspec(arg.Pathspec)
			last := LastPathspec(new_pathspec)
			name := stat.Name()
			last.NestedPath = &actions_proto.PathSpec{
				Pathtype: actions_proto.PathSpec_OS.Enum(),
				Path: &name,
			}
			stat_reply.Pathspec = new_pathspec
			responder.AddResponse(stat_reply)
		}
	}

	return responder.Return()
}
