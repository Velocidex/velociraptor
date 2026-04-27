package constants

const (
	MEMORY_SMALL  = 64 * 1024 * 1024
	MEMORY_MEDIUM = 10 * 1024 * 1024
	MEMORY_LARGE  = 100 * 1024 * 1024
	MEMORY_HUGE   = 1000 * 1024 * 1024

	// The absolute max number of rows we accept in a single blob.
	MAX_ROW_LIMIT = 1000000
)
