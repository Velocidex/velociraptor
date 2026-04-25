package hunt_dispatcher

import "fmt"

var (
	DEBUG = false
)

func (self *HuntDispatcher) Debug(msg string, args ...interface{}) {
	if !DEBUG {
		return
	}

	master := "minion"
	if self.I_am_master {
		master = "master"
	}
	fmt.Printf("HuntDispatcher: %v:%v %v\n",
		master, self.uuid, fmt.Sprintf(msg, args...))
}

func (self *HuntStorageManagerImpl) Debug(msg string, args ...interface{}) {
	if !DEBUG {
		return
	}

	master := "minion"
	if self.I_am_master {
		master = "master"
	}
	fmt.Printf("HuntStorageManagerImpl: %v:%v %v\n",
		master, self.uuid, fmt.Sprintf(msg, args...))
}
