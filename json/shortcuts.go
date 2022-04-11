package json

import (
	"fmt"
	"strings"
)

// These are shortcut methods used to operate on valid JSONL strings
// without needing to parse them and re-encode them.
func AppendJsonlItem(jsonl string, name string, value interface{}) string {
	serialized, err := Marshal(value)
	if err != nil {
		return jsonl
	}

	return strings.ReplaceAll(jsonl, "}\n",
		fmt.Sprintf(",%q:%s}\n", name, string(serialized)))
}
