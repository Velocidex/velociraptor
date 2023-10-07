package utils

func Elide(in string, length int) string {
	if len(in) < length {
		return in
	}

	return in[:length] + " ..."
}
