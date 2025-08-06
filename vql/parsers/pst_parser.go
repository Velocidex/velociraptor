//go:build !arm && !mips && !(linux && 386)
// +build !arm
// +build !mips
// +build !linux !386

package parsers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	pst "github.com/mooijtech/go-pst/v6/pkg"
	"github.com/mooijtech/go-pst/v6/pkg/properties"
	"www.velocidex.com/golang/velociraptor/accessors"
	pst_accessor "www.velocidex.com/golang/velociraptor/accessors/pst"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PSTParserArgs struct {
	Filename    *accessors.OSPath `vfilter:"required,field=filename,doc=The PST file to parse."`
	Accessor    string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	RawMessages bool              `vfilter:"optional,field=raw,doc=If set we emit the raw message object for all objects"`
}

type PSTParser struct{}

func (self *PSTParser) getAttachments(
	scope vfilter.Scope, message *pst.Message,
	pstFile *pst_accessor.PSTFile,
	accessor string, pst_path *accessors.OSPath) (res []*ordereddict.Dict) {

	attachmentIterator, err := message.GetAttachmentIterator()
	if errors.Is(err, pst.ErrAttachmentsNotFound) || err != nil {
		return res
	}

	// Iterate through attachments.
	for attachmentIterator.Next() {
		attachment := attachmentIterator.Value()

		filename := attachment.GetAttachLongFilename()
		if filename == "" {
			filename = "NONAME"
		}

		attachmentReader, err := attachment.PropertyContext.GetPropertyReader(
			14081, attachment.LocalDescriptors)
		if err != nil {
			continue
		}

		res = append(res, ordereddict.NewDict().
			Set("Name", filename).
			Set("Size", attachmentReader.Size()).
			Set("Path", pstFile.GetPath(attachment.Identifier)))
	}
	return res
}

func (self *PSTParser) getRawMessage(item interface{}) (*ordereddict.Dict, error) {
	serialized, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}

	msg_type_part := strings.Split(fmt.Sprintf("%T", item), ".")
	msg_type := ""
	if len(msg_type_part) > 1 {
		msg_type = msg_type_part[1]
	}

	output := ordereddict.NewDict()
	err = output.UnmarshalJSON(serialized)
	if err != nil {
		return nil, err
	}

	return output.Set("Type", msg_type), nil
}

func (self *PSTParser) walkFolders(
	ctx context.Context, scope vfilter.Scope, arg *PSTParserArgs,
	pstFile *pst_accessor.PSTFile, folder *pst.Folder,
	output_chan chan vfilter.Row) error {

	if arg.RawMessages {
		select {
		case <-ctx.Done():
			return nil

		case output_chan <- ordereddict.NewDict().
			Set("Type", "Folder").
			Set("Name", folder.Name).
			Set("Path", pstFile.GetPath(folder.Identifier)).
			Set("Identifier", folder.Identifier).
			Set("HasSubFolders", folder.HasSubFolders).
			Set("MessageCount", folder.MessageCount):
		}
	}

	messageIterator, err := folder.GetMessageIterator()
	if errors.Is(err, pst.ErrMessagesNotFound) {
		// Folder has no messages.
		return nil

	}

	if err != nil {
		return err
	}

	for messageIterator.Next() {
		message := messageIterator.Value()

		if arg.RawMessages && message.Properties != nil {
			output, err := self.getRawMessage(message.Properties)
			if err != nil {
				continue
			}

			output.Set("Path", pstFile.GetPath(message.Identifier)).
				Set("Attachments", self.getAttachments(
					scope, message, pstFile, arg.Accessor, arg.Filename))

			select {
			case <-ctx.Done():
				return nil
			case output_chan <- output:
			}
			continue
		}

		// Process the message and send it to the output channel
		props, ok := message.Properties.(*properties.Message)
		if !ok || props == nil {
			continue
		}

		output := ordereddict.NewDict().
			Set("Path", pstFile.GetPath(folder.Identifier)).
			Set("Sender", props.GetSenderEmailAddress()).
			Set("Receiver", props.GetReceivedByEmailAddress()).
			Set("Subject", props.GetSubject()).
			Set("Message", props.GetBody()).
			Set("Delivered", time.Unix(0, props.GetMessageDeliveryTime())).
			Set("Attachments", self.getAttachments(
				scope, message, pstFile, arg.Accessor, arg.Filename))

		select {
		case <-ctx.Done():
			return nil
		case output_chan <- output:
		}
	}

	if messageIterator.Err() != nil {
		return messageIterator.Err()
	}

	return nil
}

func (self *PSTParser) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer utils.CheckForPanic("PSTParser")
		defer close(output_chan)

		arg := &PSTParserArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_pst: %v", err)
			return
		}

		pst_cache := pst_accessor.GetPSTCache(scope)
		pstFile, err := pst_cache.Open(scope, arg.Accessor, arg.Filename)
		if err != nil {
			scope.Log("parse_pst: %v", err)
			return
		}
		defer pstFile.Close()

		// Walk through folders and process messages
		err = pstFile.WalkFolders(func(folder *pst.Folder) error {
			return self.walkFolders(ctx, scope, arg, pstFile, folder, output_chan)
		})
		if err != nil {
			scope.Log("parse_pst: %v", err)
		}
	}()

	return output_chan
}

func (self *PSTParser) Info(scope vfilter.Scope, typeMap *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "parse_pst",
		Doc:      "Parse a PST file and extract email data.",
		ArgType:  typeMap.AddType(scope, &PSTParserArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&PSTParser{})
}
