package actions

import (
	"syscall"
	"os"
	"io/ioutil"
	"github.com/golang/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/context"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

func buildStatEntryFromFileInfo(stat os.FileInfo) *actions_proto.StatEntry {
	sys_stat, ok := stat.Sys().(*syscall.Stat_t)
	if ok {
		stat_reply := &actions_proto.StatEntry{
			StMode: proto.Uint64(uint64(sys_stat.Mode)),
			StIno: proto.Uint32(uint32(sys_stat.Ino)),
			StDev: proto.Uint32(uint32(sys_stat.Dev)),
			StNlink: proto.Uint32(uint32(sys_stat.Nlink)),
			StUid: &sys_stat.Uid,
			StGid: &sys_stat.Gid,
			StSize: proto.Uint64(uint64(sys_stat.Size)),
			StAtime: proto.Uint64(uint64(sys_stat.Atim.Sec)),
			StMtime: proto.Uint64(uint64(sys_stat.Mtim.Sec)),
			StCtime: proto.Uint64(uint64(sys_stat.Ctim.Sec)),
			StBlocks: proto.Uint32(uint32(sys_stat.Blocks)),
			StBlksize: proto.Uint32(uint32(sys_stat.Blksize)),
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
			last.NestedPath = &actions_proto.PathSpec{
				Pathtype: actions_proto.PathSpec_OS.Enum(),
				Path: proto.String(stat.Name()),
			}
			stat_reply.Pathspec = new_pathspec
			responder.AddResponse(stat_reply)
		}
	}

	return responder.Return()
}
