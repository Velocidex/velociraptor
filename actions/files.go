package actions

import (
	"github.com/golang/protobuf/proto"
	"io/ioutil"
	"os"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/context"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type StatFile struct{}

func (self *StatFile) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := NewResponder(msg, output)

	arg, pres := responder.GetArgs().(*actions_proto.ListDirRequest)
	if !pres {
		responder.RaiseError("Request should be of type ListDirRequest")
		return
	}
	path, err := GetPathFromPathSpec(arg.Pathspec)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	stat, err := os.Stat(*path)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}
	stat_reply := buildStatEntryFromFileInfo(stat)
	if stat_reply != nil {
		stat_reply.Pathspec = arg.Pathspec
		responder.AddResponse(stat_reply)
	}
	responder.Return()
}

type ListDirectory struct{}

func (self *ListDirectory) Run(
	ctx *context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := NewResponder(msg, output)

	arg, pres := responder.GetArgs().(*actions_proto.ListDirRequest)
	if !pres {
		responder.RaiseError("Request should be of type ListDirRequest")
		return
	}

	path, err := GetPathFromPathSpec(arg.Pathspec)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	files, err := ioutil.ReadDir(*path)
	if err != nil {
		responder.RaiseError(err.Error())
		return
	}

	for _, stat := range files {
		stat_reply := buildStatEntryFromFileInfo(stat)
		if stat_reply != nil {
			new_pathspec := CopyPathspec(arg.Pathspec)
			last := LastPathspec(new_pathspec)
			last.NestedPath = &actions_proto.PathSpec{
				Pathtype: actions_proto.PathSpec_OS.Enum(),
				Path:     proto.String(stat.Name()),
			}
			stat_reply.Pathspec = new_pathspec
			responder.AddResponse(stat_reply)
		}
	}

	responder.Return()
}
