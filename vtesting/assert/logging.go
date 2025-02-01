package assert

import (
	"regexp"

	"www.velocidex.com/golang/velociraptor/logging"
)

func MemoryLogsContain(t TestingT, regex string, msgAndArgs ...interface{}) {
	if !MemoryLogsContainRegex(regex) {
		t.Errorf("Unable to find %v in memory logs", regex, msgAndArgs)
	}
}

func MemoryLogsContainRegex(regex string) bool {
	re := regexp.MustCompile(regex)

	for _, line := range logging.GetMemoryLogs() {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}
