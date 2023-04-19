// Handle recovery from a crash.

package executor

import (
	"context"
	"io/ioutil"
	"os"
	"sync"

	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

func CheckForCrashes(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup,
	exe Executor) error {

	if config_obj.Client == nil ||
		config_obj.Client.DisableCheckpoints {
		return nil
	}

	return config.MutateWriteback(config_obj.Client,
		func(wb *config_proto.Writeback) error {
			checkpoints := wb.Checkpoints
			wb.Checkpoints = nil

			wg.Add(1)
			go func() {
				defer wg.Done()

				logger := logging.GetLogger(config_obj, &logging.ClientComponent)

				for _, cp := range checkpoints {
					logger.Info("<red>Attempting to recover flow checkpoint</> at %v",
						cp.Path)
					fd, err := os.Open(cp.Path)
					if err != nil {
						continue
					}

					serialized, err := ioutil.ReadAll(fd)
					// Try to remove the checkpoint in any case
					_ = fd.Close()
					_ = os.Remove(cp.Path)

					if err == nil {
						msg := &crypto_proto.VeloMessage{}
						err = json.Unmarshal(serialized, msg)
						if err == nil {
							if msg.FlowStats != nil {
								msg.FlowStats.QueryStatus = append(
									msg.FlowStats.QueryStatus, &crypto_proto.VeloStatus{
										Status:       crypto_proto.VeloStatus_GENERIC_ERROR,
										ErrorMessage: "Client Crashed",
									})
							}

							// Inform the server about the crash.
							exe.SendToServer(msg)
						}
					}
				}
			}()
			return nil
		})
}
