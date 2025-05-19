package vtesting

import (
	"regexp"

	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func MemoryLogsContain(t assert.TestingT, regex string, msgAndArgs ...interface{}) {
	if !MemoryLogsContainRegex(regex) {
		t.Errorf("Unable to find '%v' in memory logs %v", regex, msgAndArgs)
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
