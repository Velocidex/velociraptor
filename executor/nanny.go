package executor

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/shirou/gopsutil/process"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

func StartNannyService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	if config_obj.Client == nil ||
		config_obj.Client.MaxMemoryHardLimit == 0 {
		return nil
	}

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer logger.Info("<red>Exiting</> nanny")

		logger.Info("<green>Starting</> nanny")
		for {
			process, err := process.NewProcess(int32(os.Getpid()))
			if err == nil {
				meminfo, err := process.MemoryInfo()
				if err == nil && meminfo.RSS > config_obj.Client.MaxMemoryHardLimit {
					logger.Error(
						"Exiting because memory exceeded hard limit: %v %v",
						meminfo.RSS, config_obj.Client.MaxMemoryHardLimit)
					os.Exit(-1)

				}
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				continue
			}
		}
	}()

	return nil
}
