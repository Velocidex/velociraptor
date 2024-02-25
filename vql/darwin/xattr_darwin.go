package darwin

const (
	prefix = ""
)

// No-op on Darwin (Mac).
func stripPrefix(s []string) []string {
	return s
}
