package utils

type ProgressReporter interface {
	Report(progress string)
}
