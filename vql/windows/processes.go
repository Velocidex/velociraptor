// +build windows

package windows

import (
	"fmt"
	"syscall"
	"time"

	"github.com/StackExchange/wmi"
	"golang.org/x/sys/windows"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type Win32_Process struct {
	Name                  string
	ExecutablePath        *string
	CommandLine           *string
	Priority              uint32
	CreationDate          *time.Time
	ProcessID             uint32
	ThreadCount           uint32
	Status                *string
	ReadOperationCount    uint64
	ReadTransferCount     uint64
	WriteOperationCount   uint64
	WriteTransferCount    uint64
	CSCreationClassName   string
	CSName                string
	Caption               *string
	CreationClassName     string
	Description           *string
	ExecutionState        *uint16
	HandleCount           uint32
	KernelModeTime        uint64
	MaximumWorkingSetSize *uint32
	MinimumWorkingSetSize *uint32
	OSCreationClassName   string
	OSName                string
	OtherOperationCount   uint64
	OtherTransferCount    uint64
	PageFaults            uint32
	PageFileUsage         uint32
	ParentProcessID       uint32
	PeakPageFileUsage     uint32
	PeakVirtualSize       uint64
	PeakWorkingSetSize    uint32
	PrivatePageCount      uint64
	TerminationDate       *time.Time
	UserModeTime          uint64
	WorkingSetSize        uint64
}

type TimesStat struct {
	CPU       string  `json:"cpu"`
	User      float64 `json:"user"`
	System    float64 `json:"system"`
	Idle      float64 `json:"idle"`
	Nice      float64 `json:"nice"`
	Iowait    float64 `json:"iowait"`
	Irq       float64 `json:"irq"`
	Softirq   float64 `json:"softirq"`
	Steal     float64 `json:"steal"`
	Guest     float64 `json:"guest"`
	GuestNice float64 `json:"guestNice"`
	Stolen    float64 `json:"stolen"`
}

type MemoryInfoStat struct {
	RSS    uint64 `json:"rss"`    // bytes
	VMS    uint64 `json:"vms"`    // bytes
	Data   uint64 `json:"data"`   // bytes
	Stack  uint64 `json:"stack"`  // bytes
	Locked uint64 `json:"locked"` // bytes
	Swap   uint64 `json:"swap"`   // bytes
}

func (self Win32_Process) MemoryInfo() *MemoryInfoStat {
	return &MemoryInfoStat{
		RSS: uint64(self.WorkingSetSize),
		VMS: uint64(self.PageFileUsage),
	}
}

func (self Win32_Process) Pid() int32 {
	return int32(self.ProcessID)
}

func (self Win32_Process) Ppid() int32 {
	return int32(self.ParentProcessID)
}

func (self Win32_Process) Exe() string {
	if self.ExecutablePath != nil {
		return *self.ExecutablePath
	}
	return ""
}

func (self Win32_Process) Cmdline() string {
	if self.CommandLine != nil {
		return *self.CommandLine
	}
	return ""
}

type SYSTEM_TIMES struct {
	CreateTime syscall.Filetime
	ExitTime   syscall.Filetime
	KernelTime syscall.Filetime
	UserTime   syscall.Filetime
}

func getProcessCPUTimes(pid int32) (SYSTEM_TIMES, error) {
	var times SYSTEM_TIMES

	// PROCESS_QUERY_LIMITED_INFORMATION is 0x1000
	h, err := windows.OpenProcess(0x1000, false, uint32(pid))
	if err != nil {
		return times, err
	}
	defer windows.CloseHandle(h)

	err = syscall.GetProcessTimes(
		syscall.Handle(h),
		&times.CreateTime,
		&times.ExitTime,
		&times.KernelTime,
		&times.UserTime,
	)

	return times, err
}

func (self Win32_Process) CreateTime() int64 {
	return self.CreationDate.UnixNano() / 10000
}

func (self Win32_Process) Times() *TimesStat {
	return &TimesStat{
		User:   float64(self.UserModeTime) / 1000000,
		System: float64(self.KernelModeTime) / 1000000,
	}
}

func (self Win32_Process) Username() (string, error) {
	pid := self.Pid()
	// 0x1000 is PROCESS_QUERY_LIMITED_INFORMATION
	c, err := syscall.OpenProcess(0x1000, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer syscall.CloseHandle(c)

	var token syscall.Token
	err = syscall.OpenProcessToken(c, syscall.TOKEN_QUERY, &token)
	if err != nil {
		return "", err
	}
	defer token.Close()
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}

	user, domain, _, err := tokenUser.User.Sid.LookupAccount("")
	return domain + "\\" + user, err
}

func GetWin32Proc() ([]Win32_Process, error) {
	var dst []Win32_Process
	q := wmi.CreateQuery(&dst, "")
	err := wmi.Query(q, &dst)
	if err != nil {
		return []Win32_Process{}, fmt.Errorf("could not get win32Proc: %s", err)
	}

	if len(dst) == 0 {
		return []Win32_Process{}, fmt.Errorf("could not get win32Proc: empty")
	}

	return dst, nil
}

func init() {
	wmi.DefaultClient.AllowMissingFields = true

	vql_subsystem.RegisterPlugin(&vfilter.GenericListPlugin{
		PluginName: "pslist",
		Function: func(
			scope *vfilter.Scope,
			args *vfilter.Dict) []vfilter.Row {
			var result []vfilter.Row
			processes, err := GetWin32Proc()
			if err == nil {
				for _, item := range processes {
					result = append(result, item)
				}
			}
			return result
		},
		RowType: &Win32_Process{},
		Doc:     "List processes",
	})
}
