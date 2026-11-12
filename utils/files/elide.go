package files

import (
	"unicode"

	"www.velocidex.com/golang/velociraptor/accessors"
)

func ElideFilename(filename *accessors.OSPath, accessor string) string {
	result := filename.String()

	// These paths can be very large so we elide them.
	switch accessor {
	case "data":
		elided_result := "data:"
		for _, r := range result {
			if unicode.IsPrint(r) {
				elided_result += string(r)
			}
			if len(elided_result) > 50 {
				elided_result += " ..."
				break
			}
		}

		return elided_result

	case "sparse":
		if len(result) > 50 {
			result = result[:50] + "..."
		}
	}

	return result
}
