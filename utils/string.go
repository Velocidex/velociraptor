package utils

func Elide(in string, length int) string {
	if len(in) < length {
		return in
	}

	return in[:length] + " ..."
}

func Uniquify(in []string) []string {
	result := make([]string, 0, len(in))
	seen := make(map[string]bool)
	for _, i := range in {
		_, pres := seen[i]
		if pres {
			continue
		}
		seen[i] = true
		result = append(result, i)
	}
	return result
}
