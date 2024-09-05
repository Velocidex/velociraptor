package paths

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var (
	ntfsTestCases = []struct {
		path       string
		components []string
	}{{
		"C:\\Users\\mike\\server.config.yaml",
		[]string{"C:", "Users", "mike", "server.config.yaml"}}, {
		"\\\\.\\C:\\Users\\mike\\server.config.yaml",
		[]string{"\\\\.\\C:", "Users", "mike", "server.config.yaml"}}, {
		"\\\\?\\GLOBALROOT\\Device\\Volume1234\\Users\\mike\\server.config.yaml",
		[]string{"\\\\?\\GLOBALROOT\\Device\\Volume1234",
			"Users", "mike", "server.config.yaml"}}, {
		"C:\\Users\\mike\\你好世界\\你好世界.txt",
		[]string{"C:", "Users", "mike", "你好世界", "你好世界.txt"},
	}}
)

func TestNTFSPathDetection(t *testing.T) {
	for _, testcase := range ntfsTestCases {
		assert.Equal(t, testcase.components,
			ExtractClientPathComponents(testcase.path))
	}
}
