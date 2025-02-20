package pst_parser

import (
	"context"
	"errors"
	"time"

	"github.com/Velocidex/ordereddict"
	pst "github.com/mooijtech/go-pst/v6/pkg"
	"github.com/mooijtech/go-pst/v6/pkg/properties"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PSTParserArgs struct {
	Filename   *accessors.OSPath `vfilter:"required,field=filename,doc=The PST file to parse."`
	FolderPath string            `vfilter:"field=FolderPath,doc=The folder path to save the attachments from emails."`
	Accessor   string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
}

type PSTParser struct{}

func (self *PSTParser) walkFolders(
	ctx context.Context, scope vfilter.Scope,
	folder *pst.Folder,
	output_chan chan vfilter.Row) error {

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

		// Process the message and send it to the output channel
		props, ok := message.Properties.(*properties.Message)
		if !ok || props == nil {
			continue
		}

		output := ordereddict.NewDict().
			Set("Sender", props.GetSenderEmailAddress()).
			Set("Receiver", props.GetReceivedByEmailAddress()).
			Set("Subject", props.GetSubject()).
			Set("Message", props.GetBody()).
			Set("Body", props.String()).
			Set("Delivered", time.Unix(0, props.GetMessageDeliveryTime()))

		/*
			attachmentIterator, err := message.GetAttachmentIterator()
			attachmentName := make([]string, 0)

			if eris.Is(err, pst.ErrAttachmentsNotFound) {
				// This message has no attachments.
				output.Set("AttachmentId", "NIL")
				output.Set("Attachments", "NIL")

				outputChan <- output
				continue
			} else if err != nil {
				return err
			}

			// Iterate through attachments.
			for attachmentIterator.Next() {
				attachment := attachmentIterator.Value()

				var attachmentNameId string

				if attachment.GetAttachLongFilename() != "" {
					attachmentNameId = fmt.Sprintf("%d-%s", attachment.Identifier, attachment.GetAttachLongFilename())
					attachmentName = append(attachmentName, attachmentNameId)
					// Set attachment name and Id to the output channel
					output.Set("AttachmentId", attachment.Identifier)
					output.Set("Attachments", attachmentName)

				} else {
					scope.Log("attachments/UNKNOWN_%d", attachment.Identifier)
				}

				// Save to attachments folder
				var attachmentOutputPath string

				if attachment.GetAttachLongFilename() != "" {
					attachmentOutputPath = fmt.Sprintf(arg.FolderPath+"/%d-%s", attachment.Identifier, attachment.GetAttachLongFilename())
				} else {
					attachmentOutputPath = fmt.Sprintf("attachments/UNKNOWN_%d", attachment.Identifier)
					scope.Log("attachments/UNKNOWN_%d", attachment.Identifier)
				}

				attachmentOutput, err := os.Create(attachmentOutputPath)
				if err != nil {
					return err
				}

				if _, err := attachment.WriteTo(attachmentOutput); err != nil {
					return err
				}

				if err := attachmentOutput.Close(); err != nil {
					return err
				}
			}
			if attachmentIterator.Err() != nil {
				return attachmentIterator.Err()
			}
		*/
		output_chan <- output
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

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("parse_pst: %s", err)
			return
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("parse_pst: %v", err)
			return
		}

		reader, err := accessor.OpenWithOSPath(arg.Filename)
		if err != nil {
			scope.Log("parse_pst: %v", err)
			return
		}

		pstFile, err := pst.New(utils.MakeReaderAtter(reader))
		if err != nil {
			scope.Log("parse_pst: %v", err)
			return
		}
		defer pstFile.Cleanup()

		/*
			// For writing attachment
			switch arg.Accessor {
			case "", "auto", "file":
				err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
				if err != nil {
					scope.Log("write_parser: %s", err)
				}

				// Create attachments directory
				if len(arg.FolderPath) != 0 {
					if _, err := os.Stat(arg.FolderPath); err != nil {
						if err := os.Mkdir(arg.FolderPath, 0755); err != nil {
							scope.Log("Failed to create attachments directory: %+v", err)
						}
					}
				}

			default:
				scope.Log("write_parser: Unsupported accessor for writing %v", arg.Accessor)
			}
		*/

		// Walk through folders and process messages
		err = pstFile.WalkFolders(func(folder *pst.Folder) error {
			return self.walkFolders(ctx, scope, folder, output_chan)
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
