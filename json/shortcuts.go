package json

import (
	"fmt"
)

// These are shortcut methods used to operate on valid JSONL strings
// without needing to parse them and re-encode them.
func AppendJsonlItem(jsonl []byte, name string, value interface{}) []byte {
	result := make([]byte, 0, len(jsonl)+4096)
	serialized, err := Marshal(value)
	if err != nil {
		return jsonl
	}

	extra := fmt.Sprintf(",%q:%s", name, string(serialized))

	for i := 0; i < len(jsonl); i++ {
		if jsonl[i] == '}' && jsonl[i+1] == '\n' {
			for j := 0; j < len(extra); j++ {
				result = append(result, extra[j])
			}
		}

		result = append(result, jsonl[i])
	}

	return result
}
