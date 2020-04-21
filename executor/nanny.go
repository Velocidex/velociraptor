package executor

import (
	"fmt"
	"os"
	"time"

	"github.com/shirou/gopsutil/process"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func StartNannyService(config_obj *config_proto.Config) {
	if config_obj.Client.MaxMemoryHardLimit == 0 {
		return
	}

	go func() {
		for {
			process, err := process.NewProcess(int32(os.Getpid()))
			if err == nil {
				meminfo, err := process.MemoryInfo()
				if err == nil && meminfo.RSS > config_obj.Client.MaxMemoryHardLimit {
					fmt.Printf("Exiting because memory exceeded hard limit: %v %v",
						meminfo.RSS, config_obj.Client.MaxMemoryHardLimit)
					os.Exit(-1)

				}
			}

			time.Sleep(10 * time.Second)
		}
	}()
}
