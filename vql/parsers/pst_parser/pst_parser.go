package pst_parser

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	pst "github.com/mooijtech/go-pst/v6/pkg"
	"github.com/mooijtech/go-pst/v6/pkg/properties"
	"github.com/rotisserie/eris"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type PSTParserArgs struct {
	Filename   string `vfilter:"required,field=filename,doc=The PST file to parse."`
	FolderPath string `vfilter:"field=FolderPath,doc=The folder path to save the attachments from emails."`
	Accessor   string `vfilter:"optional,field=accessor,doc=The accessor to use"`
}

type PSTParser struct{}

func (self *PSTParser) Call(ctx context.Context, scope vfilter.Scope, args *ordereddict.Dict) <-chan vfilter.Row {
	outputChan := make(chan vfilter.Row)

	go func() {
		defer close(outputChan)

		arg := &PSTParserArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("pst_parser: %v", err)
			return
		}

		reader, err := os.Open(arg.Filename)
		if err != nil {
			scope.Log(arg.Filename)
			scope.Log("pst_parser: os.Open %s", arg.Filename)
			scope.Log("pst_parser: %v", err)
			return
		}

		pstFile, err := pst.New(reader)
		if err != nil {
			scope.Log("pst_parser: %v", err)
			return
		}

		defer func() {
			pstFile.Cleanup()

			if errClosing := reader.Close(); errClosing != nil {
				scope.Log("pst_parser: %v", errClosing)
			}
		}()

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

		// Walk through folders and process messages
		if err := pstFile.WalkFolders(func(folder *pst.Folder) error {

			messageIterator, err := folder.GetMessageIterator()

			if eris.Is(err, pst.ErrMessagesNotFound) {
				// Folder has no messages.
				return nil
			} else if err != nil {
				scope.Log("WalkFolders error")
				scope.Log("Walking folder: %s\n", folder.Name)
				return err
			}

			for messageIterator.Next() {
				message := messageIterator.Value()

				// Process the message and send it to the output channel
				output := ordereddict.NewDict()
				output.Set("Sender", message.Properties.(*properties.Message).GetSenderEmailAddress())
				output.Set("Receiver", message.Properties.(*properties.Message).GetReceivedByEmailAddress())
				output.Set("Subject", message.Properties.(*properties.Message).GetSubject())
				output.Set("Message", message.Properties.(*properties.Message).GetBody())
				output.Set("Body", message.Properties.(*properties.Message).String())

				// Convert the int64 timestamp (in milliseconds) to a time.Time value
				deliveryTime := message.Properties.(*properties.Message).GetMessageDeliveryTime() / 1e9
				deliveryTimeValue := time.Unix(deliveryTime, 0).UTC()

				// Format the date and time in UTC
				formattedTime := deliveryTimeValue.Format("2006-01-02 15:04:05 MST")
				output.Set("DateandTime", formattedTime)

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
				outputChan <- output
			}

			if messageIterator.Err() != nil {
				return messageIterator.Err()
			}

			return nil
		}); err != nil {
			scope.Log("pst_parser: %v", err)
		}
	}()

	return outputChan
}

func (self *PSTParser) Info(scope vfilter.Scope, typeMap *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "parse_pst",
		Doc:      "Parse a PST file and extract email data.",
		ArgType:  typeMap.AddType(scope, &PSTParserArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ, acls.FILESYSTEM_WRITE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&PSTParser{})
}
