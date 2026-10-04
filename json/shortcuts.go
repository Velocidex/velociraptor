package json

import (
	"bufio"
	"bytes"
	"fmt"
)

// These are shortcut methods used to operate on valid JSONL strings
// without needing to parse them and re-encode them.
func AppendJsonlItem(jsonl []byte, name string, value interface{}) []byte {
	result := bytes.NewBuffer(nil)
	serialized, err := Marshal(value)
	if err != nil {
		return nil
	}
	// Add the extra on the end of each line.
	extra := []byte(fmt.Sprintf(",%q:%s}\n", name, string(serialized)))

	// Read single lines
	scanner := bufio.NewScanner(bytes.NewReader(jsonl))
	for scanner.Scan() {
		line := scanner.Bytes()
		// At a minimum the line should be {}
		if len(line) < 2 {
			continue
		}

		// Drop invalid lines
		if line[0] != '{' || line[len(line)-1] != '}' {
			continue
		}

		// Drop the final }
		line = line[:len(line)-1]

		// Append the extra
		result.Write(line)
		result.Write(extra)
	}

	return result.Bytes()
}

func memcat(dest *[]byte, src []byte) {
	*dest = append(*dest, src...)
}

// Allows to format a JSON string safely similar to fmt.Sprintf
func Format(template string, args ...interface{}) string {
	arg_idx := 0
	result := make([]byte, 0, len(template)*2)

	for i := 0; i < len(template); i++ {
		if template[i] == '%' && i < len(template) {
			switch template[i+1] {

			// The %s format means to just copy it in.
			case 's':
				if arg_idx < len(args) {
					arg := ToString(args[arg_idx])
					memcat(&result, []byte(arg))
					arg_idx++
					i++
				}

			case 'q', 'i', 'd':
				if arg_idx < len(args) {
					arg, err := Marshal(args[arg_idx])
					if err != nil {
						arg = []byte("null")
					}
					memcat(&result, arg)
					arg_idx++
					i++
				}

			default:
				i++
			}
		} else {
			result = append(result, template[i])
		}
	}

	return string(result)
}

func ToString(x interface{}) string {
	switch t := x.(type) {
	case string:
		return t

	case []byte:
		return string(t)

	default:
		return fmt.Sprintf("%v", x)
	}
}
