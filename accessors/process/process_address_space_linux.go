//go:build linux
// +build linux

package process

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/uploads"
)

func (self *ProcessAccessor) OpenWithOSPath(
	path *accessors.OSPath) (accessors.ReadSeekCloser, error) {
	if len(path.Components) == 0 {
		return nil, errors.New("Unable to list all processes, use the pslist() plugin.")
	}

	pid, err := strconv.ParseUint(path.Components[0], 0, 64)
	if err != nil {
		return nil, errors.New("First directory path must be a process.")
	}

	// Open the device file for the process
	fd, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		return nil, err
	}

	// Open the process and enumerate its ranges
	ranges, err := GetVads(pid)
	if err != nil {
		return nil, err
	}
	result := &ProcessReader{
		pid:    pid,
		handle: fd,
	}

	for _, r := range ranges {
		result.ranges = append(result.ranges, r)
	}

	return result, nil
}

var (
	maps_regexp = regexp.MustCompile(`(?P<Start>^[^-]+)-(?P<End>[^\s]+)\s+(?P<Perm>[^\s]+)\s+(?P<Size>[^\s]+)\s+[^\s]+\s+(?P<PermInt>[^\s]+)\s+(?P<Filename>.+?)(?P<Deleted> \(deleted\))?$`)
)

func GetVads(pid uint64) ([]*uploads.Range, error) {
	maps_fd, err := os.Open(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, err
	}
	defer maps_fd.Close()

	var result []*uploads.Range

	scanner := bufio.NewScanner(maps_fd)
	for scanner.Scan() {
		hits := maps_regexp.FindStringSubmatch(scanner.Text())
		if len(hits) > 0 {
			protection := hits[3]
			// Only include readable ranges.
			if len(protection) < 2 || protection[0] != 'r' {
				continue
			}

			start, err := strconv.ParseInt(hits[1], 16, 64)
			if err != nil {
				continue
			}

			end, err := strconv.ParseInt(hits[2], 16, 64)
			if err != nil {
				continue
			}

			// We can not read kernel memory
			if start < 0 || end < 0 {
				continue
			}

			result = append(result, &uploads.Range{
				Offset: start, Length: end - start,
			})
		}
	}

	err = scanner.Err()
	if err != nil {
		return nil, err
	}

	return result, nil
}
