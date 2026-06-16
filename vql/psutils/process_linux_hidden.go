//go:build linux
// +build linux

package psutils

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
)

const clockTicksPerSecond = 100

func GetProcessDirect(ctx context.Context, pid int32) (*ordereddict.Dict, error) {
	if pid <= 0 {
		return nil, fmt.Errorf("invalid pid %d", pid)
	}

	procRoot := fmt.Sprintf("/proc/%d", pid)

	statusBytes, err := os.ReadFile(procRoot + "/status")
	if err != nil {
		return nil, err
	}

	name, ppid, uid := parseProcStatus(statusBytes)

	result := ordereddict.NewDict().SetCaseInsensitive().
		Set("Pid", pid).
		Set("Name", name).
		Set("Ppid", ppid).
		Set("CommandLine", readProcCmdline(procRoot+"/cmdline"))

	// CreateTime: derived from /proc/<pid>/stat starttime + /proc/stat btime.
	if createTime, ok := readProcCreateTime(procRoot + "/stat"); ok {
		result.Set("CreateTime", createTime.Format(time.RFC3339))
	} else {
		result.Set("CreateTime", "")
	}

	// Exe and Cwd: readlink does not enumerate the parent directory.
	exe, _ := os.Readlink(procRoot + "/exe")
	result.Set("Exe", exe)

	cwd, _ := os.Readlink(procRoot + "/cwd")
	result.Set("Cwd", cwd)

	result.Set("Username", lookupUsername(uid))

	// Marker so artifacts can distinguish records recovered via the
	// fallback path from records returned by the normal gopsutil path.
	result.Set("HiddenProcess", true)

	return result, nil
}

// parseProcStatus extracts Name, PPid and the real Uid from the contents of
// /proc/<pid>/status. Returns ("", 0, -1) for missing fields.
func parseProcStatus(data []byte) (name string, ppid int32, uid int) {
	uid = -1
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Name:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
		case strings.HasPrefix(line, "PPid:"):
			if v, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:"))); err == nil {
				ppid = int32(v)
			}
		case strings.HasPrefix(line, "Uid:"):
			// "Uid:\treal\teffective\tsaved\tfs"
			fields := strings.Fields(strings.TrimPrefix(line, "Uid:"))
			if len(fields) > 0 {
				if v, err := strconv.Atoi(fields[0]); err == nil {
					uid = v
				}
			}
		}
	}
	return
}

func readProcCmdline(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	data = bytes.TrimRight(data, "\x00")
	return strings.ReplaceAll(string(data), "\x00", " ")
}

func readProcCreateTime(statPath string) (time.Time, bool) {
	data, err := os.ReadFile(statPath)
	if err != nil {
		return time.Time{}, false
	}
	closeIdx := bytes.LastIndexByte(data, ')')
	if closeIdx < 0 || closeIdx+2 >= len(data) {
		return time.Time{}, false
	}
	fields := strings.Fields(string(data[closeIdx+1:]))
	// After ')', "state" is index 0, so starttime (field 22 overall) is
	// index 19 in this slice.
	const starttimeIdx = 19
	if len(fields) <= starttimeIdx {
		return time.Time{}, false
	}
	startTicks, err := strconv.ParseUint(fields[starttimeIdx], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	btime, ok := readBtime()
	if !ok {
		return time.Time{}, false
	}
	seconds := int64(startTicks / clockTicksPerSecond)
	return time.Unix(int64(btime)+seconds, 0), true
}

func readBtime() (uint64, bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, false
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "btime ") {
			v, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "btime ")), 10, 64)
			if err != nil {
				return 0, false
			}
			return v, true
		}
	}
	return 0, false
}

func lookupUsername(uid int) string {
	if uid < 0 {
		return ""
	}
	if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
		return u.Username
	}
	return strconv.Itoa(uid)
}
