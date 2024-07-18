package encrypted_logs

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/crypto/storage"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Write local logs to encrypted file. Enable this by setting
// Client.logfile_name in the client's config file. NOTE: This logfile
// is not truncated - it accumulates messages between client restarts
// ensuring we can debug issues like client crashes. The file is
// truncated once it reaches its maximum size (by default 10Mb )
func StartEncryptedLog(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	if config_obj.Client == nil ||
		config_obj.Client.LogfileName == "" {
		return nil
	}

	writeback_service := writeback.GetWritebackService()
	wb, err := writeback_service.GetWriteback(config_obj)
	if err != nil {
		return nil
	}

	// Server PEM not known yet - we can not write encrypted logs.
	if wb.LastServerPem == "" {
		return nil
	}

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	// This is configured by executor.SetTempfile() already.
	tmpdir := os.Getenv("TMP")
	if tmpdir == "" {
		tmpdir = "."
	}

	filename, err := filepath.Abs(
		filepath.Join(tmpdir, config_obj.Client.LogfileName))
	if err != nil {
		return err
	}

	max_size := config_obj.Client.LogfileSize
	if max_size == 0 {
		max_size = 1024 * 1024 * 10 // 10Mb default
	}

	// How long to wait between flushes to the file. Not too long so
	// logs are relatively fresh
	max_wait := time.Second * 10

	fd, err := storage.NewCryptoFileWriter(ctx, config_obj, max_size, filename)
	if err != nil {
		logger.Error("StartEncryptedLog: %v", err)
		return nil
	}
	logger.Info("<green>StartEncryptedLog</>: Starting log file in %v", filename)

	// Flush the file periodically.
	wg.Add(1)
	go func() {
		// Prevent closing down until we finished writing the logs.
		defer wg.Done()
		defer fd.Close()

		for {
			select {
			case <-ctx.Done():
				return

			case <-utils.GetTime().After(utils.Jitter(max_wait)):
				err := fd.Flush(!storage.KEEP_ON_ERROR)
				if err != nil {
					logger.Error("StartEncryptedLog: %v", err)
				}
			}
		}
	}()

	go func() {
		// Now pump messages from all the log components to the file.
		in := make(chan string)
		for _, component := range []*string{&logging.ClientComponent,
			&logging.FrontendComponent, &logging.GUIComponent, &logging.APICmponent,
			&logging.ToolComponent, &logging.GenericComponent} {

			logger := logging.GetLogger(config_obj, component)
			closer := logger.AddListener(in)
			defer closer()
		}

		count := uint64(0)
		// Start off flushing everything in memory so far
		for _, line := range logging.GetMemoryLogs() {
			fd.AddMessage(&crypto_proto.VeloMessage{
				VQLResponse: &actions_proto.VQLResponse{
					JSONLResponse: formatLine(line),
					Part:          count,
					TotalRows:     1,
				},
			})
			count++
		}

		// Make sure the prelogs hit the disk immediately
		fd.Flush(!storage.KEEP_ON_ERROR)

		// Now add new messages.
		for {
			select {
			case <-ctx.Done():
				return

			case line, ok := <-in:
				if !ok {
					return
				}

				fd.AddMessage(&crypto_proto.VeloMessage{
					VQLResponse: &actions_proto.VQLResponse{
						JSONLResponse: formatLine(line),
						Part:          count,
						TotalRows:     1,
					},
				})
				count++
			}
		}
	}()

	return nil
}

func formatLine(line string) string {
	return json.Format("{\"timestamp\":%q,\"line\":%q}\n",
		utils.GetTime().Now(), line)
}
