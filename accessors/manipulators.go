package accessors

import "strings"

type LinuxPathManipulator int

func (self LinuxPathManipulator) PathSplit(path string) []string {
	components := strings.Split(path, "/")
	result := make([]string, 0, len(components))
	for _, c := range components {
		if c == "" {
			continue
		}
		result = append(result, c)
	}

	return result
}

func (self LinuxPathManipulator) PathJoin(components []string) string {
	return "/" + strings.Join(components, "/")
}

func NewLinuxOSPath(path string) *OSPath {
	manipulator := LinuxPathManipulator(0)
	return &OSPath{
		Components:  manipulator.PathSplit(path),
		Manipulator: manipulator,
	}
}
